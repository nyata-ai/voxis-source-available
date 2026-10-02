package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// SpeakerSuggestionProvider asks an LLM to infer speaker NAMES from a
// speaker-indexed transcript. The service layer builds the indexed transcript
// and performs all sanitization; this provider only runs the model call and
// parses the response.
type SpeakerSuggestionProvider interface {
	SuggestSpeakers(ctx context.Context, req SpeakerSuggestionRequest) (*SpeakerSuggestionResult, error)
}

// SpeakerSuggestionRequest contains the input for speaker-name inference.
type SpeakerSuggestionRequest struct {
	// Transcript is a speaker-indexed transcript, e.g. lines like
	// "[Speaker 1] My name is John Smith."
	Transcript string
	// Language is a best-effort language hint (may be empty).
	Language string
}

// SpeakerSuggestionResult contains the inferred speaker names.
type SpeakerSuggestionResult struct {
	// Suggestions keyed by speaker index as a string ("0", "1", ...).
	Suggestions      map[string]domain.SpeakerSuggestion
	PromptTokens     int
	CompletionTokens int
}
