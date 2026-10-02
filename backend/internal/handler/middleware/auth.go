package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

const (
	// ClaimsContextKey is the key for storing claims in Gin context.
	ClaimsContextKey = "auth_claims"
	// UserIDContextKey is the key for storing user ID in Gin context.
	UserIDContextKey = "user_id"
	// AdminRole is the realm role required for backend admin endpoints.
	AdminRole = "voxis-admin"
	// UserRole is the realm role every JWT caller of the REST API and /mcp
	// must hold. Admin endpoints check AdminRole instead.
	UserRole = "voxis-user"
)

// AuthConfig holds configuration for the auth middleware.
type AuthConfig struct {
	// RequireEmailVerified requires the email_verified claim to be true.
	RequireEmailVerified bool
	// Logger for auth events. If nil, uses slog.Default().
	Logger *slog.Logger
}

// Auth creates authentication middleware that validates JWT tokens.
func Auth(verifier port.TokenVerifier) gin.HandlerFunc {
	return AuthWithConfig(verifier, AuthConfig{})
}

// AuthWithConfig creates authentication middleware with custom configuration.
func AuthWithConfig(verifier port.TokenVerifier, cfg AuthConfig) gin.HandlerFunc {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		requestID := c.GetString("request_id")

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "missing authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid authorization header format, expected 'Bearer <token>'",
			})
			return
		}

		claims, err := verifier.Verify(c.Request.Context(), parts[1])
		if err != nil {
			// Log verification failure for debugging (without token content)
			logger.Debug("token verification failed",
				"request_id", requestID,
				"error", err.Error(),
				"path", c.Request.URL.Path,
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid or expired token",
			})
			return
		}

		// Optionally require email verification
		if cfg.RequireEmailVerified && !claims.EmailVerified {
			logger.Info("access denied: email not verified",
				"request_id", requestID,
				"subject", claims.Subject,
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "email verification required",
			})
			return
		}

		setJWTContext(c, claims)

		c.Next()
	}
}

// RequireRole creates middleware that checks for a specific realm role.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "no authentication claims found",
			})
			return
		}

		if !claims.HasRole(role) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "insufficient permissions",
			})
			return
		}

		c.Next()
	}
}

// RequireAdmin creates middleware that requires JWT auth with the admin role.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("auth_method") != "jwt" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "this endpoint requires JWT authentication",
			})
			return
		}

		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "no authentication claims found",
			})
			return
		}

		if !claims.HasRole(AdminRole) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "insufficient permissions",
			})
			return
		}

		c.Next()
	}
}

// RequireAudience requires JWT claims to include the expected API audience.
func RequireAudience(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "required API audience is not configured",
			})
			return
		}

		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "no authentication claims found",
			})
			return
		}

		for _, aud := range claims.Audience {
			if aud == expected {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "token audience does not include required API audience",
		})
	}
}

// RequireAuthorizedParty requires JWTs minted for a trusted OAuth client.
func RequireAuthorizedParty(allowed []string) gin.HandlerFunc {
	allowedSet := make(map[string]bool, len(allowed))
	for _, clientID := range allowed {
		clientID = strings.TrimSpace(clientID)
		if clientID != "" {
			allowedSet[clientID] = true
		}
	}

	return func(c *gin.Context) {
		if len(allowedSet) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "admin OAuth client allowlist is not configured",
			})
			return
		}

		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "no authentication claims found",
			})
			return
		}

		if allowedSet[claims.AuthorizedParty] {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "token was not issued to an allowed admin client",
		})
	}
}

// RequireRESTAudienceAndClient requires JWT REST callers to present the
// first-party API audience and to come from an allowed OAuth client. API keys
// are bypassed because their authorization is enforced through stored scopes.
func RequireRESTAudienceAndClient(expectedAudience string, allowedClients []string) gin.HandlerFunc {
	allowedSet := make(map[string]bool, len(allowedClients))
	for _, clientID := range allowedClients {
		clientID = strings.TrimSpace(clientID)
		if clientID != "" {
			allowedSet[clientID] = true
		}
	}

	return func(c *gin.Context) {
		if c.GetString("auth_method") == "api_key" {
			c.Next()
			return
		}

		if expectedAudience == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "required REST audience is not configured",
			})
			return
		}
		if len(allowedSet) == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "allowed REST client list is not configured",
			})
			return
		}

		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "no authentication claims found",
			})
			return
		}

		for _, aud := range claims.Audience {
			if aud == expectedAudience {
				if allowedSet[claims.AuthorizedParty] {
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "forbidden",
					"message": "token was not issued to an allowed REST client",
				})
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "token audience does not include required REST audience",
		})
	}
}

