package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/voxis/backend/internal/service"
)

// MaintenanceQueueName is the queue for periodic housekeeping sweeps.
//
// Sweeps are short but must run on time, so they get their own queue: sharing
// the "stitching" queue put them behind up-to-90-minute stitch jobs on a
// 2-worker queue, where a busy node could starve the sweep for hours.
const MaintenanceQueueName = "maintenance"

// CleanupRecordingJobArgs is the River job args for periodic recording cleanup.
type CleanupRecordingJobArgs struct{}

// Kind returns the unique job kind identifier.
func (CleanupRecordingJobArgs) Kind() string { return "cleanup_recording" }

// InsertOpts provides default insertion options.
// UniqueOpts limits the kind to one non-finished job at a time so a slow sweep
// cannot stack with the next 5-minute periodic insert (overlapping sweeps race
// on the same candidates and fail spuriously). Completed is excluded so the
// next periodic tick can insert as soon as the previous run finishes.
func (CleanupRecordingJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       MaintenanceQueueName,
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

// cleanupRecordingTimeout is the maximum time the cleanup job is allowed to run.
const cleanupRecordingTimeout = 5 * time.Minute

// CleanupRecordingWorker periodically detects orphaned recording sessions,
// resolves sessions stranded in 'completing', and enforces retention policy on
// abandoned/failed sessions.
type CleanupRecordingWorker struct {
	river.WorkerDefaults[CleanupRecordingJobArgs]
	recordingSvc *service.RecordingService
	logger       *slog.Logger
}

// Timeout overrides River's default 60-second job timeout.
func (w *CleanupRecordingWorker) Timeout(_ *river.Job[CleanupRecordingJobArgs]) time.Duration {
	return cleanupRecordingTimeout
}

// NewCleanupRecordingWorker creates a new CleanupRecordingWorker.
func NewCleanupRecordingWorker(
	recordingSvc *service.RecordingService,
	logger *slog.Logger,
) *CleanupRecordingWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &CleanupRecordingWorker{
		recordingSvc: recordingSvc,
		logger:       logger,
	}
}

// Work detects orphaned sessions and enforces retention policy.
func (w *CleanupRecordingWorker) Work(ctx context.Context, _ *river.Job[CleanupRecordingJobArgs]) error {
	if err := w.recordingSvc.DetectOrphans(ctx); err != nil {
		w.logger.Error("failed to detect orphaned recordings", "error", err)
	}
	if err := w.recordingSvc.SweepStuckCompleting(ctx); err != nil {
		w.logger.Error("failed to resolve stuck completing recordings", "error", err)
	}
	if err := w.recordingSvc.CleanupExpired(ctx); err != nil {
		w.logger.Error("failed to cleanup expired recording artifacts", "error", err)
	}
	retentionResult, err := w.recordingSvc.CleanupRetainedAudio(ctx)
	w.logger.Info("retention sweep completed",
		"candidates_scanned", retentionResult.CandidatesScanned,
		"deleted", retentionResult.Deleted,
		"already_missing", retentionResult.AlreadyMissing,
		"already_processed", retentionResult.AlreadyProcessed,
		"delete_failures", retentionResult.DeleteFailures,
		"mark_failures", retentionResult.MarkFailures,
		"oldest_due_at", retentionResult.OldestDueAt,
	)
	if err != nil {
		w.logger.Error("retention sweep failed", "error", err)
		return err
	}
	return nil
}
