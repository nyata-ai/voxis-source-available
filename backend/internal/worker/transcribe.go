package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/voxis/backend/internal/adapter/ffmpeg"
	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/vocab"
)

// TranscribeJobArgs holds the arguments for a transcription job.
type TranscribeJobArgs struct {
	TranscriptionID string `json:"transcription_id"`
	MediaID         string `json:"media_id"`
	OrgID           string `json:"org_id"`
}

// Kind returns the unique job kind identifier.
func (TranscribeJobArgs) Kind() string { return "transcribe" }

// InsertOpts provides default insertion options for all transcription jobs.
func (TranscribeJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "transcription",
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStatePending,
				rivertype.JobStateAvailable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// TranscribeWorker processes transcription jobs by decrypting media,
// uploading to a transcription provider, and submitting for processing.
type TranscribeWorker struct {
	river.WorkerDefaults[TranscribeJobArgs]
	mediaRepo            port.MediaRepository
	transRepo            port.TranscriptionRepository
	segRepo              port.TranscriptionSegmentRepository
	storage              port.StorageClient
	envelope             *crypto.EnvelopeService
	provider             port.TranscriptionProvider
	smRepo               port.SpeechmaticsTranscriptionRepository
	smSegRepo            port.SpeechmaticsSegmentRepository
	chunker              port.AudioChunker
	prober               port.AudioProber
	defaultPreprocessor  port.AudioPreprocessor // FFmpeg (nil = unavailable)
	enhancedPreprocessor port.AudioPreprocessor // DeepFilterNet (nil = unavailable)
	callbackBaseURL      string
	webhookSecret        string
	customVocabulary     bool
	speechmaticsWebhook  bool
	sandboxRequired      bool
	// maxMediaDuration is MEDIA_MAX_DURATION plus domain.MediaDurationGrace:
	// the longest source the worker processes and the ffmpeg output cap.
	maxMediaDuration time.Duration
	logger           *slog.Logger
}

const (
	finalAttemptUpdateTimeout = 15 * time.Second

	// speechmaticsChunkSeconds is the chunk length for Speechmatics jobs.
	//
	// Speechmatics documents no SaaS duration cap, only a 1 GB request body.
	// Chunking is what we are trying to avoid, not what we are trying to do:
	// diarization runs independently per chunk and the stitcher does not
	// reconcile speaker labels across chunks, so every extra split costs
	// speaker accuracy.
	//
	// A live probe on 2026-09-03 submitted a single 3.3-hour job (269 MB mp3,
	// 11,759 s): accepted by the SaaS API, uploaded in 44 s, finished in about
	// a minute (jobs deleted afterwards). Three hours is inside that measured
	// envelope and yields ~345 MB per 16 kHz mono 16-bit WAV chunk, under the
	// adapter's 900 MB spool ceiling. Keep this a single constant so raising it
	// stays a one-line change.
	speechmaticsChunkSeconds = 3 * 60 * 60

	// speechmaticsMaxSingleJobBytes caps the plaintext size that may go as one
	// Speechmatics job. Duration alone does not bound the request body — a
	// short but high-bitrate source can still be huge — and the adapter rejects
	// anything past its 900 MB spool ceiling as a PERMANENT failure. Chunking
	// re-encodes to 16 kHz mono WAV, which shrinks such a source far below the
	// ceiling, so the guard turns a dead job into a working one.
	speechmaticsMaxSingleJobBytes int64 = 800 << 20

	maxChunkSegments = 200
)

// TranscribeWorkerConfig holds all dependencies for a TranscribeWorker.
//
// Selector picks the transcription provider per job; leaving it nil means
// "always Provider", which keeps the wiring identical to the single-provider
// behavior. SpeechmaticsRepo/SpeechmaticsSegRepo are needed only when Selector
// can return the Speechmatics provider.
type TranscribeWorkerConfig struct {
	SandboxRequired bool
	// MaxMediaDuration is MEDIA_MAX_DURATION; zero means the domain default.
	MaxMediaDuration     time.Duration
	MediaRepo            port.MediaRepository
	TransRepo            port.TranscriptionRepository
	SegRepo              port.TranscriptionSegmentRepository
	Storage              port.StorageClient
	Envelope             *crypto.EnvelopeService
	Provider             port.TranscriptionProvider
	SpeechmaticsRepo     port.SpeechmaticsTranscriptionRepository
	SpeechmaticsSegRepo  port.SpeechmaticsSegmentRepository
	Chunker              port.AudioChunker
	Prober               port.AudioProber
	DefaultPreprocessor  port.AudioPreprocessor
	EnhancedPreprocessor port.AudioPreprocessor
	CallbackBaseURL      string
	WebhookSecret        string
	// CustomVocabularyEnabled mirrors CUSTOM_VOCABULARY_ENABLED. When false the
	// worker ignores any pack ids already persisted on a transcription row, so
	// turning the flag off actually stops terms reaching the provider.
	CustomVocabularyEnabled bool
	// SpeechmaticsWebhookEnabled mirrors SPEECHMATICS_WEBHOOK_ENABLED. When
	// false, Speechmatics jobs are submitted with no callback at all and the
	// poller plus the post-submit wait job remain the completion paths.
	SpeechmaticsWebhookEnabled bool
	Logger                     *slog.Logger
}

// NewTranscribeWorker creates a new TranscribeWorker with the given config.
func NewTranscribeWorker(cfg TranscribeWorkerConfig) *TranscribeWorker {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxMediaDuration <= 0 {
		cfg.MaxMediaDuration = domain.DefaultMaxMediaDuration
	}
	return &TranscribeWorker{
		sandboxRequired:      cfg.SandboxRequired,
		mediaRepo:            cfg.MediaRepo,
		transRepo:            cfg.TransRepo,
		segRepo:              cfg.SegRepo,
		storage:              cfg.Storage,
		envelope:             cfg.Envelope,
		provider:             cfg.Provider,
		smRepo:               cfg.SpeechmaticsRepo,
		smSegRepo:            cfg.SpeechmaticsSegRepo,
		chunker:              cfg.Chunker,
		prober:               cfg.Prober,
		defaultPreprocessor:  cfg.DefaultPreprocessor,
		enhancedPreprocessor: cfg.EnhancedPreprocessor,
		callbackBaseURL:      cfg.CallbackBaseURL,
		webhookSecret:        cfg.WebhookSecret,
		customVocabulary:     cfg.CustomVocabularyEnabled,
		speechmaticsWebhook:  cfg.SpeechmaticsWebhookEnabled,
		maxMediaDuration:     cfg.MaxMediaDuration + domain.MediaDurationGrace,
		logger:               cfg.Logger,
	}
}

// Timeout returns the maximum duration a transcription job may run.
func (w *TranscribeWorker) Timeout(*river.Job[TranscribeJobArgs]) time.Duration {
	return 2 * time.Hour
}

// Work executes the transcription pipeline:
//  1. Load transcription record
//  2. Load and validate media record
//
// //  3. Decrypt media from storage
//  4. Upload decrypted audio to transcription provider
//  5. Submit transcription request
//  6. Update transcription status to submitted
func (w *TranscribeWorker) Work(ctx context.Context, job *river.Job[TranscribeJobArgs]) error {
	args := job.Args
	logger := w.logger.With(
		"transcription_id", args.TranscriptionID,
		"media_id", args.MediaID,
		"org_id", args.OrgID,
	)

	// 1. Load transcription and media, validate preconditions.
	trans, media, err := w.loadAndValidate(ctx, args, logger)
	if err != nil {
		return err
	}

	if err := w.submit(ctx, job, trans, media, logger); err != nil {
		w.failIfLastAttempt(ctx, job, trans, err, logger)
		return err
	}
	return nil
}

// submit runs the OSS provider pipeline after validation. Permanent paths mark
// the row themselves; every other error remains retryable until River's final
// attempt is reached.
func (w *TranscribeWorker) submit(
	ctx context.Context,
	job *river.Job[TranscribeJobArgs],
	trans *domain.Transcription,
	media *domain.Media,
	logger *slog.Logger,
) error {
	args := job.Args

	if w.provider == nil {
		return fmt.Errorf("speechmatics provider is not configured")
	}
	logger = logger.With("provider", domain.TranscriptionProviderSpeechmatics)
	if err := w.ensureScratchSpace(trans, media); err != nil {
		return err
	}

	var providerJobID string
	if w.needsChunking(media) {
		if err := w.submitChunked(ctx, args, trans, media, logger); err != nil {
			return err
		}
	} else {
		// 3. Decrypt and upload, then submit to provider.
		var submitErr error
		providerJobID, submitErr = w.decryptUploadSubmit(ctx, args, trans, media, logger)
		if submitErr != nil {
			return submitErr
		}

		// 4. Update transcription status to submitted.
		if err := w.persistParentSubmission(ctx, trans, providerJobID); err != nil {
			return err
		}
	}

	if providerJobID != "" {
		logger.Info("transcription submitted to provider", "provider_job_id", providerJobID)
	} else {
		logger.Info("long transcription submitted in chunked mode")
	}

	return nil
}

// failIfLastAttempt records a provider failure before River discards its final
// retry. This covers both single uploads and the chunked Speechmatics path.
func (w *TranscribeWorker) failIfLastAttempt(
	ctx context.Context,
	job *river.Job[TranscribeJobArgs],
	trans *domain.Transcription,
	err error,
	logger *slog.Logger,
) {
	var cancelErr *river.JobCancelError
	var snoozeErr *river.JobSnoozeError
	if errors.As(err, &cancelErr) || errors.As(err, &snoozeErr) {
		return
	}
	if job.MaxAttempts <= 0 || job.Attempt < job.MaxAttempts {
		return
	}
	logger.Error("transcription submission failed on last attempt", "attempt", job.Attempt, "error", err)
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalAttemptUpdateTimeout)
	defer cancel()
	w.failTranscription(
		failCtx,
		trans,
		fmt.Sprintf("submission failed after %d attempts", job.Attempt),
		logger,
	)
}