// RequireEmailVerified creates middleware that checks email is verified.
// API key auth is exempted: keys are created via JWT-only endpoints by verified users.
func RequireEmailVerified() gin.HandlerFunc {
	return func(c *gin.Context) {
		// API key auth implies the user is already verified (keys are created via RequireJWT)
		if c.GetString("auth_method") == "api_key" {
			c.Next()
			return
		}

		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "no authentication claims found",
			})
			return
		}

		if !claims.EmailVerified {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "email verification required",
			})
			return
		}

		c.Next()
	}
}

func setJWTContext(c *gin.Context, claims *port.TokenClaims) {
	c.Set(ClaimsContextKey, claims)
	c.Set(UserIDContextKey, claims.Subject)
	c.Set("auth_method", "jwt")
	c.Set("subject", claims.Subject)
	if authzScopes := filterAuthorizationScopes(claims.Scopes); len(authzScopes) > 0 {
		c.Set("scopes", authzScopes)
	}
}

// GetClaims retrieves TokenClaims from Gin context.
func GetClaims(c *gin.Context) *port.TokenClaims {
	claims, exists := c.Get(ClaimsContextKey)
	if !exists {
		return nil
	}
	if tc, ok := claims.(*port.TokenClaims); ok {
		return tc
	}
	return nil
}

// GetUserID retrieves the user ID from Gin context.
func GetUserID(c *gin.Context) string {
	return c.GetString(UserIDContextKey)
}

// contextKey is the type for request.Context() value keys (avoids collisions with string keys).
type contextKey string

const (
	// CtxKeyOrgID is the request context key for organization ID.
	CtxKeyOrgID contextKey = "org_id"
	// CtxKeyScopeList is the request context key for API key scopes.
	CtxKeyScopeList contextKey = "scopes"
	// CtxKeyAuthMethod is the request context key for auth method ("api_key" or "jwt").
	CtxKeyAuthMethod contextKey = "auth_method"
	// CtxKeyAPIKeyID is the request context key for the authenticating API key's ID.
	// Empty for JWT-authenticated requests.
	CtxKeyAPIKeyID contextKey = "api_key_id"
	// CtxKeySubject is the request context key for the authenticated user subject.
	CtxKeySubject contextKey = "subject"
	// CtxKeyRequestID is the request context key for the per-request correlation ID
	// produced by middleware.RequestID. Empty if no RequestID middleware ran.
	CtxKeyRequestID contextKey = "request_id"
)

// extractBearerToken extracts the token from an "Authorization: Bearer <token>" header.
// Returns empty string if the header is missing or malformed.
func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return ""
	}
	return parts[1]
}

// DualAuth creates middleware that authenticates via JWT or API key.
// Tokens starting with "vxs_" are treated as API keys; all others go through
// the standard OIDC/JWT verification path. JWT callers must hold UserRole;
// API keys are checked against their owner's account by APIKeyService.
func DualAuth(verifier port.TokenVerifier, apiKeySvc *service.APIKeyService) gin.HandlerFunc {
	logger := slog.Default()

	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "missing or invalid authorization header",
			})
			return
		}

		if strings.HasPrefix(token, "vxs_") {
			authenticateAPIKey(c, apiKeySvc, token, logger)
			return
		}
		authenticateJWT(c, verifier, token, logger)
	}
}

// authenticateAPIKey resolves a "vxs_" bearer token and populates the Gin
// context, or aborts with 401. Every failure, including an owner check that
// could not complete, is reported identically.
func authenticateAPIKey(c *gin.Context, apiKeySvc *service.APIKeyService, token string, logger *slog.Logger) {
	key, err := apiKeySvc.Authenticate(c.Request.Context(), token)
	if err != nil {
		logger.Debug("API key authentication failed",
			"request_id", c.GetString("request_id"),
			"error", err.Error(),
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "invalid or expired API key",
		})
		return
	}

	c.Set(ClaimsContextKey, &port.TokenClaims{Subject: key.CreatedBy})
	c.Set(UserIDContextKey, key.CreatedBy)
	c.Set("auth_method", "api_key")
	c.Set("org_id", key.OrganizationID)
	c.Set("scopes", key.Scopes)
	c.Set("subject", key.CreatedBy)
	c.Set("api_key_id", key.ID)
	c.Next()
}

