package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestMCPWWWAuthenticate_StampsHeaderBeforeAuth(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Insert the stamp before a stub auth that returns 401 — emulates the
	// real chain order.
	router.Use(MCPWWWAuthenticate("https://voxis.example.com/.well-known/oauth-protected-resource"))
	router.Use(func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	wwwAuth := rec.Header().Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer")
	assert.Contains(t, wwwAuth, `resource_metadata="https://voxis.example.com/.well-known/oauth-protected-resource"`)
}

func TestMCPWWWAuthenticate_EmptyURL_IsNoOp(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(MCPWWWAuthenticate(""))
	router.Use(func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Header().Get("WWW-Authenticate"))
}

func TestMCPWWWAuthenticate_HeaderPresentOnSuccess(t *testing.T) {
	// Spec mandates the header on 401; it's also safe to expose on 200
	// responses (RFC 9728 doesn't forbid it). Keeping it always-on simplifies
	// the implementation and lets curious clients introspect the auth server
	// without a deliberate failure.
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(MCPWWWAuthenticate("https://voxis.example.com/.well-known/oauth-protected-resource"))
	router.GET("/mcp", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("WWW-Authenticate"))
}
