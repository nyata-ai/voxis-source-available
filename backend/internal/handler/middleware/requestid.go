package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader is the HTTP header used for request tracing.
const RequestIDHeader = "X-Request-ID"

const (
	maxRequestIDLength = 64
)

// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for existing request ID from client/load balancer
		requestID := c.GetHeader(RequestIDHeader)

		// Validate the incoming request ID
		if !isValidRequestID(requestID) {
			requestID = generateRequestID()
		}

		// Store in context and response header
		c.Set("request_id", requestID)
		c.Header(RequestIDHeader, requestID)

		c.Next()
	}
}

// isValidRequestID checks if a request ID is safe to use
// Valid IDs: non-empty, max 64 chars, alphanumeric + dash + underscore only
func isValidRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, r := range id {
		if !isValidRequestIDChar(r) {
			return false
		}
	}
	return true
}

// isValidRequestIDChar returns true for alphanumeric, dash, and underscore
func isValidRequestIDChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}

func generateRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		slog.Error("failed to generate request ID", "error", err)
		return "unknown"
	}
	return hex.EncodeToString(bytes)
}
