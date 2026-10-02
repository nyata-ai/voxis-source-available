package port

import (
	"context"
	"time"
)

// SummaryPromptVersion identifies the code-owned summary safety envelope.
// It is not stored with admin presentation or task overrides.
const SummaryPromptVersion = "summary-invariant-v1"

// StructuredSummaryPromptVersion identifies the invariant, source-v1, and
// schema-constrained behavior used by structured summary generation.
const StructuredSummaryPromptVersion = "summary-invariant-source-schema-v1"

// LLMPromptConfig is the effective prompt/model configuration exposed to admins.
type LLMPromptConfig struct {
	Key               string     `json:"key"`
	Label             string     `json:"label"`
	SummaryType       string     `json:"summary_type"`
	PromptVersion     string     `json:"prompt_version"`
	SystemInstruction string     `json:"system_instruction"`
	UserPrompt        string     `json:"user_prompt"`
	Model             string     `json:"model"`
	HasOverride       bool       `json:"has_override"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
}

// LLMPromptOverride is a persisted admin override for one prompt key.
type LLMPromptOverride struct {
	Key               string
	SystemInstruction string
	UserPrompt        string
	Model             string
	UpdatedAt         time.Time
	UpdatedBy         string
}

// LLMPromptRepository stores global LLM prompt/model overrides.
type LLMPromptRepository interface {
	ListOverrides(ctx context.Context) ([]LLMPromptOverride, error)
	GetOverride(ctx context.Context, key string) (*LLMPromptOverride, error)
	UpsertOverride(ctx context.Context, override LLMPromptOverride) (*LLMPromptOverride, error)
	DeleteOverride(ctx context.Context, key string) error
}
