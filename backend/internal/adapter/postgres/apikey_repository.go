package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// APIKeyRepository implements port.APIKeyRepository using PostgreSQL.
type APIKeyRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewAPIKeyRepository creates a new PostgreSQL API key repository.
func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

var _ port.APIKeyRepository = (*APIKeyRepository)(nil)

// Create persists a new API key. Returns domain.ErrConflict on key_prefix collision.
func (r *APIKeyRepository) Create(ctx context.Context, key *domain.APIKey) error {
	orgUUID, err := parseUUID(key.OrganizationID)
	if err != nil {
		return err
	}

	var expiresAt pgtype.Timestamptz
	if key.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: *key.ExpiresAt, Valid: true}
	}

	row, err := r.queries.CreateAPIKey(ctx, sqlcdb.CreateAPIKeyParams{
		OrganizationID: orgUUID,
		CreatedBy:      key.CreatedBy,
		Name:           key.Name,
		KeyPrefix:      key.KeyPrefix,
		KeyHash:        key.KeyHash,
		Scopes:         key.Scopes,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	// Update key with DB-generated values
	key.ID = uuidToString(row.ID)
	key.CreatedAt = row.CreatedAt
	key.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByPrefix retrieves an active API key by its prefix.
// Returns domain.ErrNotFound if not found or revoked.
func (r *APIKeyRepository) GetByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	row, err := r.queries.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return apiKeyToDomain(row), nil
}

// ListByOrg returns active (non-revoked) API keys for an organization.
func (r *APIKeyRepository) ListByOrg(ctx context.Context, orgID string) ([]*domain.APIKey, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, err
	}

	rows, err := r.queries.ListAPIKeysByOrg(ctx, orgUUID)
	if err != nil {
		return nil, err
	}

	keys := make([]*domain.APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, apiKeyToDomain(row))
	}
	return keys, nil
}

// CountByOrg returns the number of active API keys for an organization.
func (r *APIKeyRepository) CountByOrg(ctx context.Context, orgID string) (int, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, err
	}

	count, err := r.queries.CountAPIKeysByOrg(ctx, orgUUID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// Revoke marks an API key as revoked. Returns domain.ErrNotFound if not found or already revoked.
func (r *APIKeyRepository) Revoke(ctx context.Context, id, orgID string) error {
	idUUID, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return domain.ErrNotFound
	}

	// Use the pool directly to check rows affected, since sqlcdb's RevokeAPIKey
	// is :exec and discards the CommandTag.
	tag, err := r.pool.Exec(ctx,
		"UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL",
		idUUID, orgUUID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateLastUsed updates the last_used_at timestamp.
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	idUUID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return r.queries.UpdateAPIKeyLastUsed(ctx, idUUID)
}

// apiKeyToDomain converts a sqlcdb.ApiKey row to a domain.APIKey.
func apiKeyToDomain(row sqlcdb.ApiKey) *domain.APIKey {
	key := &domain.APIKey{
		ID:             uuidToString(row.ID),
		OrganizationID: uuidToString(row.OrganizationID),
		CreatedBy:      row.CreatedBy,
		Name:           row.Name,
		KeyPrefix:      row.KeyPrefix,
		KeyHash:        row.KeyHash,
		Scopes:         row.Scopes,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}

	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		key.LastUsedAt = &t
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		key.ExpiresAt = &t
	}
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		key.RevokedAt = &t
	}

	return key
}
