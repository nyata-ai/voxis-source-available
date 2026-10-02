package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/voxis/backend/internal/domain"
)

func TestProviderUploadNameDropsTheUsersFilename(t *testing.T) {
	tests := map[string]string{
		"Counseling session - Ada Smith.MP3": "audio.mp3",
		"interview.m4a":                      "audio.m4a",
		"no-extension":                       "audio",
		"weird.ext with space":               "audio",
		"archive.verylongext":                "audio",
		"":                                   "audio",
	}
	for in, want := range tests {
		if got := providerUploadName(in); got != want {
			t.Errorf("providerUploadName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEstimateTranscribeScratchBytes(t *testing.T) {
	media := &domain.Media{Size: 10 << 20, Duration: 3600}
	if got, want := estimateTranscribeScratchBytes(media, false, time.Hour), int64(20<<20)+3600*pcm16kBytesPerSecond; got != want {
		t.Fatalf("plain estimate = %d, want %d", got, want)
	}
	enhanced := estimateTranscribeScratchBytes(media, true, time.Hour)
	if want := int64(20<<20) + 3600*(pcm16kBytesPerSecond+2*pcm48kBytesPerSecond); enhanced != want {
		t.Fatalf("enhanced estimate = %d, want %d", enhanced, want)
	}
	unknown := estimateTranscribeScratchBytes(&domain.Media{Size: 1}, false, 2*time.Hour)
	if want := int64(2) + 7200*pcm16kBytesPerSecond; unknown != want {
		t.Fatalf("unknown-duration estimate = %d, want the maximum-duration bound %d", unknown, want)
	}
}

func TestTranscribeWorkerRefusesMediaLongerThanTheLimit(t *testing.T) {
	worker, repo, transcription := ossWorkRetryHarness(t)
	worker.maxMediaDuration = time.Hour
	mediaRepo, ok := worker.mediaRepo.(*ossRetryMediaRepository)
	if !ok {
		t.Fatal("unexpected media repository type")
	}
	mediaRepo.items[transcription.MediaID].Duration = 2 * 3600
	worker.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	_, _, err := worker.loadAndValidate(context.Background(), TranscribeJobArgs{
		TranscriptionID: transcription.ID, MediaID: transcription.MediaID, OrgID: transcription.OrganizationID,
	}, worker.logger)
	var cancelErr *river.JobCancelError
	if !errors.As(err, &cancelErr) || !errors.Is(err, domain.ErrMediaTooLong) {
		t.Fatalf("loadAndValidate() error = %v, want a JobCancel for ErrMediaTooLong", err)
	}
	updated, err := repo.GetByID(context.Background(), transcription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.TranscriptionStatusFailed {
		t.Fatalf("status = %q, want failed", updated.Status)
	}
}
