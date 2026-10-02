package port

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// The two interfaces below are deliberately narrow and separate from
// TranscriptionRepository / TranscriptionSegmentRepository. Every Speechmatics
// query filters on speechmatics_job_id only, so the other provider paths and these can
// never select each other's rows; keeping them out of the existing interfaces
// also means no existing mock or fake has to grow methods it will never use.

// SpeechmaticsWaitJobInserter enqueues one bounded server-side wait on a
// Speechmatics job. The completion webhook is only a wake-up signal, so it
// carries no result of its own: it enqueues this job, which re-fetches the
// canonical status from Speechmatics and runs the poller's dispatch.
//
// Exactly one of transcriptionID and segmentID is set.
type SpeechmaticsWaitJobInserter interface {
	InsertSpeechmaticsWaitJob(ctx context.Context, transcriptionID, segmentID, jobID string) error
}

// SpeechmaticsTranscriptionRepository is the parent-row persistence the
// Speechmatics submission, poll and clean paths need.
type SpeechmaticsTranscriptionRepository interface {
	// MarkSubmittedToSpeechmatics stamps provider='speechmatics' and the job id
	// in the same write that moves the row to "submitted" (pin-at-submission).
	// jobID is empty for a chunked parent, whose segments carry the per-chunk
	// ids; an empty jobID leaves the column untouched.
	//
	// preprocessorUsed carries the audio-preprocessing pipeline that produced
	// the submitted audio. This write replaces the other provider path's full-row
	// Update, so it is the only chance to persist it; empty leaves the stored
	// value untouched rather than erasing it.
	//
	// Returns domain.ErrNotFound if the transcription does not exist.
	MarkSubmittedToSpeechmatics(ctx context.Context, transcriptionID, jobID, preprocessorUsed string) error

	// GetBySpeechmaticsJobID retrieves a transcription by its Speechmatics job id.
	// Returns domain.ErrNotFound if not found.
	GetBySpeechmaticsJobID(ctx context.Context, jobID string) (*domain.Transcription, error)

	// ListStaleSubmittedSpeechmatics returns Speechmatics-owned transcriptions
	// in "submitted" older than the given cutoff. Bounded to 50 rows.
	ListStaleSubmittedSpeechmatics(ctx context.Context, olderThan time.Time) ([]*domain.Transcription, error)

	// ListUndeletedFromSpeechmatics returns terminal Speechmatics transcriptions
	// whose provider-side job has not been confirmed deleted. Bounded to 50 rows.
	ListUndeletedFromSpeechmatics(ctx context.Context) ([]*domain.Transcription, error)

	// MarkSpeechmaticsDeleted records provider-side deletion.
	// Returns domain.ErrNotFound if the row does not exist or is already marked.
	MarkSpeechmaticsDeleted(ctx context.Context, id string) error
}

// SpeechmaticsSegmentRepository is the chunk-level twin of
// SpeechmaticsTranscriptionRepository.
type SpeechmaticsSegmentRepository interface {
	// MarkSegmentSubmittedToSpeechmatics records the per-chunk job id and moves
	// the segment to "submitted".
	MarkSegmentSubmittedToSpeechmatics(ctx context.Context, segmentID, jobID string) error

	// GetSegmentBySpeechmaticsJobID retrieves a segment by its Speechmatics job id.
	// Returns domain.ErrNotFound if not found.
	GetSegmentBySpeechmaticsJobID(ctx context.Context, jobID string) (*domain.TranscriptionSegment, error)

	// ListStaleSubmittedSpeechmaticsSegments returns submitted Speechmatics
	// segments older than the given cutoff. Bounded to 50 rows.
	ListStaleSubmittedSpeechmaticsSegments(ctx context.Context, olderThan time.Time) ([]*domain.TranscriptionSegment, error)

	// ListUndeletedSpeechmaticsSegments returns segments whose Speechmatics job
	// still needs deletion. Bounded to 50 rows.
	ListUndeletedSpeechmaticsSegments(ctx context.Context) ([]*domain.TranscriptionSegment, error)

	// MarkSegmentSpeechmaticsDeleted records provider-side deletion of a segment job.
	MarkSegmentSpeechmaticsDeleted(ctx context.Context, id string) error
}
