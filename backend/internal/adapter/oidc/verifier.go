package oidc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/voxis/backend/internal/port"
)

// allowedAudiences captures the set of audiences a verifier will accept. A
// token whose `aud` claim contains ANY entry passes; a token whose `aud`
// contains none is rejected. Empty slice means "single-audience mode" using
// the legacy ClientID-based check.
type allowedAudiences []string

func (a allowedAudiences) match(tokenAudiences []string) bool {
	for _, tokenAud := range tokenAudiences {
		for _, allowed := range a {
			if tokenAud == allowed {
				return true
			}
		}
	}
	return false
}

// keycloakClaims represents the JWT claims structure from Keycloak.
type keycloakClaims struct {
	Subject           string                 `json:"sub"`
	Email             string                 `json:"email"`
	EmailVerified     bool                   `json:"email_verified"`
	PreferredUsername string                 `json:"preferred_username"`
	Name              string                 `json:"name"`
	AuthorizedParty   string                 `json:"azp"`
	AuthTime          int64                  `json:"auth_time"`
	RealmAccess       realmAccess            `json:"realm_access"`
	ResourceAccess    map[string]clientRoles `json:"resource_access"`
	// Scope is the OAuth2 `scope` claim — space-separated string per RFC 6749.
	// Parsed into TokenClaims.Scopes by Verify.
	Scope string `json:"scope"`
}

type realmAccess struct {
	Roles []string `json:"roles"`
}

type clientRoles struct {
	Roles []string `json:"roles"`
}

// Verifier implements port.TokenVerifier using go-oidc.
type Verifier struct {
	verifier  *oidc.IDTokenVerifier
	audiences allowedAudiences // non-empty when multi-audience mode is active
}

// NewVerifier creates a new OIDC token verifier with a single allowed audience.
// fetchURL is the URL to fetch OIDC configuration from.
// expectedIssuer is the issuer claim expected in tokens (may differ from fetchURL when behind NAT).
// Token validation requires `aud` to contain clientID exactly (legacy behavior).
func NewVerifier(ctx context.Context, fetchURL, expectedIssuer, clientID string) (*Verifier, error) {
	if expectedIssuer != "" && expectedIssuer != fetchURL {
		ctx = oidc.InsecureIssuerURLContext(ctx, expectedIssuer)
	}

	provider, err := oidc.NewProvider(ctx, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	return &Verifier{
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

// NewVerifierWithAudiences creates a verifier that accepts any token whose
// `aud` claim contains at least one of the supplied audiences. Use when the
// resource server needs to accept tokens minted for several OAuth clients —
// e.g. the original `voxis-api` plus a public MCP client used by external
// LLM wrappers (Claude.ai, ChatGPT). MCP spec 2025-06-18 §2.4 requires the
// resource server to validate audience binding, and this constructor lets
// callers express the allowed-audience set explicitly.
func NewVerifierWithAudiences(ctx context.Context, fetchURL, expectedIssuer string, audiences []string) (*Verifier, error) {
	if len(audiences) == 0 {
		return nil, fmt.Errorf("at least one allowed audience is required")
	}
	if expectedIssuer != "" && expectedIssuer != fetchURL {
		ctx = oidc.InsecureIssuerURLContext(ctx, expectedIssuer)
	}

	provider, err := oidc.NewProvider(ctx, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// SkipClientIDCheck disables go-oidc's built-in audience check so we can
	// run our own multi-audience comparison in Verify().
	return &Verifier{
		verifier:  provider.Verifier(&oidc.Config{SkipClientIDCheck: true}),
		audiences: append(allowedAudiences(nil), audiences...),
	}, nil
}

// Verify validates the token and extracts claims.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (*port.TokenClaims, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	if len(v.audiences) > 0 && !v.audiences.match(idToken.Audience) {
		return nil, fmt.Errorf("token verification failed: audience %v does not match any allowed audience", idToken.Audience)
	}

	var claims keycloakClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	clientRolesMap := make(map[string][]string)
	for client, roles := range claims.ResourceAccess {
		clientRolesMap[client] = roles.Roles
	}

	var authTime time.Time
	if claims.AuthTime > 0 {
		authTime = time.Unix(claims.AuthTime, 0).UTC()
	}

	return &port.TokenClaims{
		Subject:           claims.Subject,
		Email:             claims.Email,
		EmailVerified:     claims.EmailVerified,
		PreferredUsername: claims.PreferredUsername,
		Name:              claims.Name,
		AuthTime:          authTime,
		RealmRoles:        claims.RealmAccess.Roles,
		ClientRoles:       clientRolesMap,
		Audience:          append([]string(nil), idToken.Audience...),
		AuthorizedParty:   claims.AuthorizedParty,
		Scopes:            parseScopeClaim(claims.Scope),
	}, nil
}

// parseScopeClaim splits a space-separated OAuth2 scope string. Empty input
// yields nil so callers can distinguish "no scope claim" from "empty scope".
func parseScopeClaim(scope string) []string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return nil
	}
	return strings.Fields(scope)
}

var _ port.TokenVerifier = (*Verifier)(nil)
