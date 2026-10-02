package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// APIKeyRepository handles API key persistence.
type APIKeyRepository interface {
	// Create persists a new API key. Returns domain.ErrConflict on key_prefix collision.
	Create(ctx context.Context, key *domain.APIKey) error

	// GetByPrefix retrieves an active API key by its prefix.
	// Returns domain.ErrNotFound if not found or revoked.
	GetByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error)

	// ListByOrg returns active (non-revoked) API keys for an organization.
	ListByOrg(ctx context.Context, orgID string) ([]*domain.APIKey, error)

	// CountByOrg returns the number of active API keys for an organization.
	CountByOrg(ctx context.Context, orgID string) (int, error)

	// Revoke marks an API key as revoked. Returns domain.ErrNotFound if not found.
	Revoke(ctx context.Context, id, orgID string) error

	// UpdateLastUsed updates the last_used_at timestamp.
	UpdateLastUsed(ctx context.Context, id string) error
}

// APIKeyOwnerVerifier confirms that the identity-provider account which
// created an API key may still use Voxis: it exists, is enabled, and still
// holds the required realm role.
type APIKeyOwnerVerifier interface {
	// VerifyOwner returns nil when subject may use Voxis, an error wrapping
	// domain.ErrUnauthorized when the account is missing, disabled, or lacks
	// the role, and any other error when the check could not be completed.
	VerifyOwner(ctx context.Context, subject string) error
}
