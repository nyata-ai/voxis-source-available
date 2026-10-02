package port

import (
	"context"
	"time"
)

// AdminOpsEntityStats contains aggregate counts for one operational entity.
type AdminOpsEntityStats struct {
	Total    int64            `json:"total"`
	ByStatus map[string]int64 `json:"by_status"`
}

// AdminOpsTranscriptionStats contains aggregate parent transcription counts.
type AdminOpsTranscriptionStats struct {
	Total                  int64            `json:"total"`
	ByStatus               map[string]int64 `json:"by_status"`
	StaleSubmittedOver2Hrs int64            `json:"stale_submitted_over_2h"`
}

// AdminOpsSummaryStats contains aggregate summary pipeline counts.
type AdminOpsSummaryStats struct {
	Total                 int64            `json:"total"`
	ByStatus              map[string]int64 `json:"by_status"`
	StalePendingOver30Min int64            `json:"stale_pending_over_30m"`
}

// AdminSTTUsageStats contains aggregate processing counters for one
// speech-to-text provider. Both providers report the same shape.
type AdminSTTUsageStats struct {
	SubmittedJobs          int64   `json:"submitted_jobs"`
	CompletedJobs          int64   `json:"completed_jobs"`
	FailedJobs             int64   `json:"failed_jobs"`
	StaleSubmittedOver2Hrs int64   `json:"stale_submitted_over_2h"`
	ProcessedAudioSeconds  float64 `json:"processed_audio_seconds"`
}

// AdminGemmaUsageStats contains aggregate Gemma token counters. Backend
// adapters may round component counters for privacy; TotalTokens must match
// the values returned in the component counters.
type AdminGemmaUsageStats struct {
	CompletedSummaries int64 `json:"completed_summaries"`
	UsageAvailable     bool  `json:"usage_available"`
	PromptTokens       int64 `json:"prompt_tokens"`
	CompletionTokens   int64 `json:"completion_tokens"`
	ThinkingTokens     int64 `json:"thinking_tokens"`
	TotalTokens        int64 `json:"total_tokens"`
}

// AdminSummaryModelMetadata identifies the configured local model build. Its
// usage flag describes aggregate persisted measurements, not a model claim.
type AdminSummaryModelMetadata struct {
	Model          string `json:"model"`
	Runtime        string `json:"runtime"`
	Revision       string `json:"revision"`
	Quantization   string `json:"quantization"`
	UsageAvailable bool   `json:"usage_available"`
}

// AdminProviderUsageStats contains aggregate provider usage counters. The
// Speechmatics block is always present and reads zero on deployments that have
// never submitted to it.
type AdminProviderUsageStats struct {
	Speechmatics AdminSTTUsageStats   `json:"speechmatics"`
	Gemma        AdminGemmaUsageStats `json:"gemma"`
}

// AdminRetentionStats contains aggregate retention sweep counters.
type AdminRetentionStats struct {
	EnabledOrganizations int64  `json:"enabled_organizations"`
	DueNow               int64  `json:"due_now"`
	OldestDueAgeSeconds  *int64 `json:"oldest_due_age_seconds,omitempty"`
	AudioDeleted         int64  `json:"audio_deleted"`
}

// AdminOpsStats contains zero-knowledge operational statistics for admins.
type AdminOpsStats struct {
	GeneratedAt    time.Time                  `json:"generated_at"`
	Media          AdminOpsEntityStats        `json:"media"`
	Scan           AdminOpsEntityStats        `json:"scan"`
	Transcriptions AdminOpsTranscriptionStats `json:"transcriptions"`
	Summaries      AdminOpsSummaryStats       `json:"summaries"`
	Recordings     AdminOpsEntityStats        `json:"recordings"`
	Providers      AdminProviderUsageStats    `json:"providers"`
	SummaryModel   *AdminSummaryModelMetadata `json:"summary_model,omitempty"`
	Retention      AdminRetentionStats        `json:"retention"`
}

// AdminRepository provides aggregate admin operations statistics.
type AdminRepository interface {
	GetOpsStats(ctx context.Context, now time.Time) (*AdminOpsStats, error)
}
