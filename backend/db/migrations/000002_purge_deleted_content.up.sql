-- Deletion used to mark rows deleted while keeping their ciphertext and the
-- media names. Deletion now purges both (see MediaRepository.CascadeDelete);
-- this migration applies the same purge to rows deleted before the upgrade.
-- It changes data only, so there is nothing for the down migration to undo.
--
-- It clears media.storage_key without deleting the stored object. On an
-- install that deleted media before this migration, those encrypted objects
-- stay on disk as orphans: nothing sweeps them except purge-user, which removes
-- the whole organization prefix. This is accepted because the edition has no
-- public installs from before this migration; every supported installation is
-- fresh (see infra/oss/README.md).

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
WHERE status = 'deleted'
   OR transcription_id IN (SELECT id FROM transcriptions WHERE status = 'deleted');

UPDATE transcription_segments SET
    status = CASE WHEN status IN ('completed', 'failed') THEN status ELSE 'failed' END,
    content_encrypted = NULL,
    content_nonce = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    error_message = NULL
WHERE transcription_id IN (SELECT id FROM transcriptions WHERE status = 'deleted');

UPDATE transcriptions SET
    content_encrypted = NULL,
    content_nonce = NULL,
    wrapped_dek = NULL,
    wrapping_nonce = NULL,
    error_message = NULL
WHERE status = 'deleted';

DELETE FROM mcp_collection_items
WHERE transcription_id IN (SELECT id FROM transcriptions WHERE status = 'deleted');

UPDATE recording_sessions SET microphone_label = NULL
WHERE media_id IN (SELECT id FROM media WHERE status = 'deleted');

UPDATE media SET
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
WHERE status = 'deleted';

-- Summary metadata used to record the internal model URL. Replace it with the
-- label the application now writes.
UPDATE summaries SET endpoint_location = 'local-gemma'
WHERE endpoint_location IS NOT NULL AND endpoint_location <> 'local-gemma';
