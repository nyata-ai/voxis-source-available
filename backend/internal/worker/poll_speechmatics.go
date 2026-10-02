package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// speechmaticsDeleteTimeout bounds one provider deletion so a slow API cannot
// stall the poll cycle.
const speechmaticsDeleteTimeout = 30 * time.Second

// speechmaticsStaleCutoff is the Speechmatics twin of staleCutoff, deliberately
// far shorter.
//
// Melia compute is 1-2 seconds per file, so waiting 2 minutes before the first poll
// turns a 30-second clip into 2-3 minutes of wall clock, almost all of it idle
// waiting. 15s is short enough to catch most jobs on the first poll after this
// worker's periodic interval (20s, see main.go) without turning every cycle
// into a GET storm: the client-side limiter is 25 GET/s, and one cycle walks at
// most 100 rows (50 parents + 50 segments), so even a full cycle of stale work
// stays under 5 GET/s against that budget.
const speechmaticsStaleCutoff = 15 * time.Second

// pollSpeechmaticsTimeout overrides River's 60-second default. One cycle walks
// up to 50 parents plus 50 segments, and a single Speechmatics transcript fetch
// can take minutes on a long recording; the default would kill the job
// mid-cycle and leave rows stuck in submitted. Mirrors cleanSpeechmaticsTimeout.
const pollSpeechmaticsTimeout = 30 * time.Minute

// PollSpeechmaticsJobArgs holds the arguments for the poll-speechmatics
// periodic job. Zero-arg: it discovers its own work.
type PollSpeechmaticsJobArgs struct{}

// Kind returns the unique job kind identifier.
func (PollSpeechmaticsJobArgs) Kind() string { return "poll_speechmatics" }