// loadAndValidate loads the transcription and media records, validating
// preconditions. Returns JobCancel for permanent failures.
func (w *TranscribeWorker) loadAndValidate(
	ctx context.Context,
	args TranscribeJobArgs,
	logger *slog.Logger,
) (*domain.Transcription, *domain.Media, error) {
	trans, err := w.transRepo.GetByID(ctx, args.TranscriptionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			logger.Warn("transcription not found, canceling job")
			return nil, nil, river.JobCancel(fmt.Errorf("transcription %s not found", args.TranscriptionID))
		}
		return nil, nil, fmt.Errorf("load transcription: %w", err)
	}

	// Only process pending transcriptions — prevent double-submission.
	if trans.Status != domain.TranscriptionStatusPending {
		logger.Info("transcription not in pending state, skipping",
			"status", trans.Status,
		)
		return nil, nil, river.JobCancel(fmt.Errorf("transcription %s already %s", args.TranscriptionID, trans.Status))
	}

	// Verify org isolation between job args and loaded records.
	if trans.OrganizationID != args.OrgID {
		logger.Error("org mismatch between job args and transcription",
			"expected_org", args.OrgID,
			"actual_org", trans.OrganizationID,
		)
		return nil, nil, river.JobCancel(fmt.Errorf("org mismatch for transcription %s", args.TranscriptionID))
	}

	media, err := w.mediaRepo.GetByID(ctx, args.MediaID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			logger.Warn("media not found, canceling job")
			w.failTranscription(ctx, trans, "media not found", logger)
			return nil, nil, river.JobCancel(fmt.Errorf("media %s not found", args.MediaID))
		}
		return nil, nil, fmt.Errorf("load media: %w", err)
	}

	if media.OrganizationID != args.OrgID {
		logger.Error("org mismatch between job args and media",
			"expected_org", args.OrgID,
			"actual_org", media.OrganizationID,
		)
		return nil, nil, river.JobCancel(fmt.Errorf("org mismatch for media %s", args.MediaID))
	}

	if media.Status != domain.MediaStatusReady {
		logger.Warn("media not ready", "media_status", media.Status)
		w.failTranscription(ctx, trans, "media not ready", logger)
		return nil, nil, river.JobCancel(fmt.Errorf("media %s not ready (status=%s)", args.MediaID, media.Status))
	}

	// Gate on scan status — wait for scan to complete before transcribing.
	// NewMedia always stamps pending, so an empty status is an unknown state
	// (a hand-written row, a failed migration) and falls through to the default
	// branch: this gate fails closed rather than transcribing unscanned audio.
	switch media.ScanStatus {
	case domain.ScanStatusClean, domain.ScanStatusSkipped:
		// OK to proceed
	case domain.ScanStatusPending:
		return nil, nil, river.JobSnooze(30 * time.Second)
	default:
		// Infected, error, empty, or unknown — cancel permanently
		logger.Warn("media failed scan, canceling transcription", "scan_status", media.ScanStatus)
		w.failTranscription(ctx, trans, fmt.Sprintf("media scan status: %s", media.ScanStatus), logger)
		return nil, nil, river.JobCancel(fmt.Errorf("media scan status: %s", media.ScanStatus))
	}

	// Media stored before MEDIA_MAX_DURATION existed, or whose probe failed at
	// upload, reaches the worker unchecked. Refuse it before any decryption.
	if limitErr := domain.CheckMediaDuration(media.Duration, w.maxMediaDuration); limitErr != nil {
		w.failTranscription(ctx, trans, "audio is longer than the maximum allowed duration", logger)
		return nil, nil, river.JobCancel(limitErr)
	}

	return trans, media, nil
}

