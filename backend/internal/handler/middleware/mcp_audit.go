package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// MCPAudit emits one structured log line per /mcp request after the response
// is written. It captures correlation fields (request_id, client_ip, auth_method,
// org_id, api_key_id), HTTP status, latency, and a coarse outcome label. The
// middleware never reads or logs request or response bodies.
//
// Wire it BEFORE pre-auth limiting so denied requests still get a log line.
func MCPAudit(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		logger.Info("mcp request",
			"request_id", c.GetString("request_id"),
			"client_ip", c.ClientIP(),
			"auth_method", c.GetString("auth_method"),
			"org_id", c.GetString("org_id"),
			"api_key_id", c.GetString("api_key_id"),
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"outcome", mcpOutcomeForStatus(status),
		)
	}
}

// mcpOutcomeForStatus maps an HTTP status to a stable outcome label suitable
// for dashboards and alerts.
func mcpOutcomeForStatus(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "ok"
	case status == http.StatusUnauthorized:
		return "unauthorized"
	case status == http.StatusForbidden:
		return "forbidden"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status == http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case status == http.StatusBadRequest:
		return "bad_request"
	case status >= 500:
		return "internal_error"
	default:
		return "other"
	}
}