// InsertOpts provides default insertion options. MaxAttempts=1 because this is
// a periodic job — a new instance is scheduled at the next interval.
// UniqueOpts limits the kind to one non-finished job at a time so a slow cycle
// cannot stack with the next periodic insert (at a 20s interval and a 30-minute
// job timeout, overlapping cycles would poll the same stale rows and duplicate
// provider calls). Completed is excluded so the next tick can insert as soon as
// the previous run finishes.
func (PollSpeechmaticsJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByState: []rivertype.JobState{
				rivertype.JobStatePending,
				rivertype.JobStateAvailable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// PollSpeechmaticsWorker polls
// Speechmatics-owned transcriptions and segments that have been "submitted"
// longer than speechmaticsStaleCutoff, processes their results through the
// same services, and deletes the provider-side job afterwards.
//
// It selects work purely by the speechmatics_* columns, so the two pollers can
// run side by side without ever seeing each other's rows. That is what makes a
// TRANSCRIPTION_PROVIDER flip safe with jobs in flight.
//
// Individual failures are logged, never returned: the next periodic run retries.
type PollSpeechmaticsWorker struct {
	river.WorkerDefaults[PollSpeechmaticsJobArgs]
	speechmaticsDeps
}

// speechmaticsDeps is the dependency set shared by every worker that turns a
// Speechmatics provider result into local state. Embedded rather than passed so
// PollSpeechmaticsWorker and SpeechmaticsWaitWorker run the SAME dispatch code:
// the wait job is only an optimization, and any divergence between the two
// paths would be a correctness bug that only shows up on one of them.
type speechmaticsDeps struct {
	transRepo port.SpeechmaticsTranscriptionRepository
	segRepo   port.SpeechmaticsSegmentRepository
	transSvc  *service.TranscriptionService
	segSvc    *service.SegmentTranscriptionService
	provider  port.TranscriptionProvider
	logger    *slog.Logger
}

// completionSource labels which path completed a job, so the logs distinguish
// the post-submit wait from the periodic poller.
const (
	completionSourcePoll = "poll"
	completionSourceWait = "wait"
)

// NewPollSpeechmaticsWorker creates a PollSpeechmaticsWorker.
func NewPollSpeechmaticsWorker(
	transRepo port.SpeechmaticsTranscriptionRepository,
	segRepo port.SpeechmaticsSegmentRepository,
	transSvc *service.TranscriptionService,
	segSvc *service.SegmentTranscriptionService,
	provider port.TranscriptionProvider,
	logger *slog.Logger,
) *PollSpeechmaticsWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &PollSpeechmaticsWorker{
		speechmaticsDeps: speechmaticsDeps{
			transRepo: transRepo,
			segRepo:   segRepo,
			transSvc:  transSvc,
			segSvc:    segSvc,
			provider:  provider,
			logger:    logger,
		},
	}
}

// Timeout overrides River's default 60-second job timeout.
func (w *PollSpeechmaticsWorker) Timeout(_ *river.Job[PollSpeechmaticsJobArgs]) time.Duration {
	return pollSpeechmaticsTimeout
}

// Work polls stale Speechmatics parents, then stale Speechmatics segments.
// Always returns nil.
func (w *PollSpeechmaticsWorker) Work(ctx context.Context, _ *river.Job[PollSpeechmaticsJobArgs]) error {
	cutoff := time.Now().Add(-speechmaticsStaleCutoff)

	stale, err := w.transRepo.ListStaleSubmittedSpeechmatics(ctx, cutoff)
	if err != nil {
		w.logger.Error("failed to list stale submitted Speechmatics transcriptions", "error", err)
		return nil // non-fatal: next periodic run retries
	}
	if len(stale) > 0 {
		w.logger.Info("polling stale submitted Speechmatics transcriptions", "count", len(stale))
		for _, trans := range stale {
			w.pollOne(ctx, trans)
		}
	}

	staleSegments, err := w.segRepo.ListStaleSubmittedSpeechmaticsSegments(ctx, cutoff)
	if err != nil {
		w.logger.Error("failed to list stale submitted Speechmatics segments", "error", err)
		return nil
	}
	if len(staleSegments) == 0 {
		return nil
	}

	w.logger.Info("polling stale submitted Speechmatics segments", "count", len(staleSegments))
	for _, seg := range staleSegments {
		w.pollSegmentOne(ctx, seg)
	}

	return nil
}

// pollOne polls one parent transcription and dispatches on the returned status.
func (w *PollSpeechmaticsWorker) pollOne(ctx context.Context, trans *domain.Transcription) {
	logger := w.logger.With(
		"transcription_id", trans.ID,
		"speechmatics_job_id", trans.SpeechmaticsJobID,
	)

	result, err := w.provider.GetStatus(ctx, trans.SpeechmaticsJobID)
	if err != nil {
		logger.Warn("failed to poll Speechmatics status", "error", err)
		return
	}

	w.dispatchParentResult(ctx, logger, trans, result, completionSourcePoll)
}

// dispatchParentResult turns one provider result for a parent transcription
// into local state. Shared by the poller and the post-submit wait job.
func (w speechmaticsDeps) dispatchParentResult(
	ctx context.Context,
	logger *slog.Logger,
	trans *domain.Transcription,
	result *port.TranscriptionResult,
	source string,
) {
	switch result.Status {
	case "done":
		if err := w.transSvc.ProcessResult(ctx, trans.ID, result); err != nil {
			logger.Error("failed to process transcription result", "error", err)
			return // Do NOT delete — data not saved locally yet.
		}
		logger.Info("transcription result processed successfully", "completion_source", source)
		w.deleteFromSpeechmatics(ctx, logger, trans.ID, trans.SpeechmaticsJobID)

	case "error":
		errMsg := result.ErrorMessage
		if errMsg == "" {
			errMsg = "unknown provider error"
		}
		if err := w.transSvc.HandleFailure(ctx, trans.ID, errMsg); err != nil {
			logger.Error("failed to handle transcription failure", "error", err)
			return // Do NOT delete — failure not recorded locally yet.
		}
		logger.Warn("transcription marked as failed", "error_message", errMsg)
		w.deleteFromSpeechmatics(ctx, logger, trans.ID, trans.SpeechmaticsJobID)

	case "processing", "queued":
		logger.Debug("transcription still in progress", "provider_status", result.Status, "stale_age_sec", staleAgeSeconds(trans.UpdatedAt))

	default:
		logger.Warn("unknown provider status", "provider_status", result.Status)
	}
}

// pollSegmentOne polls one chunk job. The segment row is already loaded, so it
// goes through the provider-neutral service entry points.
func (w *PollSpeechmaticsWorker) pollSegmentOne(ctx context.Context, seg *domain.TranscriptionSegment) {
	logger := w.logger.With(
		"transcription_id", seg.TranscriptionID,
		"segment_id", seg.ID,
		"segment_index", seg.SegmentIndex,
		"speechmatics_job_id", seg.SpeechmaticsJobID,
	)

	result, err := w.provider.GetStatus(ctx, seg.SpeechmaticsJobID)
	if err != nil {
		logger.Warn("failed to poll Speechmatics segment status", "error", err)
		return
	}

	w.dispatchSegmentResult(ctx, logger, seg, result, completionSourcePoll)
}

// dispatchSegmentResult turns one provider result for a chunk segment into
// local state. Shared by the poller and the post-submit wait job.
func (w speechmaticsDeps) dispatchSegmentResult(
	ctx context.Context,
	logger *slog.Logger,
	seg *domain.TranscriptionSegment,
	result *port.TranscriptionResult,
	source string,
) {
	switch result.Status {
	case "done":
		updatedSeg, processErr := w.segSvc.ProcessSegmentResult(ctx, seg, result)
		if processErr != nil {
			logger.Error("failed to process segment result", "error", processErr)
			return
		}
		logger.Info("segment result recovered via polling",
			"completion_source", source,
			"stale_age_sec", staleAgeSeconds(seg.UpdatedAt),
		)
		w.deleteSegmentFromSpeechmatics(ctx, logger, updatedSeg.ID, updatedSeg.SpeechmaticsJobID)

	case "error":
		errMsg := result.ErrorMessage
		if errMsg == "" {
			errMsg = "unknown provider error"
		}
		updatedSeg, processErr := w.segSvc.ProcessSegmentFailureFor(ctx, seg, errMsg)
		if processErr != nil {
			logger.Error("failed to process segment failure", "error", processErr)
			return
		}
		logger.Warn("segment failure recovered via polling",
			"completion_source", source,
			"stale_age_sec", staleAgeSeconds(seg.UpdatedAt),
			"error_message", errMsg,
		)
		w.deleteSegmentFromSpeechmatics(ctx, logger, updatedSeg.ID, updatedSeg.SpeechmaticsJobID)

	case "processing", "queued":
		logger.Debug("segment still in progress", "provider_status", result.Status)

	default:
		logger.Warn("unknown segment provider status", "provider_status", result.Status)
	}
}

// staleAgeSeconds is the non-negative age of a row in seconds.
func staleAgeSeconds(updatedAt time.Time) float64 {
	age := time.Since(updatedAt).Seconds()
	if age < 0 {
		return 0
	}
	return age
}

// deleteFromSpeechmatics removes audio and transcript data from the provider.
// Failures never block the transcription flow — CleanSpeechmaticsWorker retries.
func (w speechmaticsDeps) deleteFromSpeechmatics(ctx context.Context, logger *slog.Logger, transcriptionID, jobID string) {
	ctx, cancel := context.WithTimeout(ctx, speechmaticsDeleteTimeout)
	defer cancel()

	if err := w.provider.Delete(ctx, jobID); err != nil {
		logger.Warn("failed to delete transcription from Speechmatics (cleanup worker will retry)", "error", err)
		return
	}

	if err := w.transRepo.MarkSpeechmaticsDeleted(ctx, transcriptionID); err != nil {
		logger.Error("failed to mark Speechmatics deletion", "error", err)
		return
	}

	logger.Info("deleted transcription data from Speechmatics")
}

func (w speechmaticsDeps) deleteSegmentFromSpeechmatics(ctx context.Context, logger *slog.Logger, segmentID, jobID string) {
	ctx, cancel := context.WithTimeout(ctx, speechmaticsDeleteTimeout)
	defer cancel()

	if err := w.provider.Delete(ctx, jobID); err != nil {
		logger.Warn("failed to delete segment from Speechmatics (cleanup worker will retry)", "error", err)
		return
	}

	if err := w.segRepo.MarkSegmentSpeechmaticsDeleted(ctx, segmentID); err != nil {
		logger.Error("failed to mark segment Speechmatics deletion", "error", err)
		return
	}

	logger.Info("deleted segment data from Speechmatics")
}
