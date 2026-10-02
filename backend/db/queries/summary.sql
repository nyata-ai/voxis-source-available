-- name: CreateSummary :one
INSERT INTO summaries (
    organization_id, transcription_id, summary_type, status, high_stakes, review_status,
    summary_profile
) VALUES (
    @organization_id, @transcription_id, @summary_type, @status, @high_stakes, @review_status,
    @summary_profile
) RETURNING *;

-- name: GetSummaryByID :one
SELECT * FROM summaries WHERE id = @id;

-- name: GetActiveSummaryByTranscriptionAndType :one
SELECT * FROM summaries
WHERE transcription_id = @transcription_id
  AND summary_type = @summary_type
  AND status NOT IN ('failed', 'deleted')
LIMIT 1;

-- name: ListSummariesByTranscription :many
SELECT s.*, COALESCE(m.filename, '') AS transcription_media_filename,
  COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') AS transcription_audio_name,
  COALESCE(m.description, '') AS transcription_media_description
FROM summaries s
JOIN transcriptions t ON s.transcription_id = t.id AND t.status != 'deleted'
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE s.transcription_id = @transcription_id AND s.status != 'deleted'
ORDER BY s.created_at DESC;

-- name: ListSummariesByOrganization :many
SELECT s.*, COALESCE(m.filename, '') AS transcription_media_filename,
  COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') AS transcription_audio_name,
  COALESCE(m.description, '') AS transcription_media_description
FROM summaries s
JOIN transcriptions t ON s.transcription_id = t.id AND t.status != 'deleted'
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE s.organization_id = @organization_id AND s.status != 'deleted'
  AND (
    @search::text = '' OR
    COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') ILIKE '%' || replace(replace(replace(@search::text, '\\', '\\\\'), '%', '\\%'), '_', '\\_') || '%' ESCAPE '\\' OR
    COALESCE(m.description, '') ILIKE '%' || replace(replace(replace(@search::text, '\\', '\\\\'), '%', '\\%'), '_', '\\_') || '%' ESCAPE '\\'
  )
ORDER BY s.created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSummariesByOrganization :one
SELECT COUNT(*) FROM summaries s
JOIN transcriptions t ON s.transcription_id = t.id AND t.status != 'deleted'
JOIN media m ON t.media_id = m.id AND m.status != 'deleted'
WHERE s.organization_id = @organization_id AND s.status != 'deleted'
  AND (
    @search::text = '' OR
    COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') ILIKE '%' || replace(replace(replace(@search::text, '\\', '\\\\'), '%', '\\%'), '_', '\\_') || '%' ESCAPE '\\' OR
    COALESCE(m.description, '') ILIKE '%' || replace(replace(replace(@search::text, '\\', '\\\\'), '%', '\\%'), '_', '\\_') || '%' ESCAPE '\\'
  );

-- name: UpdateSummaryContent :one
UPDATE summaries SET
    status = @status,
    content_encrypted = @content_encrypted,
    content_nonce = @content_nonce,
    wrapped_dek = @wrapped_dek,
    wrapping_nonce = @wrapping_nonce,
    word_count = @word_count,
    prompt_tokens = @prompt_tokens,
    completion_tokens = @completion_tokens,
    thinking_tokens = @thinking_tokens,
    review_status = @review_status,
    extraction_encrypted = @extraction_encrypted,
    extraction_nonce = @extraction_nonce,
    extraction_wrapped_dek = @extraction_wrapped_dek,
    extraction_wrapping_nonce = @extraction_wrapping_nonce,
    prompt_version = @prompt_version,
    model = @model,
    model_metadata = @model_metadata,
    endpoint_location = @endpoint_location,
    source_version = @source_version,
    source_hash = @source_hash,
    degradation_codes = @degradation_codes,
    structured_content_ciphertext = @structured_content_ciphertext,
    structured_content_nonce = @structured_content_nonce,
    structured_content_wrapped_dek = @structured_content_wrapped_dek,
    structured_content_wrapping_nonce = @structured_content_wrapping_nonce,
    structured_schema_version = @structured_schema_version,
    completed_at = COALESCE(completed_at, NOW())
WHERE id = @id AND status NOT IN ('failed', 'deleted')
RETURNING *;

-- name: UpdateSummaryStatus :one
UPDATE summaries SET status = @status, error_message = COALESCE(@error_message, error_message)
WHERE id = @id AND (status NOT IN ('completed', 'failed', 'deleted') OR status = @status::text OR @status::text = 'deleted')
RETURNING *;

-- name: SoftDeleteSummariesByTranscriptionID :execrows
UPDATE summaries SET status = 'deleted'
WHERE transcription_id = @transcription_id AND status != 'deleted';

-- name: SoftDeleteSummariesByMediaID :execrows
UPDATE summaries SET status = 'deleted'
WHERE transcription_id IN (SELECT id FROM transcriptions WHERE media_id = @media_id)
  AND status != 'deleted';

-- name: UndoDeleteSummary :one
UPDATE summaries SET status = @status
WHERE id = @id AND organization_id = @organization_id AND status = 'deleted'
RETURNING *;
