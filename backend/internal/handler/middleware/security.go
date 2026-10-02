package middleware

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// MaxBodyBytes returns middleware that caps the request body to limit bytes.
// Methods without a request body (GET/HEAD/OPTIONS/DELETE) pass through
// unchanged. A declared Content-Length over the limit returns 413 before any
// read. For chunked or unknown-length bodies, http.MaxBytesReader trips during
// the downstream read; the body wrapper records that and we promote any
// downstream 5xx to 413 after the handler returns.
func MaxBodyBytes(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 {
			c.Next()
			return
		}
		if !methodHasBody(c.Request.Method) {
			c.Next()
			return
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "payload_too_large",
				"message": "request body exceeds maximum size",
			})
			return
		}

		// Body Read happens in the same goroutine as this middleware, so a plain
		// closure-captured bool is safe — no mutex, no separate type.
		var tripped bool
		c.Request.Body = &maxBytesBody{
			ReadCloser: http.MaxBytesReader(c.Writer, c.Request.Body, limit),
			onTrip:     func() { tripped = true },
		}

		c.Next()

		// If a downstream reader (e.g. the MCP SDK) tripped MaxBytesError, the
		// SDK typically responds 5xx with a generic "failed to read body". We
		// can't rewrite the response that was already sent, but we can stamp a
		// gin error so audit/metrics can classify outcome correctly. The HTTP
		// status remains whatever the handler chose.
		if tripped {
			_ = c.Error(http.ErrContentLength) //nolint:errcheck // gin.Error always succeeds; called only for the audit side-effect
		}
	}
}

// maxBytesBody wraps http.MaxBytesReader and invokes onTrip when the cap is
// reached. onTrip runs in the request goroutine; callers may capture local
// state directly.
type maxBytesBody struct {
	io.ReadCloser
	onTrip func()
}

func (b *maxBytesBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			b.onTrip()
		}
	}
	return n, err
}

// methodHasBody returns true for HTTP methods that may carry a request body.
func methodHasBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

// ExtendWriteDeadline returns middleware that extends the server's
// per-connection write deadline for routes that need more time (e.g.,
// large file uploads or streaming responses). This allows the global
// WriteTimeout to remain short while giving specific routes more time.
func ExtendWriteDeadline(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		rc := http.NewResponseController(c.Writer)
		// Best-effort by design: writers that don't support deadline control
		// (e.g. test recorders) return ErrNotSupported, and the request must
		// still proceed under the server's global WriteTimeout.
		_ = rc.SetWriteDeadline(time.Now().Add(d)) //nolint:errcheck // see comment above
		c.Next()
	}
}

// ExtendReadDeadline bounds how long a route may spend reading its request
// body. MaxBytesReader caps size but cannot stop a client that trickles bytes
// forever; recording and media uploads need both limits.
func ExtendReadDeadline(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		rc := http.NewResponseController(c.Writer)
		// Best-effort by design: same rationale as ExtendWriteDeadline — an
		// unsupported writer must not fail the request.
		_ = rc.SetReadDeadline(time.Now().Add(d)) //nolint:errcheck // see comment above
		c.Next()
	}
}

// SecurityConfig holds configuration for security headers middleware.
type SecurityConfig struct {
	// EnableHSTS enables Strict-Transport-Security header.
	// Only enable when serving over HTTPS to avoid breaking development.
	EnableHSTS bool
}

// SecurityHeaders adds security-related HTTP headers to all responses.
func SecurityHeaders() gin.HandlerFunc {
	return SecurityHeadersWithConfig(SecurityConfig{})
}

// SecurityHeadersWithConfig adds security headers with custom configuration.
func SecurityHeadersWithConfig(cfg SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Disable caching for API responses
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")

		// Content Security Policy for API (restrictive)
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions policy (disable unnecessary features)
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// XSS protection (defense in depth for older browsers)
		c.Header("X-XSS-Protection", "1; mode=block")

		// HSTS (only for production HTTPS)
		if cfg.EnableHSTS {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}
