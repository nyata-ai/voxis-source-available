package domain

import "encoding/json"

// TranscriptionContent is the canonical JSON structure encrypted and stored
// with a completed transcription. It contains the full transcript text, the
// raw utterances array from the provider, the human-confirmed speaker map, and
// optional AI-inferred speaker suggestions.
//
// This is the single source of truth for the encrypted content envelope. Both
// the service and worker layers (un)marshal through this type so a future
// decrypt → modify → re-encrypt write path cannot silently drop fields.
//
// The omitempty tags preserve backward-compatible serialization of existing
// encrypted blobs: old blobs lack the new keys, and new fields stay absent
// when empty.
type TranscriptionContent struct {
	FullTranscript       string                       `json:"full_transcript"`
	Utterances           json.RawMessage              `json:"utterances"`
	SpeakerMap           map[string]string            `json:"speaker_map,omitempty"`
	SuggestedSpeakerMap  map[string]SpeakerSuggestion `json:"suggested_speaker_map,omitempty"`
	SuggestionsGenerated bool                         `json:"suggestions_generated,omitempty"`
	// SuggestionAttempts bounds retries of non-definitive generation failures
	// (a non-block provider error that consistently fails, e.g. unparseable
	// output). omitempty keeps legacy blobs backward-compatible: absent → 0.
	SuggestionAttempts int `json:"suggestion_attempts,omitempty"`
}

// SpeakerSuggestion is an AI-inferred speaker name kept separate from the
// human-confirmed SpeakerMap so suggested vs. confirmed provenance is explicit.
type SpeakerSuggestion struct {
	Name       string `json:"name"`
	Evidence   string `json:"evidence"`
	Confidence string `json:"confidence"` // high | medium | low
}

// Confidence levels for SpeakerSuggestion.Confidence. Any other value is
// normalized to ConfidenceLow by the service sanitizer.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)
