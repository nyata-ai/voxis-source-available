package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time interface compliance checks.
var _ port.UserRepository = (*UserRepository)(nil)
var _ port.OrgMemberLister = (*UserRepository)(nil)
var _ port.NormalizedEmailRepository = (*UserRepository)(nil)

// UserRepository implements port.UserRepository using PostgreSQL.
type UserRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewUserRepository creates a new PostgreSQL user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// GetByID retrieves a user by their Keycloak subject ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(row), nil
}

// Create creates a new user.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	orgUUID, err := parseUUID(user.OrganizationID)
	if err != nil {
		return err
	}

	row, err := r.queries.CreateUser(ctx, sqlcdb.CreateUserParams{
		ID:              user.ID,
		OrganizationID:  orgUUID,
		Email:           user.Email,
		Name:            user.Name,
		NormalizedEmail: normalizedEmailParam(user.Email),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	// Update user with DB-generated timestamps
	user.CreatedAt = row.CreatedAt
	user.UpdatedAt = row.UpdatedAt
	user.Role = row.Role
	return nil
}

// Update updates an existing user's profile.
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	row, err := r.queries.UpdateUser(ctx, sqlcdb.UpdateUserParams{
		ID:              user.ID,
		Email:           user.Email,
		Name:            user.Name,
		NormalizedEmail: normalizedEmailParam(user.Email),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	user.UpdatedAt = row.UpdatedAt
	return nil
}

// Exists checks if a user with the given ID exists.
func (r *UserRepository) Exists(ctx context.Context, id string) (bool, error) {
	return r.queries.UserExistsByID(ctx, id)
}

// GetWithOrganization retrieves a user with their organization details.
func (r *UserRepository) GetWithOrganization(ctx context.Context, id string) (*domain.User, *domain.Organization, error) {
	row, err := r.queries.GetUserWithOrganization(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}

	user := &domain.User{
		ID:             row.ID,
		OrganizationID: uuidToString(row.OrganizationID),
		Email:          row.Email,
		Name:           row.Name,
		Role:           row.Role,
		Preferences:    parsePreferences(row.Preferences),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}

	org := &domain.Organization{
		ID:              uuidToString(row.OrgID),
		Name:            row.OrgName,
		Slug:            row.OrgSlug,
		Tier:            row.OrgTier,
		EncryptionKeyID: textToString(row.OrgEncryptionKeyID),
	}

	return user, org, nil
}

// UpdatePreferences persists the user's JSONB preferences blob.
func (r *UserRepository) UpdatePreferences(ctx context.Context, userID string, prefs []byte) error {
	return r.queries.UpdateUserPreferences(ctx, sqlcdb.UpdateUserPreferencesParams{
		ID:          userID,
		Preferences: prefs,
	})
}

// maxOrgMemberPage bounds one page of an organization's members. It is a clamp,
// not a default: the generated query takes int32, so an unclamped caller value
// could be cast into a negative LIMIT.
const maxOrgMemberPage = 500

// maxOrgMemberOffset bounds how far into an organization's members a caller may
// page. Same reason as the LIMIT clamp, and it is far beyond any real tenant.
const maxOrgMemberOffset = 100_000

// ListByOrganization returns one page of an organization's members, newest
// first. An organization with no members yields an empty slice, not an error.
//
// Ordering is created_at DESC, which is what the existing query provides. Two
// members created in the same microsecond could in principle swap between pages;
// that is a theoretical tie in a TIMESTAMPTZ written by separate transactions,
// and the only consumer pages over tenants of a handful of users. Switching this
// walk to keyset pagination (ORDER BY created_at DESC, id DESC with a
// WHERE (created_at, id) < (...) cursor) would settle both the tie and the
// caller's page bound; it needs a new query and an sqlc regeneration, so it is
// recorded here as the direction rather than done in passing.
//
// The limit is clamped, so a caller may receive fewer rows than it asked for
// with members still to come. A page shorter than the limit therefore does not
// mean the listing is exhausted — only an empty page does.
func (r *UserRepository) ListByOrganization(
	ctx context.Context, orgID string, limit, offset int,
) ([]*domain.User, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxOrgMemberPage {
		limit = maxOrgMemberPage
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxOrgMemberOffset {
		// Past the paging bound is exhaustion, not a wrap to the first page.
		// Clamping to 0 here would hand a walking caller page 1 again, which
		// reads as "more members" and walks forever.
		return []*domain.User{}, nil
	}

	rows, err := r.queries.ListUsersByOrganization(ctx, sqlcdb.ListUsersByOrganizationParams{
		OrganizationID: orgUUID,
		Limit:          int32(limit),  //nolint:gosec // clamped above
		Offset:         int32(offset), //nolint:gosec // clamped above
	})
	if err != nil {
		return nil, fmt.Errorf("list users by organization: %w", err)
	}

	users := make([]*domain.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, r.toDomain(row))
	}
	return users, nil
}

// ExistsByNormalizedEmail reports whether a provisioned user already occupies
// normalizedEmail's alias form (domain.NormalizeEmailAlias), used by
// registration's alias-farming dedup guard. The caller passes an already
// normalized value — unlike Create/Update, this method does not re-derive it
// from a raw address.
func (r *UserRepository) ExistsByNormalizedEmail(ctx context.Context, normalizedEmail string) (bool, error) {
	if normalizedEmail == "" {
		return false, nil
	}
	return r.queries.ExistsByNormalizedEmail(ctx, pgtype.Text{String: normalizedEmail, Valid: true})
}

// GetPreferences returns the raw JSONB preferences for a user.
func (r *UserRepository) GetPreferences(ctx context.Context, userID string) ([]byte, error) {
	data, err := r.queries.GetUserPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

func (r *UserRepository) toDomain(row sqlcdb.User) *domain.User {
	return &domain.User{
		ID:             row.ID,
		OrganizationID: uuidToString(row.OrganizationID),
		Email:          row.Email,
		Name:           row.Name,
		Role:           row.Role,
		Preferences:    parsePreferences(row.Preferences),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

// parsePreferences unmarshals JSONB preferences and merges with defaults.
// Falls back to DefaultPreferences on unmarshal error.
func parsePreferences(data []byte) domain.UserPreferences {
	if len(data) == 0 {
		return domain.DefaultPreferences()
	}
	var prefs domain.UserPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		slog.Warn("failed to unmarshal user preferences, using defaults", "error", err)
		return domain.DefaultPreferences()
	}
	return prefs.MergeWithDefaults()
}

// Helper functions for pgtype conversions

func parseUUID(s string) (pgtype.UUID, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(s); err != nil {
		return uuid, err
	}
	return uuid, nil
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return formatUUID(u.Bytes)
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func textToString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// normalizedEmailParam derives the alias-dedup key from a raw email and wraps
// it for the nullable normalized_email column. An empty raw email (should not
// happen — domain.NewUser rejects it) leaves the column NULL rather than
// writing a key that would spuriously collide with other empty-email rows.
func normalizedEmailParam(rawEmail string) pgtype.Text {
	normalized := domain.NormalizeEmailAlias(rawEmail)
	if normalized == "" || normalized == "@" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: normalized, Valid: true}
}
