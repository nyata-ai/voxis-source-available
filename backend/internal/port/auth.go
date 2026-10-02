package port

import (
	"context"
	"time"
)

// TokenClaims represents validated JWT claims from Keycloak.
type TokenClaims struct {
	Subject           string
	Email             string
	EmailVerified     bool
	PreferredUsername string
	Name              string
	AuthTime          time.Time
	RealmRoles        []string
	ClientRoles       map[string][]string
	// Audience is the verified `aud` claim. Used by downstream middleware
	// to enforce RFC 8707 Resource Indicators (e.g. on /mcp).
	Audience []string
	// AuthorizedParty is the `azp` claim. Admin routes use it to ensure the
	// token came from a trusted first-party OAuth client, not an MCP client.
	AuthorizedParty string
	// Scopes is the space-separated `scope` claim split into entries (e.g.
	// "openid profile media:read" → ["openid","profile","media:read"]).
	// Used by MCP scope checks so JWT-auth requests can be limited to the
	// scopes the user actually granted via consent.
	Scopes []string
}

// HasRole checks if the user has a specific realm role.
func (c *TokenClaims) HasRole(role string) bool {
	for _, r := range c.RealmRoles {
		if r == role {
			return true
		}
	}
	return false
}

// HasClientRole checks if the user has a specific role for a client.
func (c *TokenClaims) HasClientRole(clientID, role string) bool {
	if roles, ok := c.ClientRoles[clientID]; ok {
		for _, r := range roles {
			if r == role {
				return true
			}
		}
	}
	return false
}

// TokenVerifier validates JWT tokens and extracts claims.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*TokenClaims, error)
}