// decryptUploadSubmit performs the decrypt -> upload -> submit pipeline.
// If ffmpeg is available, metadata is stripped before uploading to the provider.
func (w *TranscribeWorker) decryptUploadSubmit(
	ctx context.Context,
	args TranscribeJobArgs,
	trans *domain.Transcription,
	media *domain.Media,
	logger *slog.Logger,
) (string, error) {
	// Decrypt media from storage.
	encrypted, err := w.storage.Download(ctx, media.StorageKey)
	if err != nil {
		return "", fmt.Errorf("download encrypted media: %w", err)
	}

	sealedMeta := &crypto.SealedMeta{
		EncryptionMeta: crypto.EncryptionMeta{
			Algorithm:     media.EncryptionAlgo,
			ChunkSize:     media.ChunkSize,
			ChunkCount:    media.ChunkCount,
			PlaintextSize: media.Size,
		},
		WrappedDEK:    media.WrappedDEK,
		WrappingNonce: media.WrappingNonce,
	}

	// If enhance requested AND a preprocessor is available, try preprocessing path.
	// Preprocessing subsumes metadata stripping (FFmpeg produces a fresh WAV).
	if trans.EnhanceAudio && w.defaultPreprocessor != nil && w.defaultPreprocessor.Available() {
		audioURL, preprocessorUsed, ppErr := w.decryptPreprocessUpload(ctx, args.OrgID, media, sealedMeta, encrypted, logger)
		if ppErr == nil {
			trans.PreprocessorUsed = preprocessorUsed
			// Submit to provider with preprocessed audio.
			return w.submitAndClassify(ctx, trans, audioURL, logger)
		}
		logger.Warn("audio preprocessing failed, falling back to baseline path", "error", ppErr)
		// Re-download because decryptPreprocessUpload consumed the encrypted reader.
		encrypted, err = w.storage.Download(ctx, media.StorageKey)
		if err != nil {
			return "", fmt.Errorf("re-download encrypted media after preprocess failure: %w", err)
		}
	}

	audioURL, err := w.baselineUpload(ctx, args.OrgID, encrypted, media, sealedMeta, logger)
	if err != nil {
		return "", w.classifyUploadFailure(ctx, trans, err, logger)
	}

	// Submit transcription to provider.
	return w.submitAndClassify(ctx, trans, audioURL, logger)
}

// baselineUpload decrypts and uploads without preprocessing. When ffmpeg is
// available it spools to a temp file and strips metadata first; otherwise it
// streams directly to the provider.
func (w *TranscribeWorker) baselineUpload(
	ctx context.Context,
	orgID string,
	encrypted io.ReadCloser,
	media *domain.Media,
	sealedMeta *crypto.SealedMeta,
	logger *slog.Logger,
) (string, error) {
	if ffmpegAvailable() {
		return w.decryptStripUpload(ctx, orgID, encrypted, media, sealedMeta, logger)
	}
	logger.Warn("ffmpeg not available, skipping metadata stripping")
	audioURL, decryptErr, uploadErr := w.pipeDecryptUpload(ctx, orgID, encrypted, media, sealedMeta)
	if decryptErr != nil {
		// Tag with the errDecryption sentinel like the sibling paths, or
		// classifyUploadFailure treats a permanently poisoned decrypt as
		// transient and River retries it forever.
		return audioURL, wrapDecryptionError(decryptErr)
	}
	return audioURL, uploadErr
}

// classifyUploadFailure maps an upload error to
// either a permanent JobCancel (auth/decryption) or a transient retry error.
func (w *TranscribeWorker) classifyUploadFailure(
	ctx context.Context,
	trans *domain.Transcription,
	err error,
	logger *slog.Logger,
) error {
	if errors.Is(err, domain.ErrUnauthorized) {
		w.failTranscription(ctx, trans, "provider authentication failed", logger)
		return river.JobCancel(fmt.Errorf("provider upload unauthorized: %w", err))
	}
	// Check if this is a decryption error (permanent).
	if isDecryptionError(err) {
		logger.Error("decryption failed", "error", err)
		w.failTranscription(ctx, trans, "decryption failed", logger)
		return river.JobCancel(fmt.Errorf("decrypt media: %w", err))
	}
	return fmt.Errorf("provider upload: %w", err) // transient — retry
}

