-- Voxis-OSS initial schema. This is intentionally independent of the
-- commercial migration sequence and is for a new installation only.
-- It contains retained uploads, standard recording, Speechmatics,
-- summaries, exports, activity, API keys, and MCP collections.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    settings JSONB NOT NULL DEFAULT '{}',
    encryption_key_id VARCHAR(255),
    tier VARCHAR(50) NOT NULL DEFAULT 'oss',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_organizations_slug ON organizations(slug);
CREATE TRIGGER update_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE users (
    id VARCHAR(255) PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    normalized_email VARCHAR(255),
    name VARCHAR(255) NOT NULL DEFAULT '',
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    preferences JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_users_organization_id ON users(organization_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_normalized_email ON users(normalized_email);
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE media (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_by TEXT,
    filename VARCHAR(500) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL CHECK (size >= 0),
    duration DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (duration >= 0),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    scan_status TEXT NOT NULL DEFAULT 'scan_pending',
    storage_key VARCHAR(1000),
    wrapped_dek BYTEA,
    wrapping_nonce BYTEA,
    encryption_algo VARCHAR(50),
    chunk_size INTEGER NOT NULL DEFAULT 0 CHECK (chunk_size >= 0),
    chunk_count INTEGER NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    title VARCHAR(255),
    description VARCHAR(1000),
    file_hash VARCHAR(64),
    audio_metadata BYTEA,
    audio_deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_media_organization_id ON media(organization_id);
CREATE INDEX idx_media_status ON media(status);
CREATE INDEX idx_media_created_at ON media(created_at DESC);
CREATE INDEX idx_media_created_by ON media(created_by) WHERE created_by IS NOT NULL;
CREATE INDEX idx_media_scan_status ON media(scan_status) WHERE scan_status = 'scan_pending';
CREATE INDEX idx_media_title_trgm ON media USING gin (title gin_trgm_ops);
CREATE INDEX idx_media_description_trgm ON media USING gin (description gin_trgm_ops);
CREATE INDEX idx_media_filename_trgm ON media USING gin (filename gin_trgm_ops);
CREATE TRIGGER update_media_updated_at
    BEFORE UPDATE ON media
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE transcriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    media_id UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    provider TEXT NOT NULL DEFAULT 'speechmatics'
        CHECK (provider = 'speechmatics'),
    speechmatics_job_id TEXT,
    speechmatics_deleted_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    languages TEXT[] NOT NULL DEFAULT '{auto}',
    diarization BOOLEAN NOT NULL DEFAULT TRUE,
    expected_speakers INTEGER CHECK (expected_speakers IS NULL OR expected_speakers BETWEEN 1 AND 10),
    vocabulary_packs TEXT[],
    enhance_audio BOOLEAN NOT NULL DEFAULT FALSE,
    preprocessor_used TEXT,
    content_encrypted BYTEA,
    content_nonce BYTEA,
    wrapped_dek BYTEA,
    wrapping_nonce BYTEA,
    speaker_count INTEGER NOT NULL DEFAULT 0 CHECK (speaker_count >= 0),
    word_count INTEGER NOT NULL DEFAULT 0 CHECK (word_count >= 0),
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX idx_transcriptions_org_id ON transcriptions(organization_id);
CREATE INDEX idx_transcriptions_media_id ON transcriptions(media_id);
CREATE INDEX idx_transcriptions_status ON transcriptions(status);
CREATE INDEX idx_transcriptions_created_at ON transcriptions(created_at DESC);
CREATE INDEX idx_transcriptions_org_status_completed
    ON transcriptions(organization_id, status, completed_at DESC);
CREATE UNIQUE INDEX idx_transcriptions_active_media
    ON transcriptions(media_id) WHERE status NOT IN ('failed', 'deleted');
CREATE UNIQUE INDEX idx_transcriptions_speechmatics_job_id
    ON transcriptions(speechmatics_job_id) WHERE speechmatics_job_id IS NOT NULL;
CREATE INDEX idx_transcriptions_speechmatics_stale_submitted
    ON transcriptions(updated_at ASC)
    WHERE status = 'submitted' AND speechmatics_job_id IS NOT NULL;
CREATE INDEX idx_transcriptions_speechmatics_undeleted
    ON transcriptions(updated_at ASC)
    WHERE speechmatics_job_id IS NOT NULL
      AND speechmatics_deleted_at IS NULL
      AND status IN ('completed', 'failed', 'deleted');
CREATE TRIGGER update_transcriptions_updated_at
    BEFORE UPDATE ON transcriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE transcription_segments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    transcription_id UUID NOT NULL REFERENCES transcriptions(id) ON DELETE CASCADE,
    segment_index INTEGER NOT NULL CHECK (segment_index >= 0),
    start_offset_sec DOUBLE PRECISION NOT NULL,
    end_offset_sec DOUBLE PRECISION NOT NULL,
    speechmatics_job_id TEXT,
    speechmatics_deleted_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    content_encrypted BYTEA,
    content_nonce BYTEA,
    wrapped_dek BYTEA,
    wrapping_nonce BYTEA,
    speaker_count INTEGER NOT NULL DEFAULT 0 CHECK (speaker_count >= 0),
    word_count INTEGER NOT NULL DEFAULT 0 CHECK (word_count >= 0),
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT transcription_segments_offsets_valid CHECK (end_offset_sec > start_offset_sec),
    CONSTRAINT transcription_segments_unique_index UNIQUE (transcription_id, segment_index)
);
CREATE UNIQUE INDEX idx_transcription_segments_speechmatics_job_id
    ON transcription_segments(speechmatics_job_id) WHERE speechmatics_job_id IS NOT NULL;
CREATE INDEX idx_transcription_segments_transcription_id
    ON transcription_segments(transcription_id);
CREATE INDEX idx_transcription_segments_speechmatics_stale_submitted
    ON transcription_segments(updated_at ASC)
    WHERE status = 'submitted' AND speechmatics_job_id IS NOT NULL;
CREATE INDEX idx_transcription_segments_speechmatics_undeleted
    ON transcription_segments(updated_at ASC)
    WHERE speechmatics_job_id IS NOT NULL AND speechmatics_deleted_at IS NULL;
CREATE TRIGGER update_transcription_segments_updated_at
    BEFORE UPDATE ON transcription_segments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE summaries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    transcription_id UUID NOT NULL REFERENCES transcriptions(id) ON DELETE CASCADE,
    summary_type VARCHAR(20) NOT NULL,
    summary_profile TEXT NOT NULL DEFAULT 'general_professional',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    content_encrypted BYTEA,
    content_nonce BYTEA,
    wrapped_dek BYTEA,
    wrapping_nonce BYTEA,
    word_count INTEGER NOT NULL DEFAULT 0 CHECK (word_count >= 0),
    prompt_tokens INTEGER NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens INTEGER NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    thinking_tokens INTEGER NOT NULL DEFAULT 0 CHECK (thinking_tokens >= 0),
    error_message TEXT,
    high_stakes BOOLEAN NOT NULL DEFAULT FALSE,
    review_status TEXT NOT NULL DEFAULT 'skipped',
    review_findings_encrypted BYTEA,
    review_findings_nonce BYTEA,
    review_wrapped_dek BYTEA,
    review_wrapping_nonce BYTEA,
    extraction_encrypted BYTEA,
    extraction_nonce BYTEA,
    extraction_wrapped_dek BYTEA,
    extraction_wrapping_nonce BYTEA,
    prompt_version TEXT,
    model TEXT,
    model_metadata JSONB NOT NULL DEFAULT '{}',
    endpoint_location TEXT,
    source_version TEXT,
    source_hash TEXT,
    structured_content_ciphertext BYTEA,
    structured_content_nonce BYTEA,
    structured_content_wrapped_dek BYTEA,
    structured_content_wrapping_nonce BYTEA,
    structured_schema_version TEXT,
    degradation_codes TEXT[] NOT NULL DEFAULT '{}'
        CHECK (cardinality(degradation_codes) <= 8),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX idx_summaries_org_id ON summaries(organization_id);
CREATE INDEX idx_summaries_transcription_id ON summaries(transcription_id);
CREATE INDEX idx_summaries_status ON summaries(status);
CREATE INDEX idx_summaries_created_at ON summaries(created_at DESC);
CREATE INDEX idx_summaries_high_stakes ON summaries(high_stakes) WHERE high_stakes = TRUE;
CREATE UNIQUE INDEX idx_summaries_active_transcription_type
    ON summaries(transcription_id, summary_type) WHERE status NOT IN ('failed', 'deleted');
CREATE TRIGGER update_summaries_updated_at
    BEFORE UPDATE ON summaries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE llm_prompt_overrides (
    key TEXT PRIMARY KEY,
    system_instruction TEXT NOT NULL CHECK (length(system_instruction) BETWEEN 1 AND 20000),
    user_prompt TEXT NOT NULL CHECK (length(user_prompt) BETWEEN 1 AND 20000),
    model TEXT NOT NULL CHECK (length(model) BETWEEN 1 AND 128),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by TEXT NOT NULL DEFAULT ''
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_by VARCHAR(255) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL UNIQUE,
    key_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_api_keys_prefix_active ON api_keys(key_prefix) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_keys_org_active ON api_keys(organization_id, created_at DESC) WHERE revoked_at IS NULL;
CREATE TRIGGER update_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE recording_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'recording',
    mime_type VARCHAR(100) NOT NULL DEFAULT 'audio/webm',
    microphone_label VARCHAR(255),
    capture_source TEXT NOT NULL DEFAULT 'microphone'
        CHECK (capture_source IN ('microphone', 'mixed_audio')),
    media_id UUID REFERENCES media(id) ON DELETE SET NULL,
    total_duration DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (total_duration >= 0),
    last_chunk_at TIMESTAMPTZ,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX idx_recording_sessions_org_id ON recording_sessions(organization_id);
CREATE INDEX idx_recording_sessions_user_status ON recording_sessions(user_id, status);
CREATE INDEX idx_recording_sessions_created_at ON recording_sessions(created_at DESC);
CREATE INDEX idx_recording_sessions_status_last_activity_at
    ON recording_sessions(status, last_activity_at);
CREATE UNIQUE INDEX idx_recording_sessions_active_user
    ON recording_sessions(user_id) WHERE status IN ('recording', 'paused');
CREATE TRIGGER update_recording_sessions_updated_at
    BEFORE UPDATE ON recording_sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE recording_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES recording_sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL CHECK (seq >= 0),
    storage_key TEXT NOT NULL,
    wrapped_dek BYTEA NOT NULL,
    wrapping_nonce BYTEA NOT NULL,
    encryption_algo VARCHAR(50) NOT NULL DEFAULT 'aes-256-gcm-chunked',
    chunk_size INTEGER NOT NULL CHECK (chunk_size > 0),
    chunk_count INTEGER NOT NULL CHECK (chunk_count > 0),
    plaintext_size BIGINT NOT NULL CHECK (plaintext_size >= 0),
    checksum VARCHAR(64) NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_recording_chunks_session_seq UNIQUE (session_id, seq)
);
CREATE INDEX idx_recording_chunks_session_id ON recording_chunks(session_id);

CREATE TABLE app_recording_retention_policy (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    retention_days SMALLINT NOT NULL DEFAULT 7 CHECK (retention_days BETWEEN 1 AND 30),
    effective_at TIMESTAMPTZ,
    apply_to_existing BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ,
    updated_by TEXT
);
INSERT INTO app_recording_retention_policy (id) VALUES (TRUE);

CREATE TABLE app_storage_quota_policy (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    default_limit_bytes BIGINT NOT NULL DEFAULT 2147483648 CHECK (default_limit_bytes > 0),
    warning_threshold_percent SMALLINT NOT NULL DEFAULT 85 CHECK (warning_threshold_percent = 85),
    updated_at TIMESTAMPTZ,
    updated_by TEXT
);
INSERT INTO app_storage_quota_policy (id) VALUES (TRUE);

CREATE TABLE user_storage_allocations (
    id TEXT PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    user_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('media', 'recording_chunk')),
    media_id UUID,
    recording_session_id UUID,
    recording_chunk_seq INTEGER,
    replaces_session_id UUID,
    bytes BIGINT NOT NULL CHECK (bytes >= 0),
    state TEXT NOT NULL CHECK (state IN ('reserved', 'committed', 'released')),
    expires_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_user_storage_allocation_target CHECK (
        (kind = 'media' AND media_id IS NOT NULL AND recording_session_id IS NULL)
        OR (kind = 'recording_chunk' AND media_id IS NULL AND recording_session_id IS NOT NULL AND recording_chunk_seq IS NOT NULL)
    )
);
CREATE UNIQUE INDEX uq_user_storage_media_allocation
    ON user_storage_allocations(media_id) WHERE media_id IS NOT NULL;
CREATE UNIQUE INDEX uq_user_storage_chunk_allocation
    ON user_storage_allocations(recording_session_id, recording_chunk_seq)
    WHERE recording_session_id IS NOT NULL;
CREATE UNIQUE INDEX uq_user_storage_active_stitch_replacement
    ON user_storage_allocations(replaces_session_id)
    WHERE kind = 'media' AND state != 'released' AND replaces_session_id IS NOT NULL;
CREATE INDEX idx_user_storage_allocations_user_active
    ON user_storage_allocations(user_id, state, expires_at) WHERE state != 'released';

CREATE TABLE mcp_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);
CREATE TABLE mcp_collection_items (
    collection_id UUID NOT NULL REFERENCES mcp_collections(id) ON DELETE CASCADE,
    transcription_id UUID NOT NULL REFERENCES transcriptions(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (collection_id, transcription_id)
);
CREATE INDEX idx_mcp_collections_org_created ON mcp_collections(organization_id, created_at DESC);
CREATE INDEX idx_mcp_collection_items_collection_added ON mcp_collection_items(collection_id, added_at DESC);
