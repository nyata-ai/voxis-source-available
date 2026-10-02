package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// speechmaticsWaitDuration is how long the provider is asked to hold the
// connection open waiting for a terminal state. Melia finishes most files in
// 1-5 seconds, so one bounded wait right after submission usually completes the
// job in seconds instead of at the next periodic poll.
const speechmaticsWaitDuration = 30 * time.Second

// speechmaticsWaitTimeout bounds the whole job: the server-side wait plus the
// transcript fetch and the local processing that follows it.
const speechmaticsWaitTimeout = 90 * time.Second

// SpeechmaticsWaitJobArgs asks for one bounded server-side wait on a
// just-submitted Speechmatics job. Exactly one of the two ids is set.
type SpeechmaticsWaitJobArgs struct {
	// TranscriptionID is set for a whole-file (parent) submission.
	TranscriptionID string `json:"transcription_id,omitempty"`
	// SegmentID is set for one chunk of a chunked submission.
	SegmentID string `json:"segment_id,omitempty"`
	// JobID is the Speechmatics job id, which is also how the row is looked
	// up: the Speechmatics repositories index on it, so no extra query is
	// needed. The id above is kept to assert the row that comes back is the
	// one this job was enqueued for.
	JobID string `json:"job_id"`
}

// Kind returns the unique job kind identifier.
func (SpeechmaticsWaitJobArgs) Kind() string { return "speechmatics_wait" }

// InsertOpts provides default insertion options. MaxAttempts=1 because this job
// is a pure optimization: the periodic poller is the guaranteed completion path
// and retrying a failed wait buys nothing it does not already cover.
//
// It runs on the default queue (20 workers). A heavily chunked submission
// enqueues one wait per segment, each holding a slot for up to the wait
// duration, so a long chunked job can briefly saturate that queue. Nothing is
// lost when it does — the other default-queue jobs are periodic sweeps that
// simply run a cycle later — but a deployment that chunks routinely should
// give this kind its own queue.
func (SpeechmaticsWaitJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
	}
}

// SpeechmaticsWaitWorker completes a freshly submitted Speechmatics job without
// waiting for the next poll cycle, using the provider's server-side
// GET /v2/jobs/{id}?wait=N.
//
// It is BEST EFFORT by design. Every failure — a missing row, an unsupported
// provider, a network error, a job still running when the wait expires — is
// logged and swallowed, and PollSpeechmaticsWorker picks the row up on its
// normal cadence. Work therefore never returns an error.
type SpeechmaticsWaitWorker struct {
	river.WorkerDefaults[SpeechmaticsWaitJobArgs]
	speechmaticsDeps
}

// NewSpeechmaticsWaitWorker creates a SpeechmaticsWaitWorker. It takes the same
// dependencies as NewPollSpeechmaticsWorker because it runs the same dispatch.
func NewSpeechmaticsWaitWorker(
	transRepo port.SpeechmaticsTranscriptionRepository,
	segRepo port.SpeechmaticsSegmentRepository,
	transSvc *service.TranscriptionService,
	segSvc *service.SegmentTranscriptionService,
	provider port.TranscriptionProvider,
	logger *slog.Logger,
) *SpeechmaticsWaitWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &SpeechmaticsWaitWorker{
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

// speechmaticsResultWaiter is the optional adapter capability this worker needs.
// It is NOT part of port.TranscriptionProvider — the server-side wait is a
// Speechmatics-only optimization — so the worker discovers it by assertion and
// does nothing when the bound provider lacks it.
type speechmaticsResultWaiter interface {
	WaitForResult(ctx context.Context, jobID string, maxWait time.Duration) (*port.TranscriptionResult, error)
}

// Timeout bounds one wait job.
func (w *SpeechmaticsWaitWorker) Timeout(_ *river.Job[SpeechmaticsWaitJobArgs]) time.Duration {
	return speechmaticsWaitTimeout
}

// Work waits once for the submitted job and dispatches whatever came back
// through the same code the poller uses. Always returns nil.
func (w *SpeechmaticsWaitWorker) Work(ctx context.Context, job *river.Job[SpeechmaticsWaitJobArgs]) error {
	args := job.Args
	if args.JobID == "" {
		w.logger.Warn("speechmatics wait job carried no provider job id")
		return nil
	}

	waiter, ok := w.provider.(speechmaticsResultWaiter)
	if !ok {
		w.logger.Debug("transcription provider does not support a server-side wait; leaving the job to the poller",
			"speechmatics_job_id", args.JobID)
		return nil
	}

	if args.SegmentID != "" {
		w.waitSegment(ctx, waiter, args)
		return nil
	}
	w.waitParent(ctx, waiter, args)
	return nil
}

// waitParent handles a whole-file submission.
func (w *SpeechmaticsWaitWorker) waitParent(
	ctx context.Context, waiter speechmaticsResultWaiter, args SpeechmaticsWaitJobArgs,
) {
	logger := w.logger.With(
		"transcription_id", args.TranscriptionID,
		"speechmatics_job_id", args.JobID,
	)

	trans, err := w.transRepo.GetBySpeechmaticsJobID(ctx, args.JobID)
	if err != nil {
		logger.Debug("speechmatics wait: transcription lookup failed; leaving it to the poller", "error", err)
		return
	}
	// A row that moved on since the wait was enqueued (a webhook, an earlier
	// poll, a failure) must not be reprocessed.
	if trans.ID != args.TranscriptionID || trans.Status != domain.TranscriptionStatusSubmitted {
		logger.Debug("speechmatics wait: transcription is no longer awaiting a result",
			"status", trans.Status)
		return
	}

	result, err := waiter.WaitForResult(ctx, args.JobID, speechmaticsWaitDuration)
	if err != nil {
		logger.Info("speechmatics wait failed; leaving it to the poller", "error", err)
		return
	}

	w.dispatchParentResult(ctx, logger, trans, result, completionSourceWait)
}

// waitSegment handles one chunk of a chunked submission.
func (w *SpeechmaticsWaitWorker) waitSegment(
	ctx context.Context, waiter speechmaticsResultWaiter, args SpeechmaticsWaitJobArgs,
) {
	logger := w.logger.With(
		"segment_id", args.SegmentID,
		"speechmatics_job_id", args.JobID,
	)

	seg, err := w.segRepo.GetSegmentBySpeechmaticsJobID(ctx, args.JobID)
	if err != nil {
		logger.Debug("speechmatics wait: segment lookup failed; leaving it to the poller", "error", err)
		return
	}
	if seg.ID != args.SegmentID || seg.Status != domain.TranscriptionSegmentStatusSubmitted {
		logger.Debug("speechmatics wait: segment is no longer awaiting a result", "status", seg.Status)
		return
	}

	result, err := waiter.WaitForResult(ctx, args.JobID, speechmaticsWaitDuration)
	if err != nil {
		logger.Info("speechmatics segment wait failed; leaving it to the poller", "error", err)
		return
	}

	w.dispatchSegmentResult(ctx, logger, seg, result, completionSourceWait)
}
