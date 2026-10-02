package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// minIdleTTL is the shortest time an unused bucket is kept in memory.
const minIdleTTL = 3 * time.Minute

// maxLimiterEntries bounds the limiters map by size. idleTTL already bounds it
// by time, but a burst of distinct keys (e.g. spoofed IPs) between cleanup
// ticks could otherwise grow it without limit.
const maxLimiterEntries = 100_000

// RateLimiter provides per-IP token bucket rate limiting.
type RateLimiter struct {
	mu         sync.Mutex
	limiters   map[string]*ipLimiter
	rate       rate.Limit
	burst      int
	idleTTL    time.Duration
	maxEntries int
}

// NewRateLimiter creates a rate limiter with the given requests-per-second
// rate and burst size. The rps parameter is a float64 to keep callers free
// from importing golang.org/x/time/rate.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return newRateLimiterWithCap(rps, burst, maxLimiterEntries)
}

// newRateLimiterWithCap is NewRateLimiter with an overridable entry cap, so
// tests can exercise eviction without allocating maxLimiterEntries buckets.
func newRateLimiterWithCap(rps float64, burst, maxEntries int) *RateLimiter {
	rl := &RateLimiter{
		limiters:   make(map[string]*ipLimiter),
		rate:       rate.Limit(rps),
		burst:      burst,
		idleTTL:    idleTTLFor(rps, burst),
		maxEntries: maxEntries,
	}
	go rl.cleanup()
	return rl
}

// idleTTLFor returns how long an unused bucket must be kept. A bucket has to
// outlive its own refill time: evicting it sooner hands the next caller a full
// burst, which would silently void a long-window quota such as 5 per hour.
func idleTTLFor(rps float64, burst int) time.Duration {
	if rps <= 0 || burst <= 0 {
		return minIdleTTL
	}
	refill := time.Duration(float64(burst) / rps * float64(time.Second))
	if refill < minIdleTTL {
		return minIdleTTL
	}
	return refill + time.Minute
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	l, exists := rl.limiters[ip]
	if !exists {
		if len(rl.limiters) >= rl.maxEntries {
			rl.evictOldestLocked()
		}
		l = &ipLimiter{limiter: rate.NewLimiter(rl.rate, rl.burst)}
		rl.limiters[ip] = l
	}
	l.lastSeen = time.Now()
	return l.limiter
}

// evictOldestLocked drops the single least-recently-seen entry. It only runs
// at the size cap, so the linear scan is not on the hot path. Caller must hold rl.mu.
func (rl *RateLimiter) evictOldestLocked() {
	var oldestKey string
	var oldestSeen time.Time
	found := false
	for k, l := range rl.limiters {
		if !found || l.lastSeen.Before(oldestSeen) {
			oldestKey, oldestSeen, found = k, l.lastSeen, true
		}
	}
	if found {
		delete(rl.limiters, oldestKey)
	}
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, l := range rl.limiters {
			if time.Since(l.lastSeen) > rl.idleTTL {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow reports whether the given key can proceed under the rate limit.
func (rl *RateLimiter) Allow(key string) bool {
	return rl.getLimiter(key).Allow()
}

// AllowN reports whether n events may proceed now for the given key. Used when
// one request fans out to several billable operations (e.g. generate-all
// summaries enqueues four jobs), so the cost charged matches the work done.
// Note: n must not exceed the configured burst, or it always returns false.
func (rl *RateLimiter) AllowN(key string, n int) bool {
	return rl.getLimiter(key).AllowN(time.Now(), n)
}

// Middleware returns a gin middleware that enforces rate limits keyed by client IP.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests",
			})
			return
		}
		c.Next()
	}
}

// MCPMiddleware returns a gin middleware for MCP routes. The bucket key uses
// the most specific identifier available: api_key_id (per key), then subject
// (per user, JWT path), then ClientIP (unauthenticated fallback).
func (rl *RateLimiter) MCPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := mcpBucketKey(c)
		if !rl.Allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests",
			})
			return
		}
		c.Next()
	}
}

// TranscriptionCreateLimitKey namespaces the per-user bucket shared by every
// route and MCP tool that starts a billable transcription.
const TranscriptionCreateLimitKey = "transcription_create"

// AllowUser meters one event for an authenticated user under prefix. It
// shares buckets with UserMiddleware(prefix), so a REST route and an MCP tool
// that start the same billable work draw on one quota.
func (rl *RateLimiter) AllowUser(prefix, subject string) bool {
	return rl.Allow(prefix + ":user:" + subject)
}

// PathPrefixMiddleware enforces the per-client-IP limit only on requests
// whose path starts with one of prefixes; every other route (health checks,
// static metadata) passes untouched. Mount it before authentication so it
// also bounds requests carrying forged tokens.
func (rl *RateLimiter) PathPrefixMiddleware(prefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, prefix := range prefixes {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			if !rl.Allow("ip:" + c.ClientIP()) {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error":   "rate_limited",
					"message": "too many requests",
				})
				return
			}
			break
		}
		c.Next()
	}
}

// UserMiddleware returns a gin middleware that meters a route per
// authenticated user. The prefix namespaces the bucket so two routes sharing
// one limiter instance never share a quota.
//
// Unauthenticated requests fall back to the client IP: these routes always sit
// behind auth, so that path is only a safety net, never the normal case.
func (rl *RateLimiter) UserMiddleware(prefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.Allow(prefix + ":" + userBucketKey(c)) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests",
			})
			return
		}
		c.Next()
	}
}

// userBucketKey returns the most specific stable identifier for a user request.
func userBucketKey(c *gin.Context) string {
	if claims := GetClaims(c); claims != nil && claims.Subject != "" {
		return "user:" + claims.Subject
	}
	if v, ok := c.Get("subject"); ok {
		if s, ok := v.(string); ok && s != "" {
			return "user:" + s
		}
	}
	return "ip:" + c.ClientIP()
}

// MCPOrgMiddleware returns a gin middleware that enforces a per-organization
// rate limit. Falls through (no-op) when no org_id is set on the context, so
// callers can compose it after auth without crashing on unauthenticated paths.
func (rl *RateLimiter) MCPOrgMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, ok := c.Get("org_id")
		if !ok {
			c.Next()
			return
		}
		s, ok := orgID.(string)
		if !ok || s == "" {
			c.Next()
			return
		}
		if !rl.Allow("mcp:org:" + s) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "organization rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}

// mcpBucketKey returns the most specific stable identifier for an MCP request.
func mcpBucketKey(c *gin.Context) string {
	if v, ok := c.Get("api_key_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return "mcp:key:" + s
		}
	}
	if v, ok := c.Get("subject"); ok {
		if s, ok := v.(string); ok && s != "" {
			return "mcp:user:" + s
		}
	}
	return "mcp:ip:" + c.ClientIP()
}
