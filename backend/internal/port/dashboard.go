package port

import (
	"context"
	"time"
)

// DashboardStats holds aggregated statistics for the dashboard.
type DashboardStats struct {
	TotalMedia              int64   `json:"total_media"`
	ProcessingCount         int64   `json:"processing_count"`
	EncryptingCount         int64   `json:"encrypting_count"`
	TranscribingCount       int64   `json:"transcribing_count"`
	CompletedTranscriptions int64   `json:"completed_transcriptions"`
	CompletedThisMonth      int64   `json:"completed_this_month"`
	CompletedLastMonth      int64   `json:"completed_last_month"`
	SecondsThisMonth        float64 `json:"seconds_this_month"`
	// LastCompletedAt is the latest completed_at timestamp; nil serializes to
	// JSON null, meaning no transcription has completed for this organization.
	LastCompletedAt *time.Time `json:"last_completed_at"`
}

// DashboardRepository provides aggregated dashboard statistics.
type DashboardRepository interface {
	GetStats(ctx context.Context, orgID string) (*DashboardStats, error)
}
