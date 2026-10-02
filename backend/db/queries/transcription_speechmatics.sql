-- name: MarkTranscriptionSubmittedToSpeechmatics :one
UPDATE transcriptions SET
    status = @status,
    provider = 'speechmatics',
    speechmatics_job_id = COALESCE(sqlc.narg('speechmatics_job_id'), speechmatics_job_id),
    preprocessor_used = COALESCE(sqlc.narg('preprocessor_used'), preprocessor_used)
WHERE id = @id AND status IN ('pending', 'submitted')
RETURNING *;

-- name: GetTranscriptionBySpeechmaticsJobID :one
SELECT * FROM transcriptions WHERE speechmatics_job_id = @speechmatics_job_id;

-- name: ListStaleSubmittedSpeechmatics :many
SELECT * FROM transcriptions
WHERE status = 'submitted'
  AND speechmatics_job_id IS NOT NULL
  AND updated_at < @older_than
ORDER BY updated_at ASC
LIMIT 50;

-- name: ListUndeletedFromSpeechmatics :many
SELECT * FROM transcriptions
WHERE speechmatics_job_id IS NOT NULL
  AND speechmatics_job_id != ''
  AND speechmatics_deleted_at IS NULL
  AND status IN ('completed', 'failed', 'deleted')
ORDER BY updated_at ASC
LIMIT 50;

-- name: MarkSpeechmaticsDeleted :execrows
UPDATE transcriptions SET speechmatics_deleted_at = NOW(), updated_at = NOW()
WHERE id = @id AND speechmatics_deleted_at IS NULL;
