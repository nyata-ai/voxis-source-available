package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newPrefixLimitedRouter(rl *RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(rl.PathPrefixMiddleware("/api/", "/mcp"))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	router.GET("/health", ok)
	router.GET("/ready", ok)
	router.GET("/api/v1/me", ok)
	router.POST("/mcp", ok)
	return router
}

func servePrefixLimited(router *gin.Engine, method, path, remoteAddr string) int {
	req := httptest.NewRequest(method, path, http.NoBody)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestPathPrefixMiddleware_LimitsAPIAndMCPPerIP(t *testing.T) {
	router := newPrefixLimitedRouter(NewRateLimiter(0.001, 2))

	assert.Equal(t, http.StatusOK, servePrefixLimited(router, http.MethodGet, "/api/v1/me", "203.0.113.7:1000"))
	assert.Equal(t, http.StatusOK, servePrefixLimited(router, http.MethodPost, "/mcp", "203.0.113.7:1001"))
	assert.Equal(t, http.StatusTooManyRequests, servePrefixLimited(router, http.MethodGet, "/api/v1/me", "203.0.113.7:1002"),
		"/api and /mcp share one per-IP bucket")
	assert.Equal(t, http.StatusTooManyRequests, servePrefixLimited(router, http.MethodPost, "/mcp", "203.0.113.7:1003"))
	assert.Equal(t, http.StatusOK, servePrefixLimited(router, http.MethodGet, "/api/v1/me", "198.51.100.9:1000"),
		"another client IP has its own bucket")
}

func TestPathPrefixMiddleware_LeavesHealthChecksAlone(t *testing.T) {
	router := newPrefixLimitedRouter(NewRateLimiter(0.001, 1))

	for range 5 {
		assert.Equal(t, http.StatusOK, servePrefixLimited(router, http.MethodGet, "/health", "203.0.113.7:1000"))
		assert.Equal(t, http.StatusOK, servePrefixLimited(router, http.MethodGet, "/ready", "203.0.113.7:1000"))
	}
	assert.Equal(t, http.StatusOK, servePrefixLimited(router, http.MethodGet, "/api/v1/me", "203.0.113.7:1000"),
		"health checks did not consume the API budget")
}

func TestAllowUser_SharesBucketWithUserMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(0.001, 1)
	router := gin.New()
	router.POST("/transcriptions", func(c *gin.Context) {
		c.Set("subject", "user-1")
		c.Next()
	}, rl.UserMiddleware("transcription_create"), func(c *gin.Context) { c.Status(http.StatusCreated) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/transcriptions", http.NoBody))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.False(t, rl.AllowUser("transcription_create", "user-1"), "REST spent the user's only token")
	assert.True(t, rl.AllowUser("transcription_create", "user-2"))
	assert.True(t, rl.AllowUser("other_prefix", "user-1"))
}