// submitAndClassify submits audio to the provider and classifies permanent versus transient failures.
func (w *TranscribeWorker) submitAndClassify(
	ctx context.Context,
	trans *domain.Transcription,
	audioURL string,
	logger *slog.Logger,
) (string, error) {
	providerJobID, submitErr := w.submitToProvider(ctx, trans, audioURL, trans.ID, false)
	if submitErr != nil {
		// Bad credentials surface from Submit, not Upload, on providers whose
		// "upload" is a local staging step. Retrying cannot fix a rejected key,
		// so fail the row instead of stranding it in pending until the
		// attempts run out. Mirrors classifyUploadFailure.
		if errors.Is(submitErr, domain.ErrUnauthorized) {
			w.failTranscription(ctx, trans, "provider authentication failed", logger)
			return "", river.JobCancel(fmt.Errorf("provider submit unauthorized: %w", submitErr))
		}
		if errors.Is(submitErr, domain.ErrInvalidInput) {
			w.failTranscription(ctx, trans, "Speechmatics"+" rejected request: "+submitErr.Error(), logger)
			return "", river.JobCancel(fmt.Errorf("%s submit bad request: %w", domain.TranscriptionProviderSpeechmatics, submitErr))
		}
		return "", fmt.Errorf("%s submit: %w", domain.TranscriptionProviderSpeechmatics, submitErr) // transient — retry
	}
	return providerJobID, nil
}

