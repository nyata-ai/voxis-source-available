-- name: CreateRecordingSession :one
INSERT INTO recording_sessions (
    organization_id, user_id, status, mime_type, microphone_label, last_activity_at, capture_source
) VALUES (
    @organization_id, @user_id, @status, @mime_type, @microphone_label, @last_activity_at, @capture_source
) RETURNING *;

-- name: GetRecordingSessionByID :one
SELECT * FROM recording_sessions WHERE id = @id;

-- name: GetActiveRecordingByUserID :one
SELECT * FROM recording_sessions
WHERE user_id = @user_id AND status IN ('recording', 'paused')
LIMIT 1;

-- name: GetInterruptedRecordingsByUserID :many
SELECT * FROM recording_sessions
WHERE user_id = @user_id AND status = 'interrupted'
ORDER BY created_at DESC;

-- name: CountInterruptedRecordingsByUserID :one
SELECT COUNT(*) FROM recording_sessions WHERE user_id = @user_id AND status = 'interrupted';

-- name: UpdateRecordingSessionStatus :execrows
UPDATE recording_sessions SET status = @new_status
WHERE id = @id AND status = @old_status;

-- name: SetRecordingMediaID :exec
UPDATE recording_sessions SET media_id = @media_id WHERE id = @id;

-- name: UpdateRecordingLastChunkAt :exec
UPDATE recording_sessions SET
    last_chunk_at = GREATEST(COALESCE(last_chunk_at, @last_chunk_at), @last_chunk_at),
    last_activity_at = GREATEST(COALESCE(last_activity_at, @last_chunk_at), @last_chunk_at)
WHERE id = @id;

-- name: UpdateRecordingLastActivityAt :exec
UPDATE recording_sessions SET last_activity_at = GREATEST(COALESCE(last_activity_at, @last_activity_at), @last_activity_at)
WHERE id = @id;

-- name: FindStaleRecordingSessions :many
SELECT * FROM recording_sessions
WHERE status IN ('recording', 'paused') AND last_activity_at < @threshold_time
ORDER BY created_at ASC;

-- name: ListRecordingSessionsByStatusOlderThan :many
SELECT * FROM recording_sessions
WHERE status = @status AND updated_at < @threshold_time
ORDER BY updated_at ASC;

-- name: ListRecordingSessionsWithChunksByStatusOlderThan :many
SELECT rs.* FROM recording_sessions rs
WHERE rs.status = @status AND rs.updated_at < @threshold_time
  AND EXISTS (SELECT 1 FROM recording_chunks rc WHERE rc.session_id = rs.id)
ORDER BY rs.updated_at ASC
LIMIT @limit_val;

-- name: SetRecordingCompletedAt :exec
UPDATE recording_sessions
SET completed_at = @completed_at, total_duration = @total_duration
WHERE id = @id;

-- name: CompleteRecordingSession :execrows
UPDATE recording_sessions
SET status = 'completed', completed_at = @completed_at, total_duration = @total_duration
WHERE id = @id AND status = 'completing';

-- name: CreateRecordingChunk :one
INSERT INTO recording_chunks (
    session_id, seq, storage_key, wrapped_dek, wrapping_nonce,
    encryption_algo, chunk_size, chunk_count, plaintext_size, checksum
) VALUES (
    @session_id, @seq, @storage_key, @wrapped_dek, @wrapping_nonce,
    @encryption_algo, @chunk_size, @chunk_count, @plaintext_size, @checksum
) RETURNING *;

-- name: UpsertRecordingChunk :one
INSERT INTO recording_chunks (
    session_id, seq, storage_key, wrapped_dek, wrapping_nonce,
    encryption_algo, chunk_size, chunk_count, plaintext_size, checksum
) VALUES (
    @session_id, @seq, @storage_key, @wrapped_dek, @wrapping_nonce,
    @encryption_algo, @chunk_size, @chunk_count, @plaintext_size, @checksum
) ON CONFLICT ON CONSTRAINT uq_recording_chunks_session_seq DO UPDATE SET
    storage_key = EXCLUDED.storage_key,
    wrapped_dek = EXCLUDED.wrapped_dek,
    wrapping_nonce = EXCLUDED.wrapping_nonce,
    encryption_algo = EXCLUDED.encryption_algo,
    chunk_size = EXCLUDED.chunk_size,
    chunk_count = EXCLUDED.chunk_count,
    plaintext_size = EXCLUDED.plaintext_size,
    checksum = EXCLUDED.checksum,
    uploaded_at = now()
RETURNING *;

-- name: GetRecordingChunkBySessionAndSeq :one
SELECT * FROM recording_chunks WHERE session_id = @session_id AND seq = @seq;

-- name: ListRecordingChunksBySession :many
SELECT * FROM recording_chunks WHERE session_id = @session_id ORDER BY seq ASC;

-- name: CountRecordingChunksBySession :one
SELECT COUNT(*) FROM recording_chunks WHERE session_id = @session_id;

-- name: MaxSeqRecordingChunksBySession :one
SELECT COALESCE(MAX(seq), -1)::integer AS max_seq FROM recording_chunks WHERE session_id = @session_id;

-- name: TotalRecordingChunkPlaintextSizeBySession :one
SELECT COALESCE(SUM(plaintext_size), 0)::bigint AS total_plaintext_size
FROM recording_chunks WHERE session_id = @session_id;

-- name: DeleteRecordingChunksBySession :exec
DELETE FROM recording_chunks WHERE session_id = @session_id;

-- name: ListLiveRecordingAudioRetentionCandidates :many
SELECT rs.id AS session_id, m.id AS media_id, m.organization_id, m.storage_key, rs.completed_at
FROM recording_sessions rs
JOIN media m ON m.id = rs.media_id
JOIN app_recording_retention_policy p ON p.id = TRUE
WHERE p.enabled = TRUE
  AND rs.status = 'completed'
  AND rs.completed_at IS NOT NULL
  AND rs.completed_at <= sqlc.arg(now_time)::timestamptz - (p.retention_days::int * INTERVAL '1 day')
  AND (p.apply_to_existing = TRUE OR (p.effective_at IS NOT NULL AND rs.completed_at >= p.effective_at))
  AND rs.media_id IS NOT NULL
  AND m.status != 'deleted'
  AND m.storage_key IS NOT NULL
  AND m.audio_deleted_at IS NULL
ORDER BY rs.completed_at ASC
LIMIT sqlc.arg(limit_val);
