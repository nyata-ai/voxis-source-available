package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// UsageStats holds aggregated usage statistics for an organization.
type UsageStats struct {
	TotalDurationSeconds            float64          `json:"total_duration_seconds"`
	TotalPromptTokens               int64            `json:"total_prompt_tokens"`
	TotalCompletionTokens           int64            `json:"total_completion_tokens"`
	TotalThinkingTokens             int64            `json:"total_thinking_tokens"`
	URLTranscriptionsRemainingToday int64            `json:"url_transcriptions_remaining_today"`
	LiveRecordingRetentionEnabled   bool             `json:"live_recording_retention_enabled"`
	LiveRecordingRetentionDays      int16            `json:"live_recording_retention_days"`
	StorageUsedBytes                int64            `json:"storage_used_bytes"`
	UserStorage                     UserStorageUsage `json:"user_storage"`
}

// UserStorageUsage is the authenticated user's logical bucket-storage view.
type UserStorageUsage struct {
	UsedBytes               int64  `json:"used_bytes"`
	LimitBytes              int64  `json:"limit_bytes"`
	RemainingBytes          int64  `json:"remaining_bytes"`
	UsagePercent            int    `json:"usage_percent"`
	WarningThresholdPercent int    `json:"warning_threshold_percent"`
	State                   string `json:"state"`
	Enforced                bool   `json:"enforced"`
}

// URLTranscriptionCounts holds rolling 24-hour URL transcription counts.
type URLTranscriptionCounts struct {
	User int64
	Org  int64
}

// UsageRepository provides aggregated usage statistics.
type UsageRepository interface {
	GetStats(ctx context.Context, orgID string) (*UsageStats, error)
	GetURLTranscriptionCounts(ctx context.Context, orgID, userID string) (URLTranscriptionCounts, error)
}

// UserStorageUsageRepository is kept narrow so existing organization-level
// callers, including MCP, retain their stable aggregate contract.
type UserStorageUsageRepository interface {
	GetStorageQuotaPolicy(ctx context.Context) (domain.StorageQuotaPolicy, error)
	GetUserStorageUsedBytes(ctx context.Context, orgID, userID string) (int64, error)
}