// authenticateJWT verifies an OIDC access token, requires UserRole, and
// populates the Gin context, or aborts with 401/403.
func authenticateJWT(c *gin.Context, verifier port.TokenVerifier, token string, logger *slog.Logger) {
	requestID := c.GetString("request_id")
	claims, err := verifier.Verify(c.Request.Context(), token)
	if err != nil {
		logger.Debug("token verification failed",
			"request_id", requestID,
			"error", err.Error(),
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "invalid or expired token",
		})
		return
	}

	// A verified realm account is not enough: the operator grants access by
	// assigning UserRole, and removing it must revoke access.
	if !claims.HasRole(UserRole) {
		logger.Info("access denied: missing realm role",
			"request_id", requestID,
			"subject", claims.Subject,
			"role", UserRole,
		)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "role_required",
			"message": "your account does not have access to Voxis; ask an administrator to grant the " + UserRole + " role",
		})
		return
	}

	// Set auth method and subject so downstream middleware (rate limiters,
	// audit, MCP bucket key) can identify the user. org_id and api_key_id
	// are not set here — JWT users resolve org via orgService at the
	// handler boundary, not at auth time.
	// Propagate ONLY Voxis-namespace scopes (those containing ":") so
	// RequireScope can enforce least-privilege for JWT-auth too. OIDC
	// identity scopes (openid, profile, email, address, phone,
	// offline_access) are identity attributes, not authorization scopes;
	// surfacing them here would make RequireScope reject every
	// JWT-auth REST request because none of them match "media:read" etc.
	// When no Voxis scopes are present (the current frontend reality),
	// `scopes` stays unset and RequireScope falls through to legacy JWT
	// bypass.
	setJWTContext(c, claims)
	c.Next()
}

// filterAuthorizationScopes drops OIDC identity scopes from a JWT scope
// claim, returning only Voxis-style authorization scopes (those containing
// the `:` namespace separator, e.g. "media:read"). Used by DualAuth so the
// `scopes` context key never carries identity scopes that would trip
// RequireScope's membership check.
func filterAuthorizationScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if strings.Contains(s, ":") {
			out = append(out, s)
		}
	}
	return out
}

// RequireScope creates middleware that checks if the request has the required
// scope. API keys carry their granted scopes in the `scopes` context key;
// JWT requests get their scopes populated by DualAuth when the token contains
// an OAuth `scope` claim. JWT requests without any scopes (legacy tokens, or
// when no Keycloak scope mapping exists) fall through to "full access" to
// preserve backward compatibility with the existing frontend flow.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scopesVal, hasScopes := c.Get("scopes")
		scopes, ok := scopesVal.([]string)
		if !hasScopes || !ok || len(scopes) == 0 {
			// API-key auth always sets scopes (possibly empty slice). If empty,
			// deny — an API key with no scopes can't call anything. JWT auth
			// without scopes is treated as full access (legacy).
			authMethod, _ := c.Get("auth_method")
			if authMethod == "api_key" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "forbidden",
					"message": "missing required scope: " + scope,
				})
				return
			}
			c.Next()
			return
		}
		for _, s := range scopes {
			if s == scope {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "missing required scope: " + scope,
		})
	}
}

// RequireJWT creates middleware that rejects API key auth (only allows JWT).
// Use this for sensitive operations like API key management to prevent privilege escalation.
func RequireJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		authMethod, _ := c.Get("auth_method")
		if authMethod == "api_key" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "this endpoint requires JWT authentication",
			})
			return
		}
		c.Next()
	}
}

// AuthBridge propagates authentication values from Gin context to request.Context().
// This allows downstream code that uses standard context (e.g., MCP handlers) to
// access org_id, scopes, and auth_method.
func AuthBridge() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		if orgID, ok := c.Get("org_id"); ok {
			ctx = context.WithValue(ctx, CtxKeyOrgID, orgID)
		}
		if scopes, ok := c.Get("scopes"); ok {
			ctx = context.WithValue(ctx, CtxKeyScopeList, scopes)
		}
		if method, ok := c.Get("auth_method"); ok {
			ctx = context.WithValue(ctx, CtxKeyAuthMethod, method)
		}
		if keyID, ok := c.Get("api_key_id"); ok {
			ctx = context.WithValue(ctx, CtxKeyAPIKeyID, keyID)
		}
		if subject, ok := c.Get("subject"); ok {
			ctx = context.WithValue(ctx, CtxKeySubject, subject)
		}
		if rid := c.GetString("request_id"); rid != "" {
			ctx = context.WithValue(ctx, CtxKeyRequestID, rid)
		}

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ValidateOrigin creates middleware that rejects requests with a disallowed Origin header.
// Requests without an Origin header are allowed through (server-to-server).
// Matching is case-insensitive on the entire origin string (RFC 6454 hosts
// are lowercase, but proxies/clients sometimes preserve casing).
func ValidateOrigin(allowed []string) gin.HandlerFunc {
	allowSet := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		allowSet[strings.ToLower(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if !allowSet[strings.ToLower(origin)] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "origin not allowed",
			})
			return
		}

		c.Next()
	}
}
