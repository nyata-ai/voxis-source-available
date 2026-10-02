package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/service"
)

// ResolveMCPOrg sits between DualAuth and AuthBridge in the MCP middleware chain.
// For JWT-authenticated requests it lazily creates (or fetches) the user's
// personal organization via the OrganizationService and stamps the resulting
// org_id onto the Gin context. AuthBridge then propagates org_id into
// request.Context() so MCP tool handlers can resolve it.
//
// API-key requests already have org_id set by DualAuth; for them this
// middleware is a pass-through.
//
// Why this exists: REST handlers resolve the org at the handler boundary via
// helpers.resolveOrgIDOrAPIKey, but /mcp is mounted as a raw http.Handler
// (gin.WrapH on the MCP SDK) so it never reaches a Gin handler. Without this
// middleware every MCP tool would fail with "organization ID not found in
// context" for JWT-authenticated users.
func ResolveMCPOrg(orgService *service.OrganizationService, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		authMethod, _ := c.Get("auth_method")
		if authMethod != "jwt" {
			c.Next()
			return
		}

		claims := GetClaims(c)
		if claims == nil {
			c.Next()
			return
		}

		// Validate claim shape locally so we can return 400 invalid_token for
		// real token problems and reserve 500 for infrastructure failures.
		// CreateUserWithOrg also validates internally, but its error type does
		// not currently distinguish "bad claims" from "DB down" — gating here
		// is the cheapest way to keep that distinction visible to clients.
		if claims.Subject == "" || claims.Email == "" {
			logger.Warn("mcp: jwt claims missing subject or email",
				"subject", claims.Subject,
				"request_id", c.GetString("request_id"),
			)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_token",
				"message": "token missing required subject or email claim",
			})
			return
		}

		_, org, err := orgService.CreateUserWithOrg(c.Request.Context(), claims)
		if err != nil {
			// All reachable failures here are infrastructure (DB transient,
			// Vault unavailable, etc.) — claim validation happened above.
			logger.Error("mcp: org resolution failed",
				"error", err,
				"subject", claims.Subject,
				"request_id", c.GetString("request_id"),
			)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "failed to resolve organization for user",
			})
			return
		}
		if org == nil {
			logger.Error("mcp: org service returned nil organization",
				"subject", claims.Subject,
				"request_id", c.GetString("request_id"),
			)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "no organization for user",
			})
			return
		}

		c.Set("org_id", org.ID)
		c.Next()
	}
}