// decryptStripUpload decrypts to a temp file, strips metadata, and uploads
// the stripped file to the provider. Falls through to unstripped upload if
// metadata stripping fails.
func (w *TranscribeWorker) decryptStripUpload(
	ctx context.Context,
	orgID string,
	encrypted io.ReadCloser,
	media *domain.Media,
	sealedMeta *crypto.SealedMeta,
	logger *slog.Logger,
) (string, error) {
	decryptedPath, spoolErr := w.decryptToTempFile(ctx, orgID, encrypted, sealedMeta)
	// Close the encrypted reader — decryptToTempFile consumed it fully.
	if closeErr := encrypted.Close(); closeErr != nil {
		logger.Warn("close encrypted reader", "error", closeErr)
	}
	if spoolErr != nil {
		return "", wrapDecryptionError(spoolErr)
	}
	defer func() { _ = os.Remove(decryptedPath) }() //nolint:errcheck // best-effort cleanup

	uploadPath := decryptedPath
	strippedPath, stripErr := ffmpeg.StripMetadata(ctx, decryptedPath, media.ContentType, w.ffmpegOptions()...)
	if stripErr != nil {
		logger.Warn("metadata stripping failed, uploading with metadata", "error", stripErr)
	} else {
		defer func() { _ = os.Remove(strippedPath) }() //nolint:errcheck // best-effort cleanup
		uploadPath = strippedPath
	}

	f, openErr := os.Open(uploadPath) //nolint:gosec // path comes from trusted temp dir
	if openErr != nil {
		return "", fmt.Errorf("open file for upload: %w", openErr)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // best-effort cleanup

	audioURL, uploadErr := w.provider.Upload(ctx, f, providerUploadName(media.Filename), media.ContentType)
	if uploadErr != nil {
		return "", uploadErr
	}
	return audioURL, nil
}

// preprocessResult holds the output of a preprocessing pipeline run.
type preprocessResult struct {
	tmpDir           string // caller must defer os.RemoveAll
	outputPath       string
	preprocessorUsed string
}

// preprocessAudio runs the FFmpeg → optional DeepFilter pipeline on inputPath.
// Returns a result containing a temp directory (caller must clean up) and the
// path to the final preprocessed file within it.
func (w *TranscribeWorker) preprocessAudio(ctx context.Context, inputPath string, logger *slog.Logger) (*preprocessResult, error) {
	ppDir, err := os.MkdirTemp("", "voxis-preprocess-*")
	if err != nil {
		return nil, fmt.Errorf("create preprocess temp dir: %w", err)
	}

	normalizedPath := filepath.Join(ppDir, "normalized.wav")
	ffResult, ppErr := w.defaultPreprocessor.Preprocess(ctx, inputPath, normalizedPath)
	if ppErr != nil {
		os.RemoveAll(ppDir) //nolint:errcheck // best-effort cleanup on error
		return nil, fmt.Errorf("preprocess audio: %w", ppErr)
	}

	preprocessorUsed := ffResult.PreprocessorName
	outputPath := normalizedPath

	if w.enhancedPreprocessor != nil && w.enhancedPreprocessor.Available() {
		enhancedPath := filepath.Join(ppDir, "enhanced.wav")
		dfResult, dfErr := w.enhancedPreprocessor.Preprocess(ctx, normalizedPath, enhancedPath)
		if dfErr != nil {
			logger.Warn("DeepFilterNet enhancement failed, using FFmpeg-only output", "error", dfErr)
		} else if dfResult.Applied {
			outputPath = enhancedPath
			preprocessorUsed = ffResult.PreprocessorName + "+" + dfResult.PreprocessorName
		}
	}

	logger.Info("preprocessing complete", "preprocessor_used", preprocessorUsed)

	return &preprocessResult{
		tmpDir:           ppDir,
		outputPath:       outputPath,
		preprocessorUsed: preprocessorUsed,
	}, nil
}

// decryptPreprocessUpload decrypts to temp file, runs preprocessing pipeline, and uploads.
func (w *TranscribeWorker) decryptPreprocessUpload(
	ctx context.Context,
	orgID string,
	media *domain.Media,
	sealedMeta *crypto.SealedMeta,
	encrypted io.ReadCloser,
	logger *slog.Logger,
) (audioURL, preprocessorUsed string, err error) {
	tmpDir, mkErr := os.MkdirTemp("", "voxis-preprocess-*")
	if mkErr != nil {
		return "", "", fmt.Errorf("create decrypt temp dir: %w", mkErr)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup
	defer func() {
		if closeErr := encrypted.Close(); closeErr != nil {
			logger.Warn("close encrypted reader", "error", closeErr)
		}
	}()

	// Step 1: Decrypt to temp file.
	decryptedPath := filepath.Join(tmpDir, "decrypted"+filepath.Ext(media.Filename))
	decryptFile, createErr := os.Create(decryptedPath) //nolint:gosec // path from trusted temp dir
	if createErr != nil {
		return "", "", fmt.Errorf("create decrypted file: %w", createErr)
	}
	if openErr := w.envelope.OpenStream(ctx, orgID, encrypted, decryptFile, sealedMeta); openErr != nil {
		_ = decryptFile.Close() //nolint:errcheck // best-effort cleanup
		return "", "", wrapDecryptionError(openErr)
	}
	_ = decryptFile.Close() //nolint:errcheck // best-effort cleanup

	// Step 2: Run preprocessing pipeline.
	ppResult, ppErr := w.preprocessAudio(ctx, decryptedPath, logger)
	if ppErr != nil {
		return "", "", ppErr
	}
	defer os.RemoveAll(ppResult.tmpDir) //nolint:errcheck // best-effort cleanup

	// Step 3: Upload preprocessed file.
	f, openErr := os.Open(ppResult.outputPath) //nolint:gosec // path from trusted temp dir
	if openErr != nil {
		return "", "", fmt.Errorf("open preprocessed file: %w", openErr)
	}
	defer f.Close() //nolint:errcheck // best-effort cleanup

	audioURL, uploadErr := w.provider.Upload(ctx, f, "audio.wav", "audio/wav")
	if uploadErr != nil {
		return "", "", uploadErr
	}

	return audioURL, ppResult.preprocessorUsed, nil
}

// errDecryption is a sentinel used to identify decryption failures in
// the combined decrypt-strip-upload path.
var errDecryption = errors.New("decryption: ")

// isDecryptionError checks whether err originated from decryption.
func isDecryptionError(err error) bool {
	return errors.Is(err, errDecryption)
}

// wrapDecryptionError tags err with the errDecryption sentinel so
// classifyUploadFailure treats it as permanent (JobCancel) — EXCEPT context
// cancellation/deadline: that is a shutdown or timeout, not poisoned
// ciphertext, and must propagate unwrapped so the job stays retryable. The
// sentinel wrap stringifies the original chain, so it must never swallow
// context errors.
func wrapDecryptionError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w%s", errDecryption, err.Error())
}

// ffmpegAvailable reports whether the ffmpeg binary is on PATH.
func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// needsChunking reports whether media must be split before submission.
//
// Speechmatics is bound by duration OR plaintext size: its request body is capped at 1 GB (the
// adapter refuses past 900 MB, permanently), and a short high-bitrate source
// can exceed that without being long. Chunking re-encodes to 16 kHz mono WAV,
// so the size branch is what makes such a file transcribable at all.
func (w *TranscribeWorker) needsChunking(media *domain.Media) bool {
	if media.Duration > float64(w.chunkSeconds()) {
		return true
	}
	return media.Size > speechmaticsMaxSingleJobBytes
}

// chunkSeconds is the maximum chunk length for the provider bound to this job.
// It decides both whether media is long enough to chunk at all and how long
// each chunk may be, so the two can never disagree.
func (w *TranscribeWorker) chunkSeconds() int { return speechmaticsChunkSeconds }

// submitChunked handles long media by splitting into bounded-duration segments,
// submitting each segment to the bound provider, and persisting segment mappings.
func (w *TranscribeWorker) submitChunked(
	ctx context.Context,
	args TranscribeJobArgs,
	trans *domain.Transcription,
	media *domain.Media,
	logger *slog.Logger,
) error {
	if w.chunker == nil || w.prober == nil || w.segRepo == nil {
		w.failTranscription(ctx, trans, "long audio chunking is not configured", logger)
		return river.JobCancel(fmt.Errorf("chunking unavailable for long media %s", media.ID))
	}

	decryptedPath, err := w.fetchDecryptedSource(ctx, args, trans, media, logger)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(decryptedPath) }() //nolint:errcheck // best-effort cleanup

	sourcePath, sourceContentType, cleanup := w.prepareChunkInput(ctx, trans, media, decryptedPath, logger)
	defer cleanup()

	chunkDir, err := os.MkdirTemp("", "voxis-transcribe-chunks-*")
	if err != nil {
		return fmt.Errorf("create chunk temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(chunkDir) }() //nolint:errcheck // best-effort cleanup

	chunkPaths, err := w.chunkSource(ctx, trans, sourcePath, chunkDir, sourceContentType, logger)
	if err != nil {
		return err
	}

	if err := w.submitSegments(ctx, trans, chunkPaths, logger); err != nil {
		return err
	}

	if err := w.persistChunkedParentSubmission(ctx, trans); err != nil {
		return fmt.Errorf("update transcription status: %w", err)
	}

	return nil
}

// fetchDecryptedSource downloads the encrypted media object and decrypts it to
// a temp file. The caller must remove the returned path. Decryption failures are permanent (JobCancel),
// except context cancellation/deadline, which stays retryable.
func (w *TranscribeWorker) fetchDecryptedSource(
	ctx context.Context,
	args TranscribeJobArgs,
	trans *domain.Transcription,
	media *domain.Media,
	logger *slog.Logger,
) (string, error) {
	encrypted, err := w.storage.Download(ctx, media.StorageKey)
	if err != nil {
		return "", fmt.Errorf("download encrypted media: %w", err)
	}
	defer func() {
		if closeErr := encrypted.Close(); closeErr != nil {
			logger.Warn("close encrypted media reader", "error", closeErr)
		}
	}()

	sealedMeta := &crypto.SealedMeta{
		EncryptionMeta: crypto.EncryptionMeta{
			Algorithm:     media.EncryptionAlgo,
			ChunkSize:     media.ChunkSize,
			ChunkCount:    media.ChunkCount,
			PlaintextSize: media.Size,
		},
		WrappedDEK:    media.WrappedDEK,
		WrappingNonce: media.WrappingNonce,
	}

	decryptedPath, err := w.decryptToTempFile(ctx, args.OrgID, encrypted, sealedMeta)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Shutdown or timeout mid-decrypt, not poisoned ciphertext — leave
			// the job retryable instead of permanently failing the transcription.
			return "", fmt.Errorf("decrypt media: %w", err)
		}
		w.failTranscription(ctx, trans, "decryption failed", logger)
		return "", river.JobCancel(fmt.Errorf("decrypt media: %w", err))
	}
	return decryptedPath, nil
}

