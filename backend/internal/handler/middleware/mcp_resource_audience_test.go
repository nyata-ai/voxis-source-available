package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/voxis/backend/internal/port"
)

func TestMCPRequireResourceAudience_EmptyConfig_IsNoOp(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MCPRequireResourceAudience(""))
	router.GET("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMCPRequireResourceAudience_APIKey_IsBypassed(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_method", "api_key")
		c.Next()
	})
	router.Use(MCPRequireResourceAudience("https://voxis.example.com/mcp"))
	router.GET("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMCPRequireResourceAudience_JWT_AudienceMatches(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(ClaimsContextKey, &port.TokenClaims{
			Subject:  "user-1",
			Audience: []string{"voxis-api", "https://voxis.example.com/mcp"},
		})
		c.Set("auth_method", "jwt")
		c.Next()
	})
	router.Use(MCPRequireResourceAudience("https://voxis.example.com/mcp"))
	router.GET("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMCPRequireResourceAudience_JWT_AudienceMissing_Returns403(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(ClaimsContextKey, &port.TokenClaims{
			Subject:  "user-1",
			Audience: []string{"voxis-api"},
		})
		c.Set("auth_method", "jwt")
		c.Next()
	})
	router.Use(MCPRequireResourceAudience("https://voxis.example.com/mcp"))
	router.GET("/mcp", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "resource")
}
