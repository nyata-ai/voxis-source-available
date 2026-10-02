package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MCPProtocolVersionHeader is the canonical header name defined by the MCP
// Streamable HTTP transport spec.
const MCPProtocolVersionHeader = "MCP-Protocol-Version"

// MCPProtocolVersion enforces MCP spec 2025-06-18 §2.7: when the client
// sends an `MCP-Protocol-Version` header, the server MUST reject any value
// outside its supported set with HTTP 400. A missing header is allowed —
// the spec says servers should assume `2025-03-26` for backward compatibility.
//
// supported is the allowlist of accepted version strings. Passing nil or an
// empty slice disables enforcement (no-op middleware) so deployments can opt
// out during a migration window.
func MCPProtocolVersion(supported []string) gin.HandlerFunc {
	if len(supported) == 0 {
		return func(c *gin.Context) { c.Next() }
	}
	allow := make(map[string]struct{}, len(supported))
	for _, v := range supported {
		allow[v] = struct{}{}
	}
	return func(c *gin.Context) {
		version := c.GetHeader(MCPProtocolVersionHeader)
		if version == "" {
			c.Next()
			return
		}
		if _, ok := allow[version]; !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":   "bad_request",
				"message": "unsupported MCP-Protocol-Version: " + version,
			})
			return
		}
		c.Next()
	}
}
