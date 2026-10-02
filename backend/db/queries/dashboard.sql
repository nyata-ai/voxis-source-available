-- name: GetDashboardStats :one
SELECT
    (SELECT COUNT(*) FROM media m WHERE m.organization_id = @organization_id AND m.status != 'deleted') AS total_media,
    ((SELECT COUNT(*) FROM media m WHERE m.organization_id = @organization_id AND m.status IN ('pending', 'encrypting')) +
     (SELECT COUNT(*) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status IN ('pending', 'submitted'))
    )::bigint AS processing_count,
    (SELECT COUNT(*) FROM media m WHERE m.organization_id = @organization_id AND m.status IN ('pending', 'encrypting')) AS encrypting_count,
    (SELECT COUNT(*) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status IN ('pending', 'submitted')) AS transcribing_count,
    (SELECT COUNT(*) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status = 'completed') AS completed_transcriptions,
    (SELECT COUNT(*) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status = 'completed'
       AND t.completed_at >= date_trunc('month', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC') AS completed_this_month,
    (SELECT COUNT(*) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status = 'completed'
       AND t.completed_at >= (date_trunc('month', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC') - INTERVAL '1 month'
       AND t.completed_at < date_trunc('month', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC') AS completed_last_month,
    COALESCE((SELECT SUM(t.duration_seconds) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status = 'completed'
       AND t.completed_at >= date_trunc('month', NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'), 0)::double precision AS seconds_this_month,
    COALESCE((SELECT MAX(t.completed_at) FROM transcriptions t WHERE t.organization_id = @organization_id AND t.status = 'completed'),
       '1970-01-01 00:00:00+00'::timestamptz)::timestamptz AS last_completed_at;
