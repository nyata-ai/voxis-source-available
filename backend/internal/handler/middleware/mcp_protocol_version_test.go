package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const testSupportedProtocolVersion = "2025-06-18"

func TestMCPProtocolVersion_Missing_PassesThrough(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MCPProtocolVersion([]string{testSupportedProtocolVersion}))
	router.POST("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// Spec allows missing header (backward compat → server assumes 2025-03-26).
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMCPProtocolVersion_Supported_PassesThrough(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MCPProtocolVersion([]string{testSupportedProtocolVersion}))
	router.POST("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("MCP-Protocol-Version", testSupportedProtocolVersion)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMCPProtocolVersion_Unsupported_Returns400(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MCPProtocolVersion([]string{testSupportedProtocolVersion}))
	router.POST("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("MCP-Protocol-Version", "1900-01-01")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// MCP spec 2025-06-18 §2.7 — invalid or unsupported MUST return 400.
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, strings.ToLower(rec.Body.String()), "protocol")
}

func TestMCPProtocolVersion_EmptySupportedList_PassesThrough(t *testing.T) {
	// Defensive: an empty allowlist should not break the chain — operator
	// must have intentionally disabled enforcement.
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MCPProtocolVersion(nil))
	router.POST("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("MCP-Protocol-Version", "anything")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
