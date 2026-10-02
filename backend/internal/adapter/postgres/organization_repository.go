package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
)

// OrganizationRepository implements port.OrganizationRepository using PostgreSQL.
type OrganizationRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewOrganizationRepository creates a new PostgreSQL organization repository.
func NewOrganizationRepository(pool *pgxpool.Pool) *OrganizationRepository {
	return &OrganizationRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// Create persists a new organization.
func (r *OrganizationRepository) Create(ctx context.Context, org *domain.Organization) error {
	row, err := r.queries.CreateOrganization(ctx, sqlcdb.CreateOrganizationParams{
		Name: org.Name,
		Slug: org.Slug,
		Tier: org.Tier,
	})
	if err != nil {
		return err
	}

	// Populate ID and timestamps from database
	org.ID = uuidToString(row.ID)
	org.CreatedAt = row.CreatedAt
	org.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID retrieves an organization by ID.
func (r *OrganizationRepository) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetOrganizationByID(ctx, uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(row), nil
}

// GetBySlug retrieves an organization by slug.
func (r *OrganizationRepository) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	row, err := r.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(row), nil
}

// UpdateEncryptionKeyID sets the Vault KEK reference for an organization.
func (r *OrganizationRepository) UpdateEncryptionKeyID(ctx context.Context, id, keyID string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	_, err = r.queries.UpdateOrganizationEncryptionKeyID(ctx, sqlcdb.UpdateOrganizationEncryptionKeyIDParams{
		ID:              uuid,
		EncryptionKeyID: pgtype.Text{String: keyID, Valid: keyID != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// Update updates an existing organization.
func (r *OrganizationRepository) Update(ctx context.Context, org *domain.Organization) error {
	uuid, err := parseUUID(org.ID)
	if err != nil {
		return domain.ErrNotFound
	}

	// Convert settings to JSON bytes (empty for now)
	settings := []byte("{}")

	row, err := r.queries.UpdateOrganization(ctx, sqlcdb.UpdateOrganizationParams{
		ID:       uuid,
		Name:     org.Name,
		Settings: settings,
		Tier:     org.Tier,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}

	org.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *OrganizationRepository) toDomain(row sqlcdb.Organization) *domain.Organization {
	return &domain.Organization{
		ID:              uuidToString(row.ID),
		Name:            row.Name,
		Slug:            row.Slug,
		Settings:        nil, // Parse from row.Settings if needed
		EncryptionKeyID: textToString(row.EncryptionKeyID),
		Tier:            row.Tier,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func timestamptzToTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