// prepareChunkInput optionally preprocesses (enhance) and strips metadata from
// the decrypted source, both fail-soft. It returns the path/content-type to
// chunk plus a cleanup callback for any intermediate files it created.
func (w *TranscribeWorker) prepareChunkInput(
	ctx context.Context,
	trans *domain.Transcription,
	media *domain.Media,
	decryptedPath string,
	logger *slog.Logger,
) (sourcePath, sourceContentType string, cleanup func()) {
	sourcePath = decryptedPath
	sourceContentType = media.ContentType
	cleanups := make([]func(), 0, 2)
	cleanup = func() {
		for _, fn := range cleanups {
			fn()
		}
	}

	// If enhance requested, try preprocessing the whole file before chunking.
	if trans.EnhanceAudio && w.defaultPreprocessor != nil && w.defaultPreprocessor.Available() {
		ppResult, ppErr := w.preprocessAudio(ctx, sourcePath, logger)
		if ppErr != nil {
			logger.Warn("preprocess failed for long audio, falling back to baseline chunking path", "error", ppErr)
		} else {
			tmpDir := ppResult.tmpDir
			cleanups = append(cleanups, func() { _ = os.RemoveAll(tmpDir) }) //nolint:errcheck // best-effort cleanup
			sourcePath = ppResult.outputPath
			sourceContentType = "audio/wav"
			trans.PreprocessorUsed = ppResult.preprocessorUsed
		}
	}

	// Strip metadata before chunking — only when preprocessing wasn't used, and
	// only when the chunks can inherit the source container. The Speechmatics
	// path re-encodes every chunk to a fresh PCM WAV, which carries no source
	// metadata, so a stripping pass over the whole file first buys nothing.
	skipStrip := trans.PreprocessorUsed != "" || true
	if !skipStrip && ffmpegAvailable() {
		strippedPath, stripErr := ffmpeg.StripMetadata(ctx, sourcePath, media.ContentType, w.ffmpegOptions()...)
		if stripErr != nil {
			logger.Warn("metadata stripping failed for long audio, proceeding with metadata",
				"error", stripErr)
		} else {
			_ = os.Remove(sourcePath) //nolint:errcheck // replaced by stripped version
			sourcePath = strippedPath
			cleanups = append(cleanups, func() { _ = os.Remove(strippedPath) }) //nolint:errcheck // best-effort cleanup
		}
	} else if !skipStrip {
		logger.Warn("ffmpeg not available, skipping metadata stripping for long audio")
	}
	return sourcePath, sourceContentType, cleanup
}

// chunkSource splits the source file into bounded-duration segments inside
// chunkDir, enforcing the maximum segment count.
func (w *TranscribeWorker) chunkSource(
	ctx context.Context,
	trans *domain.Transcription,
	sourcePath, chunkDir, sourceContentType string,
	logger *slog.Logger,
) ([]string, error) {
	chunkPaths, err := w.splitSource(ctx, sourcePath, chunkDir, sourceContentType)
	if err != nil {
		return nil, fmt.Errorf("split long audio: %w", err)
	}
	sort.Strings(chunkPaths)
	if len(chunkPaths) > maxChunkSegments {
		w.failTranscription(ctx, trans, "audio split produced too many chunks", logger)
		return nil, river.JobCancel(fmt.Errorf("chunk count %d exceeds max %d", len(chunkPaths), maxChunkSegments))
	}
	return chunkPaths, nil
}

// splitSource picks the chunk format for the bound provider.
//
// Speechmatics gets 16 kHz mono WAV — the input it documents as optimal.
// The generic split would otherwise hand it a lossy 128 kbit/s Opus re-encode
// inside a WebM container Speechmatics does not list as supported, which works
// the source-preserving split unchanged.
func (w *TranscribeWorker) splitSource(
	ctx context.Context,
	sourcePath, chunkDir, sourceContentType string,
) ([]string, error) {
	if true {
		return w.chunker.SplitByDurationWAV(ctx, sourcePath, chunkDir, w.chunkSeconds())
	}
	return w.chunker.SplitByDuration(ctx, sourcePath, chunkDir, w.chunkSeconds(), sourceContentType)
}

// submitSegments plans/ensures segment rows for each chunk and submits every
// segment that has not been submitted yet (idempotent across retries).
func (w *TranscribeWorker) submitSegments(
	ctx context.Context,
	trans *domain.Transcription,
	chunkPaths []string,
	logger *slog.Logger,
) error {
	planned, err := w.planSegments(ctx, trans.ID, chunkPaths)
	if err != nil {
		return fmt.Errorf("plan transcription segments: %w", err)
	}

	segments, err := w.ensureSegments(ctx, trans.ID, planned)
	if err != nil {
		return fmt.Errorf("ensure transcription segments: %w", err)
	}

	byIndex := make(map[int]*domain.TranscriptionSegment, len(segments))
	for _, seg := range segments {
		byIndex[seg.SegmentIndex] = seg
	}

	for i := 0; i < len(chunkPaths); i++ {
		seg, ok := byIndex[i]
		if !ok {
			return fmt.Errorf("missing segment row for index=%d", i)
		}
		if w.segmentSubmitted(seg) {
			continue
		}
		if err := w.submitOneSegment(ctx, trans, seg, chunkPaths[i], i, logger); err != nil {
			return err
		}
	}
	return nil
}

