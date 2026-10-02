package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Logging creates a middleware that logs HTTP requests
func Logging(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := redactedLogPath(c.Request.URL.Path)

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)
		status := c.Writer.Status()

		// Build log attributes
		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency", latency.String(),
			"client_ip", c.ClientIP(),
			"body_size", c.Writer.Size(),
		}

		// Add request ID if present
		if requestID := c.GetString("request_id"); requestID != "" {
			attrs = append(attrs, "request_id", requestID)
		}

		// Add user ID if authenticated
		if userID := c.GetString("user_id"); userID != "" {
			attrs = append(attrs, "user_id", userID)
		}

		// Log at appropriate level
		msg := "request completed"
		switch {
		case status >= 500:
			logger.Error(msg, attrs...)
		case status >= 400:
			logger.Warn(msg, attrs...)
		default:
			logger.Info(msg, attrs...)
		}
	}
}

func redactedLogPath(path string) string {
	const gladiaWebhookPrefix = "/api/v1/webhooks/gladia/"
	if strings.HasPrefix(path, gladiaWebhookPrefix) {
		return gladiaWebhookPrefix + "[redacted]"
	}
	return path
}
