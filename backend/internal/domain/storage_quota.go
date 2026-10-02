package domain

import "time"

const (
	// DefaultStorageQuotaLimitBytes is 2 GiB.
	DefaultStorageQuotaLimitBytes int64 = 2 * 1024 * 1024 * 1024
	// StorageQuotaWarningThresholdPercent is deliberately global and fixed.
	StorageQuotaWarningThresholdPercent = 85
)

// StorageQuotaPolicy is the global bucket-storage quota policy.
type StorageQuotaPolicy struct {
	Enabled                 bool       `json:"enabled"`
	DefaultLimitBytes       int64      `json:"default_limit_bytes"`
	WarningThresholdPercent int        `json:"warning_threshold_percent"`
	UpdatedAt               *time.Time `json:"updated_at"`
	UpdatedBy               string     `json:"updated_by"`
}

// IsBucketStorageBackend reports whether the runtime stores objects in a
// bucket-like backend. Keep supported names here as additional adapters land.
func IsBucketStorageBackend(backend string) bool {
	switch backend {
	case "gcs", "s3", "azure", "minio":
		return true
	default:
		return false
	}
}
