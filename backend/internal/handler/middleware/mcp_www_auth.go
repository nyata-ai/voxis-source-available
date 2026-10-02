package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// MCPWWWAuthenticate stamps a `WWW-Authenticate: Bearer resource_metadata="<url>"`
// header on every response from /mcp. The MCP spec (2025-06-18 §2.3) requires
// this header on 401 responses so clients can discover the Protected Resource
// Metadata document. RFC 9728 §5.1 defines the exact format.
//
// We pre-stamp the header unconditionally because:
//   - It must appear on 401s emitted by downstream middleware (rate limit,
//     auth, body cap). Pre-stamping is simpler than wrapping the response
//     writer to inject on status change.
//   - Including the header on 200 responses is spec-compliant; it just lets
//     clients introspect the auth server without provoking a failure.
//
// When metadataURL is empty (e.g. PUBLIC_BASE_URL unset during early bootstrap)
// the middleware is a no-op so we don't emit a malformed `Bearer resource_metadata=""`.
func MCPWWWAuthenticate(metadataURL string) gin.HandlerFunc {
	if metadataURL == "" {
		return func(c *gin.Context) { c.Next() }
	}
	header := fmt.Sprintf("Bearer resource_metadata=%q", metadataURL)
	return func(c *gin.Context) {
		c.Header("WWW-Authenticate", header)
		c.Next()
	}
}
