package domain

import (
	"fmt"
	"time"
)

// ValidScopes defines the allowed API key scopes.
var ValidScopes = []string{
	"media:read", "media:write",
	"transcription:read", "transcription:write",
	"summary:read", "summary:write",
	"export:read",
}

// MaxAPIKeysPerOrg is the maximum number of active API keys per organization.
const MaxAPIKeysPerOrg = 10

// APIKey represents an API key for programmatic access.
type APIKey struct {
	ID             string
	OrganizationID string
	CreatedBy      string
	Name           string
	KeyPrefix      string
	KeyHash        string
	Scopes         []string
	LastUsedAt     *time.Time
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsExpired returns true if the key has a set expiration time that is in the past.
func (k *APIKey) IsExpired() bool {
	return k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now())
}

// IsRevoked returns true if the key has been revoked.
func (k *APIKey) IsRevoked() bool {
	return k.RevokedAt != nil
}

// IsActive returns true if the key is neither revoked nor expired.
func (k *APIKey) IsActive() bool {
	return !k.IsRevoked() && !k.IsExpired()
}

// HasScope returns true if the key has the given scope.
func (k *APIKey) HasScope(scope string) bool {
	for _, s := range k.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ValidateScopes checks that all provided scopes are valid and non-empty.
func ValidateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return fmt.Errorf("at least one scope required")
	}
	valid := make(map[string]bool, len(ValidScopes))
	for _, s := range ValidScopes {
		valid[s] = true
	}
	for _, s := range scopes {
		if !valid[s] {
			return fmt.Errorf("invalid scope: %s", s)
		}
	}
	return nil
}
