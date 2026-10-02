package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

// StitchRecordingJobArgs holds the arguments for a recording stitch job.
type StitchRecordingJobArgs struct {
	SessionID string `json:"session_id"`
}

// Kind returns the unique job kind identifier.
func (StitchRecordingJobArgs) Kind() string { return "stitch_recording" }

const stitchMaxAttempts = 3

// defaultStitchJobTimeout covers the 10-minute pre-ffmpeg scan, the 50-minute
// service stitch budget, and 30 minutes for bounded decrypt/upload/probe work.
const defaultStitchJobTimeout = 90 * time.Minute

// InsertOpts provides default insertion options for stitch jobs.
func (StitchRecordingJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "stitching",
		MaxAttempts: stitchMaxAttempts,
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

// recordingStitcher is the slice of RecordingService the worker needs.
// An interface so tests can exercise Work's error handling without
// assembling the full stitch pipeline.
type recordingStitcher interface {
	Stitch(ctx context.Context, sessionID string) (string, error)
	MarkFailed(ctx context.Context, sessionID string) error
}

// StitchRecordingWorker processes recording stitch jobs.
type StitchRecordingWorker struct {
	river.WorkerDefaults[StitchRecordingJobArgs]
	recordingSvc recordingStitcher
	privilegeSvc *service.PrivilegeService // optional; nil if privilege not wired
	logger       *slog.Logger
	timeout      time.Duration
}

// SetPrivilegeService wires the PrivilegeService for the OnRecordingStitched hook.
func (w *StitchRecordingWorker) SetPrivilegeService(p *service.PrivilegeService) {
	w.privilegeSvc = p
}

// StitchRecordingWorkerOption configures StitchRecordingWorker.
type StitchRecordingWorkerOption func(*StitchRecordingWorker)

// WithStitchJobTimeout overrides the default stitch job timeout.
func WithStitchJobTimeout(timeout time.Duration) StitchRecordingWorkerOption {
	return func(w *StitchRecordingWorker) {
		if timeout > 0 {
			w.timeout = timeout
		}
	}
}

// Timeout overrides River's default 60-second job timeout.
func (w *StitchRecordingWorker) Timeout(_ *river.Job[StitchRecordingJobArgs]) time.Duration {
	return w.timeout
}

// NewStitchRecordingWorker creates a new StitchRecordingWorker.
func NewStitchRecordingWorker(
	recordingSvc recordingStitcher,
	logger *slog.Logger,
	opts ...StitchRecordingWorkerOption,
) *StitchRecordingWorker {
	if logger == nil {
		logger = slog.Default()
	}
	worker := &StitchRecordingWorker{
		recordingSvc: recordingSvc,
		logger:       logger,
		timeout:      defaultStitchJobTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(worker)
		}
	}
	return worker
}

// Work stitches a recording session into a Media entity.
func (w *StitchRecordingWorker) Work(ctx context.Context, job *river.Job[StitchRecordingJobArgs]) error {
	sessionID := job.Args.SessionID

	w.logger.Info("starting recording stitch",
		"session_id", sessionID,
		"attempt", job.Attempt,
	)

	mediaID, err := w.recordingSvc.Stitch(ctx, sessionID)
	if err != nil {
		// A too-short or too-long recording stitches to the same rejected file
		// on every attempt — fail the session now and cancel instead of
		// burning retries.
		if errors.Is(err, domain.ErrRecordingTooShort) || errors.Is(err, domain.ErrMediaTooLong) {
			if failErr := w.recordingSvc.MarkFailed(ctx, sessionID); failErr != nil {
				w.logger.Error("failed to mark rejected recording as failed",
					"session_id", sessionID,
					"error", failErr,
				)
			}
			w.logger.Warn("recording rejected by stitch validation, canceling job",
				"session_id", sessionID,
				"error", err,
			)
			return river.JobCancel(fmt.Errorf("stitch session %s: %w", sessionID, err))
		}
		if job.Attempt >= stitchMaxAttempts {
			if failErr := w.recordingSvc.MarkFailed(ctx, sessionID); failErr != nil {
				w.logger.Error("failed to mark recording as failed after retries exhausted",
					"session_id", sessionID,
					"error", failErr,
				)
			}
		}
		w.logger.Error("recording stitch failed",
			"session_id", sessionID,
			"attempt", job.Attempt,
			"error", err,
		)
		return fmt.Errorf("stitch session %s: %w", sessionID, err)
	}

	w.logger.Info("recording stitch completed",
		"session_id", sessionID,
		"media_id", mediaID,
	)

	// Privilege lifecycle hook (Finding #3): for privilege media, auto-enqueue
	// the transcribe job. No-op for regular media. Failure here is logged but
	// non-fatal — the stitch already succeeded; recovery is via manual retry.
	if w.privilegeSvc != nil {
		if hookErr := w.privilegeSvc.OnRecordingStitchedByMediaID(ctx, mediaID); hookErr != nil {
			w.logger.Error("privilege OnRecordingStitched failed",
				"media_id", mediaID, "error", hookErr)
		}
	}
	return nil
}
