package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/voxis/backend/internal/adapter/ffmpeg"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

type testPreprocessor struct {
	name  string
	calls [][2]string
}

func (p *testPreprocessor) Preprocess(_ context.Context, inputPath, outputPath string) (*port.PreprocessResult, error) {
	p.calls = append(p.calls, [2]string{inputPath, outputPath})
	if err := os.WriteFile(outputPath, []byte("processed"), 0o600); err != nil {
		return nil, err
	}
	return &port.PreprocessResult{OutputPath: outputPath, Applied: true, PreprocessorName: p.name}, nil
}

func (p *testPreprocessor) Available() bool { return true }

func (p *testPreprocessor) Name() string { return p.name }

func TestTranscribeWorkerUsesDeepFilterAfterFFmpeg(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.wav")
	if err := os.WriteFile(inputPath, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	normalizer := &testPreprocessor{name: "ffmpeg"}
	enhancer := &testPreprocessor{name: "deepfilter"}
	w := NewTranscribeWorker(TranscribeWorkerConfig{
		DefaultPreprocessor: normalizer, EnhancedPreprocessor: enhancer,
	})

	result, err := w.preprocessAudio(context.Background(), inputPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("preprocessAudio() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.tmpDir) })
	if result.preprocessorUsed != "ffmpeg+deepfilter" {
		t.Fatalf("preprocessorUsed = %q, want ffmpeg+deepfilter", result.preprocessorUsed)
	}
	if len(normalizer.calls) != 1 || normalizer.calls[0][0] != inputPath {
		t.Fatalf("normalizer calls = %#v, want input %q", normalizer.calls, inputPath)
	}
	if len(enhancer.calls) != 1 || enhancer.calls[0][0] != normalizer.calls[0][1] {
		t.Fatalf("enhancer calls = %#v, want normalized input %q", enhancer.calls, normalizer.calls[0][1])
	}
	if result.outputPath != enhancer.calls[0][1] {
		t.Fatalf("outputPath = %q, want enhanced output %q", result.outputPath, enhancer.calls[0][1])
	}
}

type truncatingPreprocessor struct{ testPreprocessor }

func (p *truncatingPreprocessor) Preprocess(_ context.Context, inputPath, outputPath string) (*port.PreprocessResult, error) {
	p.calls = append(p.calls, [2]string{inputPath, outputPath})
	return nil, fmt.Errorf("preprocess: %w", ffmpeg.ErrPreprocessOutputTruncated)
}

// Long media past the 48 kHz size cap is chunked from the full, unenhanced
// source rather than from a truncated enhanced copy.
func TestPrepareChunkInputSkipsEnhancementWhenOutputWouldBeTruncated(t *testing.T) {
	tmpDir := t.TempDir()
	decrypted := filepath.Join(tmpDir, "decrypted.webm")
	if err := os.WriteFile(decrypted, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	normalizer := &truncatingPreprocessor{testPreprocessor{name: "ffmpeg"}}
	enhancer := &testPreprocessor{name: "deepfilter"}
	w := NewTranscribeWorker(TranscribeWorkerConfig{DefaultPreprocessor: normalizer, EnhancedPreprocessor: enhancer})
	trans := &domain.Transcription{EnhanceAudio: true}
	media := &domain.Media{ContentType: "audio/webm", Duration: 7 * 3600}

	source, contentType, cleanup := w.prepareChunkInput(context.Background(), trans, media, decrypted,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer cleanup()

	if source != decrypted || contentType != "audio/webm" {
		t.Fatalf("chunk input = %q (%s), want the full source %q", source, contentType, decrypted)
	}
	if trans.PreprocessorUsed != "" {
		t.Fatalf("PreprocessorUsed = %q, want empty after the fallback", trans.PreprocessorUsed)
	}
	if len(normalizer.calls) != 1 || len(enhancer.calls) != 0 {
		t.Fatalf("normalizer calls = %d, enhancer calls = %d; want 1 and 0", len(normalizer.calls), len(enhancer.calls))
	}
}

func TestOSSFinalProviderFailurePersistsAfterCanceledJobContext(t *testing.T) {
	repo, transcription := ossRetryTestTranscription(t)
	worker := NewTranscribeWorker(TranscribeWorkerConfig{TransRepo: repo})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()

	worker.failIfLastAttempt(ctx, &river.Job[TranscribeJobArgs]{
		Args:   TranscribeJobArgs{TranscriptionID: transcription.ID},
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
	}, transcription, fmt.Errorf("provider temporarily unavailable"), slog.Default())

	updated, err := repo.GetByID(context.Background(), transcription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.TranscriptionStatusFailed {
		t.Fatalf("status = %q, want failed", updated.Status)
	}
	if updated.ErrorMessage != "submission failed after 3 attempts" {
		t.Fatalf("error message = %q", updated.ErrorMessage)
	}
	if !repo.updateHasDeadline {
		t.Fatal("final failure update did not have a deadline")
	}
	if repo.updateDeadline.Before(started) || repo.updateDeadline.After(started.Add(finalAttemptUpdateTimeout+time.Second)) {
		t.Fatalf("final failure update deadline = %v, want within %s", repo.updateDeadline, finalAttemptUpdateTimeout)
	}
}

func TestOSSRetryableFailureBeforeLastAttemptStaysPending(t *testing.T) {
	repo, transcription := ossRetryTestTranscription(t)
	worker := NewTranscribeWorker(TranscribeWorkerConfig{TransRepo: repo})

	worker.failIfLastAttempt(context.Background(), &river.Job[TranscribeJobArgs]{
		Args:   TranscribeJobArgs{TranscriptionID: transcription.ID},
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
	}, transcription, fmt.Errorf("provider temporarily unavailable"), slog.Default())

	updated, err := repo.GetByID(context.Background(), transcription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.TranscriptionStatusPending || updated.ErrorMessage != "" {
		t.Fatalf("retryable row = status %q, error %q", updated.Status, updated.ErrorMessage)
	}
}

func TestOSSWorkFinalRetryableFailurePersists(t *testing.T) {
	worker, repo, transcription := ossWorkRetryHarness(t)
	err := worker.Work(context.Background(), &river.Job[TranscribeJobArgs]{
		Args: TranscribeJobArgs{
			TranscriptionID: transcription.ID,
			MediaID:         transcription.MediaID,
			OrgID:           transcription.OrganizationID,
		},
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
	})
	if err == nil {
		t.Fatal("Work() error = nil, want missing provider error")
	}
	updated, getErr := repo.GetByID(context.Background(), transcription.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if updated.Status != domain.TranscriptionStatusFailed {
		t.Fatalf("status = %q, want failed", updated.Status)
	}
	if updated.ErrorMessage != "submission failed after 3 attempts" {
		t.Fatalf("error message = %q", updated.ErrorMessage)
	}
}

func TestOSSWorkRetryableFailureBeforeLastAttemptStaysPending(t *testing.T) {
	worker, repo, transcription := ossWorkRetryHarness(t)
	err := worker.Work(context.Background(), &river.Job[TranscribeJobArgs]{
		Args: TranscribeJobArgs{
			TranscriptionID: transcription.ID,
			MediaID:         transcription.MediaID,
			OrgID:           transcription.OrganizationID,
		},
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
	})
	if err == nil {
		t.Fatal("Work() error = nil, want missing provider error")
	}
	updated, getErr := repo.GetByID(context.Background(), transcription.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if updated.Status != domain.TranscriptionStatusPending || updated.ErrorMessage != "" {
		t.Fatalf("retryable row = status %q, error %q", updated.Status, updated.ErrorMessage)
	}
}

func TestOSSFinalCancelAndSnoozeStayPending(t *testing.T) {
	for _, err := range []error{
		river.JobCancel(fmt.Errorf("permanent input failure")),
		river.JobSnooze(time.Second),
	} {
		repo, transcription := ossRetryTestTranscription(t)
		worker := NewTranscribeWorker(TranscribeWorkerConfig{TransRepo: repo})
		worker.failIfLastAttempt(context.Background(), &river.Job[TranscribeJobArgs]{
			Args:   TranscribeJobArgs{TranscriptionID: transcription.ID},
			JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		}, transcription, err, slog.Default())

		updated, getErr := repo.GetByID(context.Background(), transcription.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if updated.Status != domain.TranscriptionStatusPending || updated.ErrorMessage != "" {
			t.Fatalf("non-retryable job error changed row: status %q, error %q", updated.Status, updated.ErrorMessage)
		}
	}
}

type ossRetryTranscriptionRepository struct {
	port.TranscriptionRepository
	items             map[string]*domain.Transcription
	updateHasDeadline bool
	updateDeadline    time.Time
}

func newOSSRetryTranscriptionRepository() *ossRetryTranscriptionRepository {
	return &ossRetryTranscriptionRepository{items: make(map[string]*domain.Transcription)}
}

func (r *ossRetryTranscriptionRepository) Create(_ context.Context, transcription *domain.Transcription) error {
	r.items[transcription.ID] = transcription.Clone()
	return nil
}

func (r *ossRetryTranscriptionRepository) GetByID(_ context.Context, id string) (*domain.Transcription, error) {
	transcription, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return transcription.Clone(), nil
}

func (r *ossRetryTranscriptionRepository) Update(ctx context.Context, transcription *domain.Transcription) error {
	deadline, hasDeadline := ctx.Deadline()
	r.updateHasDeadline = hasDeadline
	r.updateDeadline = deadline
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := r.items[transcription.ID]; !ok {
		return domain.ErrNotFound
	}
	r.items[transcription.ID] = transcription.Clone()
	return nil
}

type ossRetryMediaRepository struct {
	port.MediaRepository
	items map[string]*domain.Media
}

func newOSSRetryMediaRepository() *ossRetryMediaRepository {
	return &ossRetryMediaRepository{items: make(map[string]*domain.Media)}
}

func (r *ossRetryMediaRepository) GetByID(_ context.Context, id string) (*domain.Media, error) {
	media, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return media.Clone(), nil
}

func ossRetryTestTranscription(t *testing.T) (*ossRetryTranscriptionRepository, *domain.Transcription) {
	t.Helper()
	repo := newOSSRetryTranscriptionRepository()
	transcription, err := domain.NewTranscription("oss-retry-org", "oss-retry-media", []string{"auto"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	transcription.ID = "oss-retry-transcription"
	if err := repo.Create(context.Background(), transcription); err != nil {
		t.Fatal(err)
	}
	return repo, transcription
}

func ossWorkRetryHarness(t *testing.T) (*TranscribeWorker, *ossRetryTranscriptionRepository, *domain.Transcription) {
	t.Helper()
	mediaRepo := newOSSRetryMediaRepository()
	media, err := domain.NewMedia("oss-retry-org", "retry.wav", "audio/wav", domain.MinMediaSize)
	if err != nil {
		t.Fatal(err)
	}
	media.ID = "oss-retry-media"
	media.Status = domain.MediaStatusReady
	media.ScanStatus = domain.ScanStatusClean
	mediaRepo.items[media.ID] = media.Clone()
	repo := newOSSRetryTranscriptionRepository()
	transcription, err := domain.NewTranscription(media.OrganizationID, media.ID, []string{"auto"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	transcription.ID = "oss-retry-transcription"
	if err := repo.Create(context.Background(), transcription); err != nil {
		t.Fatal(err)
	}
	worker := NewTranscribeWorker(TranscribeWorkerConfig{MediaRepo: mediaRepo, TransRepo: repo})
	return worker, repo, transcription
}
