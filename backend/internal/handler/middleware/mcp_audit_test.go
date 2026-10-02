package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/voxis/backend/internal/handler/middleware"
)

func newAuditedRouter(t *testing.T, downstream gin.HandlerFunc) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := gin.New()
	r.Use(middleware.MCPAudit(logger))
	r.POST("/mcp", downstream)
	return r, &buf
}

func TestMCPAudit_LogsUnauthorized(t *testing.T) {
	t.Parallel()
	r, buf := newAuditedRouter(t, func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.RemoteAddr = "203.0.113.1:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	logs := buf.String()
	assert.Contains(t, logs, `"outcome":"unauthorized"`)
	assert.Contains(t, logs, `"status":401`)
	assert.Contains(t, logs, `"client_ip"`)
}

func TestMCPAudit_LogsRateLimited(t *testing.T) {
	t.Parallel()
	r, buf := newAuditedRouter(t, func(c *gin.Context) {
		c.AbortWithStatus(http.StatusTooManyRequests)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Contains(t, buf.String(), `"outcome":"rate_limited"`)
}

func TestMCPAudit_LogsPayloadTooLarge(t *testing.T) {
	t.Parallel()
	r, buf := newAuditedRouter(t, func(c *gin.Context) {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Contains(t, buf.String(), `"outcome":"payload_too_large"`)
}

func TestMCPAudit_LogsForbidden(t *testing.T) {
	t.Parallel()
	r, buf := newAuditedRouter(t, func(c *gin.Context) {
		c.AbortWithStatus(http.StatusForbidden)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Contains(t, buf.String(), `"outcome":"forbidden"`)
}

func TestMCPAudit_LogsOK(t *testing.T) {
	t.Parallel()
	r, buf := newAuditedRouter(t, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Contains(t, buf.String(), `"outcome":"ok"`)
}

func TestMCPAudit_NoBodyLeak(t *testing.T) {
	t.Parallel()
	r, buf := newAuditedRouter(t, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"top_secret_password": "hunter2"})
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"top_secret_input":"sensitive"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	logs := buf.String()
	assert.NotContains(t, logs, "hunter2")
	assert.NotContains(t, logs, "sensitive")
	assert.NotContains(t, logs, "top_secret_password")
	assert.NotContains(t, logs, "top_secret_input")
}

func TestMCPAudit_LogsCorrelationFieldsWhenPresent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := gin.New()
	// Simulate auth + request_id middleware running first.
	r.Use(func(c *gin.Context) {
		c.Set("request_id", "req-abc")
		c.Set("auth_method", "api_key")
		c.Set("org_id", "org-xyz")
		c.Set("api_key_id", "key-1")
		c.Next()
	})
	r.Use(middleware.MCPAudit(logger))
	r.POST("/mcp", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	logs := buf.String()
	assert.Contains(t, logs, `"request_id":"req-abc"`)
	assert.Contains(t, logs, `"auth_method":"api_key"`)
	assert.Contains(t, logs, `"org_id":"org-xyz"`)
	assert.Contains(t, logs, `"api_key_id":"key-1"`)
}

func TestMCPAudit_NilLoggerUsesDefault(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.MCPAudit(nil))
	r.POST("/mcp", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
