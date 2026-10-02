package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery returns a middleware that recovers from panics
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get request ID for traceability
				requestID := c.GetString("request_id")

				// Log the panic with stack trace and request context
				logger.Error("panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"request_id", requestID,
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
		}()

		c.Next()
	}
}
