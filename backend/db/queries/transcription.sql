-- name: CreateTranscription :one
INSERT INTO transcriptions (
    organization_id, media_id, languages, diarization, enhance_audio, status,
    vocabulary_packs, expected_speakers
) VALUES (
    @organization_id, @media_id, @languages, @diarization, @enhance_audio, @status,
    @vocabulary_packs, @expected_speakers
) RETURNING *;

-- name: GetTranscriptionByID :one
SELECT * FROM transcriptions WHERE id = @id;

-- name: GetTranscriptionByMediaID :one
SELECT * FROM transcriptions
WHERE media_id = @media_id AND status NOT IN ('failed', 'deleted')
ORDER BY created_at DESC LIMIT 1;

-- name: ListTranscriptionsByOrganization :many
SELECT t.*, m.filename AS media_filename, m.status AS media_status,
       COALESCE(m.title, '') AS media_title, COALESCE(m.description, '') AS media_description
FROM transcriptions t
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE t.organization_id = @organization_id AND t.status != 'deleted'
  AND (
    @search::text = '' OR m.title ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR m.description ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR m.filename ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
  )
ORDER BY t.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountTranscriptionsByOrganization :one
SELECT COUNT(*) FROM transcriptions t
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE t.organization_id = @organization_id AND t.status != 'deleted'
  AND (
    @search::text = '' OR m.title ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR m.description ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR m.filename ILIKE '%' || replace(replace(replace(@search::text, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
  );

-- name: UpdateTranscriptionStatus :one
UPDATE transcriptions SET
    status = @status,
    error_message = COALESCE(@error_message, error_message),
    preprocessor_used = COALESCE(@preprocessor_used, preprocessor_used)
WHERE id = @id AND (status NOT IN ('completed', 'failed', 'deleted') OR status = @status::text OR @status::text = 'deleted')
RETURNING *;

-- name: UpdateTranscriptionContent :one
UPDATE transcriptions SET
    status = @status,
    content_encrypted = @content_encrypted,
    content_nonce = @content_nonce,
    wrapped_dek = @wrapped_dek,
    wrapping_nonce = @wrapping_nonce,
    speaker_count = @speaker_count,
    word_count = @word_count,
    duration_seconds = @duration_seconds,
    completed_at = COALESCE(completed_at, NOW())
WHERE id = @id AND status NOT IN ('failed', 'deleted')
RETURNING *;

-- name: UpdateTranscriptionContentCAS :execrows
UPDATE transcriptions SET
    content_encrypted = @content_encrypted,
    content_nonce = @content_nonce,
    wrapped_dek = @wrapped_dek,
    wrapping_nonce = @wrapping_nonce
WHERE id = @id AND status = 'completed' AND content_nonce = @expected_content_nonce;

-- name: LockTranscriptionForPurge :one
SELECT id FROM transcriptions WHERE id = @id FOR UPDATE;

-- The purge queries remove every ciphertext column derived from the listed
-- transcriptions. Provider job ids stay so the Speechmatics cleaner can still
-- delete the provider-side copy of a purged job.

-- name: PurgeSummariesByTranscriptionIDs :exec
UPDATE summaries SET
    status = 'deleted',
    content_encrypted = NULL,
    content_nonce = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    review_findings_encrypted = NULL,
    review_findings_nonce = NULL,
    review_wrapped_dek = NULL,
    review_wrapping_nonce = NULL,
    extraction_encrypted = NULL,
    extraction_nonce = NULL,
    extraction_wrapped_dek = NULL,
    extraction_wrapping_nonce = NULL,
    structured_content_ciphertext = NULL,
    structured_content_nonce = NULL,
    structured_content_wrapped_dek = NULL,
    structured_content_wrapping_nonce = NULL,
    source_hash = NULL,
    error_message = NULL
WHERE transcription_id = ANY(@transcription_ids::uuid[]);

-- name: PurgeSegmentsByTranscriptionIDs :exec
UPDATE transcription_segments SET
    status = CASE WHEN status IN ('completed', 'failed') THEN status ELSE 'failed' END,
    content_encrypted = NULL,
    content_nonce = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    error_message = NULL
WHERE transcription_id = ANY(@transcription_ids::uuid[]);

-- name: PurgeTranscriptionsByIDs :exec
UPDATE transcriptions SET
    status = 'deleted',
    content_encrypted = NULL,
    content_nonce = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    error_message = NULL
WHERE id = ANY(@ids::uuid[]);

-- name: DeleteCollectionItemsByTranscriptionIDs :exec
DELETE FROM mcp_collection_items WHERE transcription_id = ANY(@transcription_ids::uuid[]);

-- name: SoftDeleteTranscriptionsByMediaID :execrows
UPDATE transcriptions SET status = 'deleted'
WHERE media_id = @media_id AND status != 'deleted';

-- name: ListCompletedTranscriptionsForTranscriptSearch :many
SELECT t.*, m.filename AS media_filename, m.status AS media_status,
       COALESCE(m.title, '') AS media_title, COALESCE(m.description, '') AS media_description
FROM transcriptions t
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE t.organization_id = @organization_id
  AND t.status = 'completed'
  AND (
    NOT @has_cursor::bool
    OR (t.created_at, t.id) < (@after_created_at::timestamptz, @after_id::uuid)
  )
ORDER BY t.created_at DESC, t.id DESC
LIMIT @limit_val;