// submitOneSegment uploads one chunk and submits it to the provider, then
// persists the resulting provider ID on the segment row.
func (w *TranscribeWorker) submitOneSegment(
	ctx context.Context,
	trans *domain.Transcription,
	seg *domain.TranscriptionSegment,
	chunkPath string,
	index int,
	logger *slog.Logger,
) error {
	audioURL, uploadErr := w.uploadChunk(ctx, chunkPath, index)
	if uploadErr != nil {
		if errors.Is(uploadErr, domain.ErrUnauthorized) {
			w.failTranscription(ctx, trans, "provider authentication failed", logger)
			return river.JobCancel(fmt.Errorf("provider upload unauthorized: %w", uploadErr))
		}
		return fmt.Errorf("provider upload chunk[%d]: %w", index, uploadErr)
	}

	providerJobID, submitErr := w.submitToProvider(ctx, trans, audioURL, seg.ID, true)
	if submitErr != nil {
		// See submitAndClassify: an auth rejection here is permanent.
		if errors.Is(submitErr, domain.ErrUnauthorized) {
			w.failTranscription(ctx, trans, "provider authentication failed", logger)
			return river.JobCancel(fmt.Errorf("provider submit unauthorized (chunk %d): %w", index, submitErr))
		}
		if errors.Is(submitErr, domain.ErrInvalidInput) {
			w.failTranscription(ctx, trans, "Speechmatics"+" rejected request: "+submitErr.Error(), logger)
			return river.JobCancel(fmt.Errorf("%s submit bad request (chunk %d): %w", domain.TranscriptionProviderSpeechmatics, index, submitErr))
		}
		return fmt.Errorf("%s submit chunk[%d]: %w", domain.TranscriptionProviderSpeechmatics, index, submitErr)
	}

	if err := w.persistSegmentSubmission(ctx, seg, providerJobID); err != nil {
		return fmt.Errorf("update segment submission status[%d]: %w", index, err)
	}
	return nil
}

func (w *TranscribeWorker) decryptToTempFile(
	ctx context.Context,
	orgID string,
	encrypted io.ReadCloser,
	sealedMeta *crypto.SealedMeta,
) (string, error) {
	tempFile, err := os.CreateTemp("", "voxis-transcribe-source-*")
	if err != nil {
		return "", fmt.Errorf("create decrypted temp file: %w", err)
	}
	path := tempFile.Name()

	if openErr := w.envelope.OpenStream(ctx, orgID, encrypted, tempFile, sealedMeta); openErr != nil {
		_ = tempFile.Close() //nolint:errcheck // best-effort cleanup
		_ = os.Remove(path)  //nolint:errcheck // best-effort cleanup
		return "", openErr
	}
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(path) //nolint:errcheck // best-effort cleanup
		return "", fmt.Errorf("close decrypted temp file: %w", closeErr)
	}
	return path, nil
}

