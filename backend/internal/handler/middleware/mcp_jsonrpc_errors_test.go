package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rpcError mirrors the JSON-RPC 2.0 error envelope the spec mandates for any
// transport-level failure that occurred while the client was speaking JSON-RPC.
type rpcError struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestMCPJSONRPCErrorEnvelope_RewritesUnauthorized(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MCPJSONRPCErrorEnvelope())
	router.POST("/mcp", func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "missing token"})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body rpcError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "2.0", body.JSONRPC)
	assert.Equal(t, -32001, body.Error.Code)
	assert.Contains(t, body.Error.Message, "missing token")
	assert.Nil(t, body.ID)
}

func TestMCPJSONRPCErrorEnvelope_RewritesRateLimited(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MCPJSONRPCErrorEnvelope())
	router.POST("/mcp", func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	var body rpcError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "2.0", body.JSONRPC)
}

func TestMCPJSONRPCErrorEnvelope_PreservesSuccessBody(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MCPJSONRPCErrorEnvelope())
	router.POST("/mcp", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "result": gin.H{"ok": true}, "id": 1})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Success body must be passed through unchanged so JSON-RPC clients see
	// the real result envelope from the MCP SDK.
	assert.Contains(t, rec.Body.String(), `"result"`)
	assert.Contains(t, rec.Body.String(), `"ok"`)
}

func TestMCPJSONRPCErrorEnvelope_FlushIsNoOpUntilEnd(t *testing.T) {
	// If downstream middleware (or the SDK) ever calls Flush() while we're
	// buffering, our defer must still produce a valid envelope. The wrapper's
	// Flush override is a no-op for exactly this reason.
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MCPJSONRPCErrorEnvelope())
	router.POST("/mcp", func(c *gin.Context) {
		c.Status(http.StatusUnauthorized)
		// Simulate a downstream component that tries to flush mid-response.
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = c.Writer.WriteString(`{"error":"unauthorized"}`)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), `"jsonrpc":"2.0"`)
	assert.Contains(t, rec.Body.String(), "unauthorized")
}

func TestMCPJSONRPCErrorEnvelope_FlushesAfterDownstreamPanic(t *testing.T) {
	// Panic during downstream handler: gin.Recovery (mounted in main as one of
	// the outer middlewares) catches it and writes a 500 into our buffered
	// writer. The envelope wrapper must still flush — wrapped in a defer so
	// the stack-unwind path produces a valid JSON-RPC error body, not a
	// zero-byte response.
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(MCPJSONRPCErrorEnvelope())
	router.POST("/mcp", func(_ *gin.Context) {
		panic("simulated downstream failure")
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code, "Recovery should commit 500")
	assert.NotEmpty(t, rec.Body.Bytes(), "envelope must still flush after panic")
	assert.Contains(t, rec.Body.String(), `"jsonrpc":"2.0"`, "body must be a JSON-RPC envelope")
	assert.Contains(t, rec.Body.String(), `"id":null`)
}

func TestMCPJSONRPCErrorEnvelope_PreservesHeaders(t *testing.T) {
	// WWW-Authenticate must survive the body rewrite so clients can still
	// discover the auth server on 401.
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(MCPJSONRPCErrorEnvelope())
	router.Use(func(c *gin.Context) {
		c.Header("WWW-Authenticate", `Bearer resource_metadata="https://x"`)
		c.Next()
	})
	router.POST("/mcp", func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "resource_metadata=")
}
