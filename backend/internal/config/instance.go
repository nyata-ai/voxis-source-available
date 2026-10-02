package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"sync"
)

var (
	instanceID     string
	instanceIDOnce sync.Once
)

// GetInstanceID returns the unique identifier for this running instance.
// It initializes the ID exactly once using sync.Once for thread safety.
func GetInstanceID() string {
	instanceIDOnce.Do(func() {
		instanceID = os.Getenv("INSTANCE_ID")
		if instanceID == "" {
			instanceID = generateShortID()
		}
		// Also update the deprecated global for backwards compatibility
		InstanceID = instanceID
	})
	return instanceID
}

// InstanceID uniquely identifies this running instance.
// Deprecated: Use GetInstanceID() for thread-safe access.
// Kept for backwards compatibility with ldflags pattern.
var InstanceID string

func init() {
	InstanceID = GetInstanceID()
}

// generateShortID creates a random 8-character hex string
func generateShortID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		slog.Error("failed to generate instance ID, using fallback", "error", err)
		return "unknown"
	}
	return hex.EncodeToString(bytes)
}
