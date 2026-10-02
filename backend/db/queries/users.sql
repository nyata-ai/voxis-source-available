-- name: GetUserByID :one
-- NOTE: id is the Keycloak subject, not a generated ID
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (id, organization_id, email, name, normalized_email)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET email = $2, name = $3, normalized_email = $4
WHERE id = $1
RETURNING *;

-- name: ExistsByNormalizedEmail :one
-- Signup anti-abuse: reports whether a provisioned user already occupies an
-- email's normalized alias form (see domain.NormalizeEmailAlias).
SELECT EXISTS(SELECT 1 FROM users WHERE normalized_email = $1);

-- name: GetUserWithOrganization :one
SELECT
    u.id,
    u.organization_id,
    u.email,
    u.name,
    u.role,
    u.preferences,
    u.created_at,
    u.updated_at,
    o.id as org_id,
    o.name as org_name,
    o.slug as org_slug,
    o.tier as org_tier,
    o.settings as org_settings,
    o.encryption_key_id as org_encryption_key_id
FROM users u
JOIN organizations o ON u.organization_id = o.id
WHERE u.id = $1;

-- name: ListUsersByOrganization :many
SELECT * FROM users
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UserExistsByID :one
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1);

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: CountUsersByOrganization :one
SELECT COUNT(*) FROM users WHERE organization_id = $1;

-- name: UpdateUserPreferences :exec
UPDATE users SET preferences = $2, updated_at = NOW() WHERE id = $1;

-- name: GetUserPreferences :one
SELECT preferences FROM users WHERE id = $1;

-- name: CountAllUsers :one
SELECT COUNT(*) FROM users;

-- name: CountUsersCreatedSince :one
SELECT COUNT(*) FROM users WHERE created_at >= $1;
