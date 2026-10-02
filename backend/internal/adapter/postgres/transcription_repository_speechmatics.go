package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Speechmatics persistence for the OSS transcription repository. Parent rows use sqlc; segments use a narrow raw projection.

var _ port.SpeechmaticsTranscriptionRepository = (*TranscriptionRepository)(nil)
var _ port.SpeechmaticsSegmentRepository = (*TranscriptionRepository)(nil)

// Every segment method uses the single retained Speechmatics projection.

// MarkSubmittedToSpeechmatics stamps provider + job id + status in one write.
func (r *TranscriptionRepository) MarkSubmittedToSpeechmatics(ctx context.Context, transcriptionID, jobID, preprocessorUsed string) error {
	uuid, err := parseUUID(transcriptionID)
	if err != nil {
		return domain.ErrNotFound
	}

	_, err = r.queries.MarkTranscriptionSubmittedToSpeechmatics(ctx, sqlcdb.MarkTranscriptionSubmittedToSpeechmaticsParams{
		ID:                uuid,
		Status:            domain.TranscriptionStatusSubmitted,
		SpeechmaticsJobID: pgtype.Text{String: jobID, Valid: jobID != ""},
		PreprocessorUsed:  pgtype.Text{String: preprocessorUsed, Valid: preprocessorUsed != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// GetBySpeechmaticsJobID retrieves a transcription by its Speechmatics job id.
func (r *TranscriptionRepository) GetBySpeechmaticsJobID(ctx context.Context, jobID string) (*domain.Transcription, error) {
	if jobID == "" {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetTranscriptionBySpeechmaticsJobID(ctx, pgtype.Text{String: jobID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return transcriptionRowToDomain(row), nil
}

// ListStaleSubmittedSpeechmatics returns Speechmatics rows stuck in "submitted".
func (r *TranscriptionRepository) ListStaleSubmittedSpeechmatics(ctx context.Context, olderThan time.Time) ([]*domain.Transcription, error) {
	rows, err := r.queries.ListStaleSubmittedSpeechmatics(ctx, olderThan)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Transcription, len(rows))
	for i, row := range rows {
		result[i] = transcriptionRowToDomain(row)
	}
	return result, nil
}

// ListUndeletedFromSpeechmatics returns terminal rows still awaiting provider deletion.
func (r *TranscriptionRepository) ListUndeletedFromSpeechmatics(ctx context.Context) ([]*domain.Transcription, error) {
	rows, err := r.queries.ListUndeletedFromSpeechmatics(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Transcription, len(rows))
	for i, row := range rows {
		result[i] = transcriptionRowToDomain(row)
	}
	return result, nil
}

// MarkSpeechmaticsDeleted records provider-side deletion for a transcription.
func (r *TranscriptionRepository) MarkSpeechmaticsDeleted(ctx context.Context, id string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	rows, err := r.queries.MarkSpeechmaticsDeleted(ctx, uuid)
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkSegmentSubmittedToSpeechmatics records the per-chunk job id on a segment.
func (r *TranscriptionRepository) MarkSegmentSubmittedToSpeechmatics(ctx context.Context, segmentID, jobID string) error {
	segUUID, err := parseUUID(segmentID)
	if err != nil {
		return domain.ErrNotFound
	}
	if jobID == "" {
		return domain.ErrInvalidInput
	}

	const q = `
UPDATE transcription_segments
SET status = $2,
    speechmatics_job_id = $3,
    error_message = NULL
WHERE id = $1`

	tag, err := r.pool.Exec(ctx, q, segUUID, domain.TranscriptionSegmentStatusSubmitted, jobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetSegmentBySpeechmaticsJobID retrieves a segment by its Speechmatics job id.
func (r *TranscriptionRepository) GetSegmentBySpeechmaticsJobID(ctx context.Context, jobID string) (*domain.TranscriptionSegment, error) {
	if jobID == "" {
		return nil, domain.ErrNotFound
	}

	const q = `SELECT ` + segmentColumns + `
FROM transcription_segments
WHERE speechmatics_job_id = $1`

	seg, err := scanTranscriptionSegmentRow(r.pool.QueryRow(ctx, q, jobID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return seg, nil
}

// ListStaleSubmittedSpeechmaticsSegments returns submitted Speechmatics segments
// older than the cutoff.
func (r *TranscriptionRepository) ListStaleSubmittedSpeechmaticsSegments(ctx context.Context, olderThan time.Time) ([]*domain.TranscriptionSegment, error) {
	const q = `SELECT ` + segmentColumns + `
FROM transcription_segments
WHERE status = 'submitted' AND speechmatics_job_id IS NOT NULL AND updated_at < $1
ORDER BY updated_at ASC
LIMIT 50`

	return r.querySegments(ctx, q, olderThan)
}

// ListUndeletedSpeechmaticsSegments lists terminal jobs that still need
// provider-side deletion confirmation.
func (r *TranscriptionRepository) ListUndeletedSpeechmaticsSegments(ctx context.Context) ([]*domain.TranscriptionSegment, error) {
	const query = `
SELECT s.id, s.transcription_id, s.segment_index, s.start_offset_sec, s.end_offset_sec,
       s.speechmatics_job_id, s.status,
       s.content_encrypted, s.content_nonce, s.wrapped_dek, s.wrapping_nonce,
       s.speaker_count, s.word_count, s.duration_seconds, s.error_message,
       s.created_at, s.updated_at, s.completed_at, s.speechmatics_deleted_at
FROM transcription_segments s
JOIN transcriptions t ON t.id = s.transcription_id
WHERE s.speechmatics_job_id IS NOT NULL
  AND s.speechmatics_deleted_at IS NULL
  AND (
      s.status IN ('completed', 'failed')
      OR t.status IN ('completed', 'failed', 'deleted')
  )
ORDER BY s.updated_at ASC
LIMIT 50`
	return r.querySegments(ctx, query)
}

// MarkSegmentSpeechmaticsDeleted records provider-side deletion for a segment.
func (r *TranscriptionRepository) MarkSegmentSpeechmaticsDeleted(ctx context.Context, id string) error {
	segUUID, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	const q = `
UPDATE transcription_segments
SET speechmatics_deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND speechmatics_deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, segUUID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
