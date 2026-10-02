-- name: CreateAPIKey :one
INSERT INTO api_keys (organization_id, created_by, name, key_prefix, key_hash, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys
WHERE key_prefix = $1 AND revoked_at IS NULL;

-- name: ListAPIKeysByOrg :many
SELECT * FROM api_keys
WHERE organization_id = $1 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: CountAPIKeysByOrg :one
SELECT COUNT(*) FROM api_keys
WHERE organization_id = $1 AND revoked_at IS NULL;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = NOW() WHERE id = $1;
