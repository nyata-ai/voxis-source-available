-- Operator data purge for one person (cmd/server purge-user). Each person has
-- a personal organization; these queries only ever address that organization.

-- name: GetUserOrganizationForPurge :one
SELECT
    u.id,
    u.organization_id,
    (SELECT COUNT(*) FROM users m WHERE m.organization_id = u.organization_id)::bigint AS member_count
FROM users u
WHERE u.id = @user_id;

-- name: CountOrganizationPurgeTargets :one
SELECT
    (SELECT COUNT(*) FROM media m WHERE m.organization_id = @org_id)::bigint AS media,
    (SELECT COUNT(*) FROM transcriptions t WHERE t.organization_id = @org_id)::bigint AS transcriptions,
    (SELECT COUNT(*) FROM summaries s WHERE s.organization_id = @org_id)::bigint AS summaries,
    (SELECT COUNT(*) FROM recording_sessions r WHERE r.organization_id = @org_id)::bigint AS recording_sessions,
    (SELECT COUNT(*) FROM recording_chunks c
        JOIN recording_sessions r ON r.id = c.session_id
        WHERE r.organization_id = @org_id)::bigint AS recording_chunks,
    (SELECT COUNT(*) FROM mcp_collections c WHERE c.organization_id = @org_id)::bigint AS collections,
    (SELECT COUNT(*) FROM api_keys k WHERE k.organization_id = @org_id)::bigint AS api_keys,
    ((SELECT COUNT(*) FROM transcriptions t
        WHERE t.organization_id = @org_id
          AND t.speechmatics_job_id IS NOT NULL
          AND t.speechmatics_deleted_at IS NULL)
     + (SELECT COUNT(*) FROM transcription_segments s
        JOIN transcriptions t ON t.id = s.transcription_id
        WHERE t.organization_id = @org_id
          AND s.speechmatics_job_id IS NOT NULL
          AND s.speechmatics_deleted_at IS NULL))::bigint AS pending_provider_deletions;

-- name: ListOrganizationMediaIDsAfter :many
SELECT id FROM media
WHERE organization_id = @org_id AND id > @after_id
ORDER BY id
LIMIT @limit_val;

-- name: DeleteOrganizationRecordingSessions :exec
DELETE FROM recording_sessions WHERE organization_id = @org_id;

-- name: DeleteOrganizationCollections :exec
DELETE FROM mcp_collections WHERE organization_id = @org_id;

-- name: DeleteOrganizationAPIKeys :exec
DELETE FROM api_keys WHERE organization_id = @org_id;

-- name: DeleteOrganizationStorageAllocations :exec
DELETE FROM user_storage_allocations WHERE organization_id = @org_id;

-- name: ScrubOrganizationIdentityForPurge :exec
UPDATE organizations SET name = 'Purged workspace', slug = 'purged-' || id::text, settings = '{}'
WHERE id = @org_id;

-- name: ScrubOrganizationUsersForPurge :exec
UPDATE users SET email = '', normalized_email = NULL, name = '', preferences = '{}'
WHERE organization_id = @org_id;

-- name: DeleteOrganizationForPurge :execrows
DELETE FROM organizations WHERE id = @org_id;
