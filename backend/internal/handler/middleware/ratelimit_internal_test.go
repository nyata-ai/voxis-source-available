package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIdleTTLFor(t *testing.T) {
	tests := []struct {
		name  string
		rps   float64
		burst int
		want  time.Duration
	}{
		{"fast limiter keeps the floor", 10, 20, minIdleTTL},
		{"per-minute limiter keeps the floor", 5.0 / 60.0, 5, minIdleTTL},
		{"five per hour outlives its refill", 5.0 / 3600.0, 5, time.Hour + time.Minute},
		{"three per hour outlives its refill", 3.0 / 3600.0, 3, time.Hour + time.Minute},
		{"zero rate falls back to the floor", 0, 5, minIdleTTL},
		{"zero burst falls back to the floor", 1, 0, minIdleTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, idleTTLFor(tt.rps, tt.burst), float64(time.Second))
		})
	}
}

// TestRateLimiter_EvictsOldestAtCap asserts the map size never exceeds the
// configured cap and that the most recently touched keys survive eviction.
func TestRateLimiter_EvictsOldestAtCap(t *testing.T) {
	rl := newRateLimiterWithCap(1, 5, 3)

	keys := []string{"a", "b", "c", "d", "e"}
	for _, k := range keys {
		rl.getLimiter(k)
		assert.LessOrEqual(t, len(rl.limiters), 3, "limiters map must never exceed the cap")
	}

	assert.Len(t, rl.limiters, 3)
	for _, k := range []string{"c", "d", "e"} {
		_, ok := rl.limiters[k]
		assert.True(t, ok, "most recently touched key %q should survive eviction", k)
	}
	for _, k := range []string{"a", "b"} {
		_, ok := rl.limiters[k]
		assert.False(t, ok, "oldest key %q should have been evicted", k)
	}
}
