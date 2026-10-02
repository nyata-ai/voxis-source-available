package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// CleanSpeechmaticsJobArgs is the River job args for periodic Speechmatics
// data cleanup.
type CleanSpeechmaticsJobArgs struct{}

// Kind returns the unique job kind identifier.
func (CleanSpeechmaticsJobArgs) Kind() string { return "clean_speechmatics" }

// InsertOpts provides default insertion options.
func (CleanSpeechmaticsJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
	}
}

// cleanSpeechmaticsTimeout mirrors cleanother providerTimeout: up to 50 items, each
// with a 30-second per-item timeout, plus headroom.
const cleanSpeechmaticsTimeout = 30 * time.Minute

// CleanSpeechmaticsWorker is the Speechmatics twin of provider cleanup worker. It
// retries provider deletion for terminal Speechmatics transcriptions and
// segments whose first delete attempt failed.
//
// It stays registered whenever Speechmatics credentials are configured, even
// when TRANSCRIPTION_PROVIDER has been rolled back to other provider: the deletion
// backlog of already-submitted jobs must drain regardless of which provider
// new submissions go to.
type CleanSpeechmaticsWorker struct {
	river.WorkerDefaults[CleanSpeechmaticsJobArgs]
	transRepo port.SpeechmaticsTranscriptionRepository
	segRepo   port.SpeechmaticsSegmentRepository
	// parentLookup resolves a segment's parent to check cleanup eligibility.
	parentLookup port.TranscriptionRepository
	provider     port.TranscriptionProvider
	logger       *slog.Logger
}

// Timeout overrides River's default 60-second job timeout.
func (w *CleanSpeechmaticsWorker) Timeout(_ *river.Job[CleanSpeechmaticsJobArgs]) time.Duration {
	return cleanSpeechmaticsTimeout
}

// NewCleanSpeechmaticsWorker creates a CleanSpeechmaticsWorker.
func NewCleanSpeechmaticsWorker(
	transRepo port.SpeechmaticsTranscriptionRepository,
	segRepo port.SpeechmaticsSegmentRepository,
	parentLookup port.TranscriptionRepository,
	provider port.TranscriptionProvider,
	logger *slog.Logger,
) *CleanSpeechmaticsWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &CleanSpeechmaticsWorker{
		transRepo:    transRepo,
		segRepo:      segRepo,
		parentLookup: parentLookup,
		provider:     provider,
		logger:       logger,
	}
}

// Work deletes every undeleted Speechmatics transcription, then every
// undeleted segment. Always returns nil — the next cycle retries.
func (w *CleanSpeechmaticsWorker) Work(ctx context.Context, _ *river.Job[CleanSpeechmaticsJobArgs]) error {
	undeleted, err := w.transRepo.ListUndeletedFromSpeechmatics(ctx)
	if err != nil {
		w.logger.Error("failed to list undeleted Speechmatics transcriptions", "error", err)
		return nil
	}

	if len(undeleted) > 0 {
		w.logger.Info("cleaning up Speechmatics transcription data", "count", len(undeleted))
		for _, trans := range undeleted {
			w.cleanOne(ctx, trans)
		}
	}

	undeletedSegments, err := w.segRepo.ListUndeletedSpeechmaticsSegments(ctx)
	if err != nil {
		w.logger.Error("failed to list undeleted Speechmatics transcription segments", "error", err)
		return nil
	}

	if len(undeletedSegments) == 0 {
		return nil
	}

	w.logger.Info("cleaning up Speechmatics transcription segment data", "count", len(undeletedSegments))
	for _, seg := range undeletedSegments {
		w.cleanSegment(ctx, seg)
	}

	return nil
}

func (w *CleanSpeechmaticsWorker) cleanOne(ctx context.Context, trans *domain.Transcription) {
	logger := w.logger.With(
		"transcription_id", trans.ID,
		"speechmatics_job_id", trans.SpeechmaticsJobID,
	)

	ctx, cancel := context.WithTimeout(ctx, speechmaticsDeleteTimeout)
	defer cancel()

	if err := w.provider.Delete(ctx, trans.SpeechmaticsJobID); err != nil {
		logger.Warn("failed to delete transcription from Speechmatics (will retry next cycle)", "error", err)
		return
	}

	if err := w.transRepo.MarkSpeechmaticsDeleted(ctx, trans.ID); err != nil {
		logger.Error("failed to mark transcription as deleted from Speechmatics", "error", err)
		return
	}

	logger.Info("cleaned up transcription data from Speechmatics")
}

func (w *CleanSpeechmaticsWorker) cleanSegment(ctx context.Context, seg *domain.TranscriptionSegment) {
	logger := w.logger.With(
		"transcription_id", seg.TranscriptionID,
		"segment_id", seg.ID,
		"segment_index", seg.SegmentIndex,
		"speechmatics_job_id", seg.SpeechmaticsJobID,
	)

	if !w.segmentEligible(ctx, seg, logger) {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, speechmaticsDeleteTimeout)
	defer cancel()

	if err := w.provider.Delete(ctx, seg.SpeechmaticsJobID); err != nil {
		logger.Warn("failed to delete transcription segment from Speechmatics (will retry next cycle)", "error", err)
		return
	}

	if err := w.segRepo.MarkSegmentSpeechmaticsDeleted(ctx, seg.ID); err != nil {
		logger.Error("failed to mark transcription segment as deleted from Speechmatics", "error", err)
		return
	}

	logger.Info("cleaned up transcription segment data from Speechmatics")
}

// segmentEligible reports whether a segment's provider job may be deleted: the
// segment itself is terminal, or its parent is. Mirrors provider cleanup worker.
func (w *CleanSpeechmaticsWorker) segmentEligible(
	ctx context.Context,
	seg *domain.TranscriptionSegment,
	logger *slog.Logger,
) bool {
	if seg.Status == domain.TranscriptionSegmentStatusCompleted ||
		seg.Status == domain.TranscriptionSegmentStatusFailed {
		return true
	}
	if w.parentLookup == nil {
		return false
	}

	parent, err := w.parentLookup.GetByID(ctx, seg.TranscriptionID)
	if err != nil {
		logger.Warn("failed to load parent transcription for segment cleanup eligibility", "error", err)
		return false
	}
	parentTerminal := parent.Status == domain.TranscriptionStatusCompleted ||
		parent.Status == domain.TranscriptionStatusFailed ||
		parent.Status == domain.TranscriptionStatusDeleted
	if !parentTerminal {
		logger.Debug("skipping segment cleanup; parent transcription still active",
			"parent_status", parent.Status, "segment_status", seg.Status)
	}
	return parentTerminal
}
