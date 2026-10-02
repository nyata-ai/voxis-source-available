package port

import "context"

// VaultClient handles encryption key management.
type VaultClient interface {
	HealthChecker

	// GenerateKEK creates a new Key Encryption Key for an organization.
	// Returns the key ID (path in Vault).
	GenerateKEK(ctx context.Context, orgID string) (string, error)

	// GetKEK retrieves an organization's KEK (latest version).
	// Uses cache when available.
	GetKEK(ctx context.Context, orgID string) ([]byte, error)

	// GetKEKVersions retrieves all KEK versions for an organization,
	// ordered newest-first. Used by envelope decryption to handle
	// KEK rotation: if the latest version fails to unwrap a DEK,
	// older versions are tried as fallback.
	GetKEKVersions(ctx context.Context, orgID string) ([][]byte, error)

	// Close cleanly shuts down the client.
	Close() error
}