func (w *TranscribeWorker) uploadChunk(ctx context.Context, path string, index int) (string, error) {
	f, err := os.Open(path) //nolint:gosec // chunk path comes from trusted temp dir
	if err != nil {
		return "", fmt.Errorf("open chunk file: %w", err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // best-effort cleanup

	filename := filepath.Base(path)
	if filename == "" {
		filename = fmt.Sprintf("chunk-%03d.webm", index)
	}
	return w.provider.Upload(ctx, f, filename, chunkContentType(filepath.Ext(filename)))
}

// chunkContentType is the MIME type a chunk file is uploaded under.
//
// mime.TypeByExtension reads the host's MIME tables, which on Linux answer
// "audio/x-wav" for .wav (and nothing at all on a minimal container image), so
// the extensions we produce ourselves are mapped explicitly rather than left to
// the deployment.
func chunkContentType(ext string) string {
	if strings.EqualFold(ext, ".wav") {
		return "audio/wav"
	}
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func (w *TranscribeWorker) planSegments(ctx context.Context, transcriptionID string, chunkPaths []string) ([]*domain.TranscriptionSegment, error) {
	segments := make([]*domain.TranscriptionSegment, 0, len(chunkPaths))
	startOffset := 0.0

	for i := 0; i < len(chunkPaths); i++ {
		probeResult, err := w.prober.Probe(ctx, chunkPaths[i])
		if err != nil {
			return nil, fmt.Errorf("probe chunk[%d]: %w", i, err)
		}
		if probeResult.Duration <= 0 {
			return nil, fmt.Errorf("invalid chunk duration for chunk[%d]", i)
		}

		endOffset := startOffset + probeResult.Duration
		seg, err := domain.NewTranscriptionSegment(transcriptionID, i, startOffset, endOffset)
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
		startOffset = endOffset
	}

	return segments, nil
}

func (w *TranscribeWorker) ensureSegments(
	ctx context.Context,
	transcriptionID string,
	planned []*domain.TranscriptionSegment,
) ([]*domain.TranscriptionSegment, error) {
	existing, err := w.segRepo.ListByTranscriptionID(ctx, transcriptionID)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		if err := w.segRepo.CreateBatch(ctx, planned); err != nil {
			return nil, err
		}
		return w.segRepo.ListByTranscriptionID(ctx, transcriptionID)
	}
	if len(existing) != len(planned) {
		return nil, fmt.Errorf("segment count mismatch: existing=%d planned=%d", len(existing), len(planned))
	}
	return existing, nil
}

// pipeDecryptUpload decrypts the media stream and pipes it to the provider upload.
// Returns the audio URL, any decryption error, and any upload error.
func (w *TranscribeWorker) pipeDecryptUpload(
	ctx context.Context,
	orgID string,
	encrypted io.ReadCloser,
	media *domain.Media,
	sealedMeta *crypto.SealedMeta,
) (audioURL string, decryptErr, uploadErr error) {
	pr, pw := io.Pipe()
	decryptErrCh := make(chan error, 1)
	go func() {
		defer func() {
			if err := encrypted.Close(); err != nil {
				w.logger.Warn("close encrypted reader", "error", err)
			}
		}()
		dErr := w.envelope.OpenStream(ctx, orgID, encrypted, pw, sealedMeta)
		if dErr != nil {
			_ = pw.CloseWithError(dErr) //nolint:errcheck // pipe close cannot fail
			decryptErrCh <- dErr
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close cannot fail
		decryptErrCh <- nil
	}()

	audioURL, uploadErr = w.provider.Upload(ctx, pr, providerUploadName(media.Filename), media.ContentType)
	_ = pr.Close() //nolint:errcheck // pipe close cannot fail; ensures goroutine completes
	decryptErr = <-decryptErrCh

	return audioURL, decryptErr, uploadErr
}

// BuildTranscriptionRequest maps media transcription settings to Speechmatics.
// vocabularyEnabled carries the CUSTOM_VOCABULARY_ENABLED deployment flag.
// logger may be nil.
func BuildTranscriptionRequest(
	audioURL string,
	trans *domain.Transcription,
	diarization bool,
	callbackBaseURL, webhookSecret string,
	vocabularyEnabled bool,
	logger *slog.Logger,
) port.TranscriptionRequest {
	req := port.TranscriptionRequest{
		AudioURL:    audioURL,
		Diarization: diarization,
	}

	if len(trans.Languages) == 1 && trans.Languages[0] != "auto" {
		req.Languages = trans.Languages
	} else if len(trans.Languages) > 1 {
		req.Languages = trans.Languages
	}

	// A declared speaker count pins diarization to exactly that many speakers.
	// It is only meaningful when diarization is on, and diarization here is the
	// effective (possibly overridden) flag, not the persisted one — so a job
	// that ends up without diarization never carries a speaker range.
	//
	// This builds the FULL-FILE request. The chunked path must run the result
	// through SegmentTranscriptionRequest before submitting.
	if diarization && trans.ExpectedSpeakers > 0 {
		req.ExpectedSpeakers = trans.ExpectedSpeakers
	}

	req.Vocabulary = resolveVocabulary(trans, vocabularyEnabled, logger)

	return req
}

// resolveVocabulary turns the persisted pack ids into provider terms.
//
// enabled is the deployment flag. Rows created while the feature was on keep
// their pack ids forever, so the flag has to be honored here too — otherwise
// turning it off stops new selections but keeps sending terms for old rows.
//
// Resolution failure must NOT fail the job — transcribing without the
// terminology bias is strictly better than not transcribing at all — so a bad
// or retired pack id degrades to an empty list plus a warning.
func resolveVocabulary(trans *domain.Transcription, enabled bool, logger *slog.Logger) []port.VocabTerm {
	if len(trans.VocabularyPacks) == 0 {
		return nil
	}
	log := logger
	if log == nil {
		log = slog.Default()
	}
	if !enabled {
		log.Debug("custom vocabulary disabled, ignoring stored packs",
			"transcription_id", trans.ID,
			"packs", trans.VocabularyPacks,
		)
		return nil
	}
	terms, err := vocab.Terms(trans.VocabularyPacks)
	if err == nil {
		return terms
	}
	log.Warn("skipping custom vocabulary",
		"transcription_id", trans.ID,
		"packs", trans.VocabularyPacks,
		"error", err,
	)
	return nil
}

// submitWithVocabFallback submits req and, when the request carried custom
// vocabulary and the provider rejected it as a bad request, resubmits ONCE
// without vocabulary.
//
// The provider can answer 400 for a custom_vocabulary config it will not accept, and the
// adapter maps 400 to domain.ErrInvalidInput — a permanent failure that kills
// the job. Vocabulary is only a recognition bias, never required output, so
// dropping it and transcribing beats failing the whole transcription. A second
// rejection is returned unchanged and stays permanent.
func submitWithVocabFallback(
	ctx context.Context,
	provider port.TranscriptionProvider,
	req port.TranscriptionRequest,
	trans *domain.Transcription,
	logger *slog.Logger,
) (string, error) {
	providerJobID, err := provider.Submit(ctx, req)
	if err == nil || len(req.Vocabulary) == 0 || !errors.Is(err, domain.ErrInvalidInput) {
		return providerJobID, err
	}

	log := logger
	if log == nil {
		log = slog.Default()
	}
	log.Warn("provider rejected vocabulary config; resubmitting without vocabulary",
		"transcription_id", trans.ID,
		"packs", trans.VocabularyPacks,
		"error", err,
	)

	// req is a value copy, so clearing it here cannot leak back to the caller.
	req.Vocabulary = nil
	return provider.Submit(ctx, req)
}

// submitToProvider builds the request and submits to the transcription provider.
// reference is the Voxis row the job belongs to (transcription id, or segment id
// on the chunked path); providers that support job metadata echo it back.
// forSegment marks a chunked-path submission, which must not carry a per-segment
// speaker floor — see SegmentTranscriptionRequest.
func (w *TranscribeWorker) submitToProvider(
	ctx context.Context,
	trans *domain.Transcription,
	audioURL, reference string,
	forSegment bool,
) (string, error) {
	req := BuildTranscriptionRequest(
		audioURL, trans, trans.Diarization,
		w.callbackBaseURL, w.webhookSecret, w.customVocabulary, w.logger,
	)
	req.Reference = reference
	w.applySpeechmaticsCallback(&req)
	if req.ExpectedSpeakers > 0 {
		// Logged at submit time so a mis-diarized job can be diagnosed from the
		// logs alone — what was declared, and which provider got it.
		w.logger.Info("submitting transcription with a declared speaker count",
			"transcription_id", trans.ID,
			"provider", domain.TranscriptionProviderSpeechmatics,
			"expected_speakers", req.ExpectedSpeakers,
			"segment", forSegment,
			"reference", reference,
		)
	}
	return submitWithVocabFallback(ctx, w.provider, req, trans, w.logger)
}

// failTranscription marks a transcription as failed (best-effort).
func (w *TranscribeWorker) failTranscription(ctx context.Context, trans *domain.Transcription, msg string, logger *slog.Logger) {
	trans.SetFailed(msg)
	if err := w.transRepo.Update(ctx, trans); err != nil {
		logger.Warn("failed to update transcription status", "error", err)
	}
}
