-- name: GetUsageStats :one
-- Returns aggregated usage statistics for an organization.
-- Used by GET /api/v1/usage/stats.
SELECT
    COALESCE(
        (SELECT SUM(t.duration_seconds) FROM transcriptions t
         WHERE t.organization_id = @organization_id
           AND t.status = 'completed'),
        0
    )::double precision AS total_duration_seconds,

    COALESCE(summary_agg.prompt_tokens, 0)::bigint AS total_prompt_tokens,
    COALESCE(summary_agg.completion_tokens, 0)::bigint AS total_completion_tokens,
    COALESCE(summary_agg.thinking_tokens, 0)::bigint AS total_thinking_tokens,

    (COALESCE(
        (SELECT SUM(m.size) FROM media m
         WHERE m.organization_id = @organization_id
           AND m.status != 'deleted'
           AND m.storage_key IS NOT NULL),
        0
    )
    + COALESCE(
        (SELECT SUM(rc.plaintext_size)
         FROM recording_chunks rc
         JOIN recording_sessions rs ON rs.id = rc.session_id
         WHERE rs.organization_id = @organization_id),
        0
    ))::bigint AS storage_used_bytes,

    COALESCE(
        (SELECT p.enabled FROM app_recording_retention_policy p WHERE p.id = TRUE),
        FALSE
    )::boolean AS live_recording_retention_enabled,
    COALESCE(
        (SELECT p.retention_days FROM app_recording_retention_policy p WHERE p.id = TRUE),
        0
    )::smallint AS live_recording_retention_days

FROM (
    SELECT
        SUM(s.prompt_tokens) AS prompt_tokens,
        SUM(s.completion_tokens) AS completion_tokens,
        SUM(s.thinking_tokens) AS thinking_tokens
    FROM summaries s
    WHERE s.organization_id = @organization_id
      AND s.status = 'completed'
) summary_agg;
