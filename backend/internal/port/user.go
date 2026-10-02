package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// UserRepository handles user persistence.
// Note: User ID is the Keycloak subject (sub claim).
type UserRepository interface {
	// GetByID retrieves a user by their Keycloak subject ID.
	// Returns domain.ErrNotFound if the user doesn't exist.
	GetByID(ctx context.Context, id string) (*domain.User, error)

	// Create creates a new user. The user.ID must be set to the Keycloak sub.
	Create(ctx context.Context, user *domain.User) error

	// Update updates an existing user's profile.
	Update(ctx context.Context, user *domain.User) error

	// Exists checks if a user with the given ID exists.
	Exists(ctx context.Context, id string) (bool, error)

	// GetWithOrganization retrieves a user with their organization details.
	GetWithOrganization(ctx context.Context, id string) (*domain.User, *domain.Organization, error)

	// UpdatePreferences persists the user's JSONB preferences blob.
	UpdatePreferences(ctx context.Context, userID string, prefs []byte) error

	// GetPreferences returns the raw JSONB preferences for a user.
	GetPreferences(ctx context.Context, userID string) ([]byte, error)
}

// NormalizedEmailRepository checks whether a provisioned user already exists
// under an email's normalized alias form (see domain.NormalizeEmailAlias).
// Kept separate from UserRepository, like OrgMemberLister, so callers that
// don't need alias-farming dedup (most of them) aren't forced to implement
// it; RegistrationService type-asserts for it optionally.
type NormalizedEmailRepository interface {
	// ExistsByNormalizedEmail reports whether any user's normalized_email
	// column matches normalizedEmail exactly.
	ExistsByNormalizedEmail(ctx context.Context, normalizedEmail string) (bool, error)
}

// OrgMemberLister lists the members of an organization, one page at a time.
//
// Kept off UserRepository on purpose. Its only consumers are the credit-expiry
// notifiers, which need addresses and nothing else; folding an org-wide listing
// into the interface every handler and service already depends on would widen
// all of them for one caller.
type OrgMemberLister interface {
	// ListByOrganization returns at most limit members starting at offset. Both
	// are clamped by the implementation, so a caller cannot ask for an unbounded
	// page. An organization with no members is an empty slice, not an error.
	ListByOrganization(ctx context.Context, orgID string, limit, offset int) ([]*domain.User, error)
}
