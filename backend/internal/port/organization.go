package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// OrganizationRepository handles organization persistence.
type OrganizationRepository interface {
	// Create persists a new organization.
	// The organization ID will be populated after creation.
	Create(ctx context.Context, org *domain.Organization) error

	// GetByID retrieves an organization by ID.
	// Returns domain.ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*domain.Organization, error)

	// GetBySlug retrieves an organization by slug.
	// Returns domain.ErrNotFound if not found.
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)

	// UpdateEncryptionKeyID sets the Vault KEK reference for an organization.
	UpdateEncryptionKeyID(ctx context.Context, id, keyID string) error

	// Update updates an existing organization.
	Update(ctx context.Context, org *domain.Organization) error
}

// OrganizationLookup is the read-only slice of OrganizationRepository that
// consumers needing nothing but an organization's display name depend on — the
// credit-expiry notices, which address the customer by name. Any
// OrganizationRepository satisfies it.
type OrganizationLookup interface {
	// GetByID retrieves an organization by ID.
	// Returns domain.ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*domain.Organization, error)
}
