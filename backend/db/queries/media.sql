-- name: CreateMedia :one
INSERT INTO media (
    organization_id, created_by, filename, content_type, size, duration, status, scan_status
) VALUES (
    @organization_id, @created_by, @filename, @content_type, @size, @duration, @status, @scan_status
) RETURNING *;

-- name: GetMediaByID :one
SELECT * FROM media WHERE id = @id;

-- name: ListMediaByOrganization :many
SELECT * FROM media
WHERE organization_id = @organization_id AND status != 'deleted'
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: UpdateMediaEncryption :one
UPDATE media SET
    storage_key = @storage_key,
    wrapped_dek = @wrapped_dek,
    wrapping_nonce = @wrapping_nonce,
    encryption_algo = @encryption_algo,
    chunk_size = @chunk_size,
    chunk_count = @chunk_count,
    status = @status
WHERE id = @id
RETURNING *;

-- name: UpdateMediaStatus :one
UPDATE media SET status = @status WHERE id = @id RETURNING *;

-- name: UpdateMediaDuration :one
UPDATE media SET duration = @duration WHERE id = @id RETURNING *;

-- name: SoftDeleteMedia :exec
UPDATE media SET status = 'deleted' WHERE id = @id;

-- The media purge runs in one transaction. Rows are locked parent-first
-- (media, then its transcriptions) so a worker that share-locks a
-- transcription before writing segment ciphertext waits for the purge and
-- then sees status 'deleted'.

-- name: LockMediaForPurge :one
SELECT id FROM media WHERE id = @id FOR UPDATE;

-- name: LockMediaTranscriptionsForPurge :many
SELECT id FROM transcriptions WHERE media_id = @media_id ORDER BY id FOR UPDATE;

-- name: ListMediaRecordingChunkKeysForPurge :many
SELECT c.storage_key FROM recording_chunks c
JOIN recording_sessions s ON s.id = c.session_id
WHERE s.media_id = @media_id
ORDER BY c.storage_key
LIMIT 5000;

-- name: DeleteMediaRecordingChunks :exec
DELETE FROM recording_chunks
WHERE session_id IN (SELECT id FROM recording_sessions WHERE media_id = @media_id);

-- name: ScrubMediaRecordingSessions :exec
UPDATE recording_sessions SET microphone_label = NULL WHERE media_id = @media_id;

-- PurgeMediaRow keeps a content-free tombstone: status, size, duration and
-- timestamps stay for activity and admin counts; every name, hash, and key
-- reference goes.

-- name: PurgeMediaRow :exec
UPDATE media SET
    status = 'deleted',
    filename = '',
    title = NULL,
    description = NULL,
    file_hash = NULL,
    audio_metadata = NULL,
    storage_key = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    encryption_algo = NULL,
    chunk_size = 0,
    chunk_count = 0,
    audio_deleted_at = COALESCE(audio_deleted_at, NOW())
WHERE id = @id;

-- name: CountMediaByOrganization :one
SELECT COUNT(*) FROM media
WHERE organization_id = @organization_id AND status != 'deleted';

-- name: UpdateMediaMetadata :one
UPDATE media SET
    title = CASE WHEN @set_title::boolean THEN @title ELSE title END,
    description = CASE WHEN @set_description::boolean THEN @description ELSE description END
WHERE id = @id
RETURNING *;

-- name: UpdateMediaForensics :one
UPDATE media SET file_hash = @file_hash, audio_metadata = @audio_metadata
WHERE id = @id
RETURNING *;

-- name: SearchMediaByOrganization :many
SELECT * FROM media
WHERE organization_id = @organization_id
  AND status != 'deleted'
  AND (@search_query::text = '' OR (
    filename ILIKE '%' || replace(replace(replace(@search_query, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(title, '') ILIKE '%' || replace(replace(replace(@search_query, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(description, '') ILIKE '%' || replace(replace(replace(@search_query, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
  ))
  AND (@status_filter::text = '' OR status = @status_filter)
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountSearchMediaByOrganization :one
SELECT COUNT(*) FROM media
WHERE organization_id = @organization_id
  AND status != 'deleted'
  AND (@search_query::text = '' OR (
    filename ILIKE '%' || replace(replace(replace(@search_query, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(title, '') ILIKE '%' || replace(replace(replace(@search_query, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
    OR COALESCE(description, '') ILIKE '%' || replace(replace(replace(@search_query, '\', '\\'), '%', '\%'), '_', '\_') || '%' ESCAPE '\'
  ))
  AND (@status_filter::text = '' OR status = @status_filter);

-- name: UpdateMediaScanStatus :exec
UPDATE media SET scan_status = @scan_status, updated_at = NOW() WHERE id = @id;

-- name: MarkMediaAudioDeleted :one
UPDATE media
SET storage_key = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    encryption_algo = NULL,
    chunk_size = 0,
    chunk_count = 0,
    file_hash = NULL,
    audio_metadata = NULL,
    audio_deleted_at = COALESCE(audio_deleted_at, @deleted_at),
    updated_at = NOW()
WHERE id = @id AND storage_key = @storage_key
RETURNING *;
