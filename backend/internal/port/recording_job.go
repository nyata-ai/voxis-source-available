package port

import "context"

// RecordingJobInserter enqueues recording-related background jobs.
type RecordingJobInserter interface {
	// InsertStitchJob enqueues a stitch job for the given recording session.
	InsertStitchJob(ctx context.Context, sessionID string) error
}
