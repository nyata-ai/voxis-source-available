package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MCPRequireResourceAudience enforces RFC 8707 Resource Indicators on
// JWT-authenticated MCP requests. When requiredAudience is non-empty, the
// middleware rejects requests whose verified token does not list that
// audience in its `aud` claim. API-key requests are bypassed because they
// are not OAuth bearer tokens and have no audience binding.
//
// MCP spec 2025-06-18 §2.4 — "MCP servers MUST validate that access tokens
// were issued specifically for them as the intended audience" — combined
// with the requirement that clients send `resource=<canonical-uri>` in
// authorization and token requests.
//
// When requiredAudience is empty, the middleware is a no-op so deployments
// that have not yet wired an RFC 8707 audience mapper in their authorization
// server are not broken by this code. Set MCP_RESOURCE_AUDIENCE (typically
// the canonical MCP URL) to turn enforcement on.
func MCPRequireResourceAudience(requiredAudience string) gin.HandlerFunc {
	if requiredAudience == "" {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		method, _ := c.Get("auth_method")
		if method == "api_key" {
			c.Next()
			return
		}
		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "missing authentication claims",
			})
			return
		}
		for _, aud := range claims.Audience {
			if aud == requiredAudience {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "token audience does not include required resource",
		})
	}
}
