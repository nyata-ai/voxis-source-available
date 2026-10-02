-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, tier)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations WHERE id = $1;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations WHERE slug = $1;

-- name: UpdateOrganizationEncryptionKeyID :one
UPDATE organizations
SET encryption_key_id = $2
WHERE id = $1
RETURNING *;

-- name: UpdateOrganization :one
UPDATE organizations
SET name = $2, settings = $3, tier = $4
WHERE id = $1
RETURNING *;

-- name: DeleteOrganization :exec
DELETE FROM organizations WHERE id = $1;

-- name: CountOrganizations :one
SELECT COUNT(*) FROM organizations;
