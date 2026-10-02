package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// ConcurrencyLimiter bounds simultaneous requests per authenticated user.
// Entries are removed when their last request exits, so the map cannot grow
// with the number of users over the lifetime of the process.
type ConcurrencyLimiter struct {
	mu     sync.Mutex
	active map[string]int
	max    int
}

// NewConcurrencyLimiter creates a limiter with the given per-key maximum.
func NewConcurrencyLimiter(maxConcurrent int) *ConcurrencyLimiter {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &ConcurrencyLimiter{
		active: make(map[string]int),
		max:    maxConcurrent,
	}
}

// UserMiddleware limits requests by authenticated subject, falling back to IP
// only when the middleware was accidentally placed before authentication.
func (l *ConcurrencyLimiter) UserMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := userBucketKey(c)
		if !l.acquire(key) {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many concurrent requests",
			})
			return
		}
		defer l.release(key)
		c.Next()
	}
}

func (l *ConcurrencyLimiter) acquire(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[key] >= l.max {
		return false
	}
	l.active[key]++
	return true
}

func (l *ConcurrencyLimiter) release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[key] <= 1 {
		delete(l.active, key)
		return
	}
	l.active[key]--
}
