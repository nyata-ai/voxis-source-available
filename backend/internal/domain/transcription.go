package domain

import (
	"fmt"
	"strings"
	"time"
)

// Transcription status, provider, and expected-speaker bounds used by OSS jobs.
const (
	TranscriptionStatusPending   = "pending"
	TranscriptionStatusSubmitted = "submitted"
	TranscriptionStatusCompleted = "completed"
	TranscriptionStatusFailed    = "failed"
	TranscriptionStatusDeleted   = "deleted"

	TranscriptionProviderSpeechmatics = "speechmatics"
	MaxExpectedSpeakers               = 10
)

// AllowedLanguages is the bounded language set accepted for Speechmatics jobs.
var AllowedLanguages = map[string]bool{
	"auto": true,
	"id":   true, "jv": true, "su": true, "en": true, "ms": true, "th": true, "vi": true,
	"zh": true, "ja": true, "ko": true,
	"es": true, "de": true, "ru": true, "fr": true,
	"ar": true, "af": true, "he": true,
	"tl": true, "pl": true, "it": true,
	"sv": true, "da": true, "no": true, "fi": true, "pt": true, "la": true,
}

// Transcription is an encrypted transcript derived from one uploaded or recorded media item.
type Transcription struct {
	ID               string
	OrganizationID   string
	MediaID          string
	Status           string
	Languages        []string
	Diarization      bool
	EnhanceAudio     bool
	PreprocessorUsed string
	VocabularyPacks  []string
	ExpectedSpeakers int
	ContentEncrypted []byte
	ContentNonce     []byte
	WrappedDEK       []byte
	WrappingNonce    []byte
	SpeakerCount     int
	WordCount        int
	DurationSeconds  float64
	ErrorMessage     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time

	Provider              string
	SpeechmaticsJobID     string
	SpeechmaticsDeletedAt *time.Time
	FullTranscript        string                       `json:"-"`
	Utterances            []byte                       `json:"-"`
	MediaFilename         string                       `json:"-"`
	MediaTitle            string                       `json:"-"`
	MediaDescription      string                       `json:"-"`
	MediaStatus           string                       `json:"-"`
	SpeakerMap            map[string]string            `json:"-"`
	SuggestedSpeakerMap   map[string]SpeakerSuggestion `json:"-"`
	SuggestionsGenerated  bool                         `json:"-"`
}

func validateLanguages(languages []string) ([]string, error) {
	if len(languages) == 0 || len(languages) > 5 {
		return nil, fmt.Errorf("between 1 and 5 languages are required: %w", ErrInvalidInput)
	}
	seen := make(map[string]bool, len(languages))
	result := make([]string, len(languages))
	for i, language := range languages {
		normalized := strings.ToLower(strings.TrimSpace(language))
		if !AllowedLanguages[normalized] || seen[normalized] {
			return nil, fmt.Errorf("unsupported or duplicate language %q: %w", language, ErrInvalidInput)
		}
		seen[normalized] = true
		result[i] = normalized
	}
	if seen["auto"] && len(result) > 1 {
		return nil, fmt.Errorf("auto cannot be combined with specific languages: %w", ErrInvalidInput)
	}
	return result, nil
}

func normalizeVocabularyPacks(packs []string) []string {
	seen := make(map[string]bool, len(packs))
	result := make([]string, 0, len(packs))
	for _, pack := range packs {
		pack = strings.TrimSpace(pack)
		if pack != "" && !seen[pack] {
			seen[pack] = true
			result = append(result, pack)
		}
	}
	return result
}

// ValidateExpectedSpeakers enforces the bounded expected-speaker request value.
func ValidateExpectedSpeakers(n int) error {
	if n < 0 || n > MaxExpectedSpeakers {
		return fmt.Errorf("expected speakers must be between 0 and %d: %w", MaxExpectedSpeakers, ErrInvalidInput)
	}
	return nil
}

// SetExpectedSpeakers validates and stores the requested speaker count.
func (t *Transcription) SetExpectedSpeakers(n int) error {
	if err := ValidateExpectedSpeakers(n); err != nil {
		return err
	}
	t.ExpectedSpeakers = n
	return nil
}

// NewTranscription creates a pending media-backed OSS transcription.
func NewTranscription(orgID, mediaID string, languages []string, diarization, enhanceAudio bool, vocabularyPacks ...string) (*Transcription, error) {
	if orgID == "" || mediaID == "" {
		return nil, fmt.Errorf("organization and media IDs are required: %w", ErrInvalidInput)
	}
	normalized, err := validateLanguages(languages)
	if err != nil {
		return nil, err
	}
	return &Transcription{
		OrganizationID: orgID, MediaID: mediaID, Languages: normalized,
		Diarization: diarization, EnhanceAudio: enhanceAudio,
		Status: TranscriptionStatusPending, VocabularyPacks: normalizeVocabularyPacks(vocabularyPacks),
	}, nil
}

// SetSubmittedSpeechmatics records the submitted provider job identifier.
func (t *Transcription) SetSubmittedSpeechmatics(jobID string) {
	t.Provider = TranscriptionProviderSpeechmatics
	t.SpeechmaticsJobID = jobID
	t.Status = TranscriptionStatusSubmitted
	t.UpdatedAt = time.Now()
}

// SetSpeechmaticsDeleted records confirmed provider-side deletion.
func (t *Transcription) SetSpeechmaticsDeleted() {
	now := time.Now()
	t.SpeechmaticsDeletedAt = &now
	t.UpdatedAt = now
}

// ProviderName returns the sole transcription provider supported by OSS.
func (t *Transcription) ProviderName() string { return TranscriptionProviderSpeechmatics }

// SetCompleted stores provider result counts and marks the transcription complete.
func (t *Transcription) SetCompleted(speakerCount, wordCount int, durationSec float64) {
	t.SpeakerCount = speakerCount
	t.WordCount = wordCount
	t.DurationSeconds = durationSec
	t.Status = TranscriptionStatusCompleted
	now := time.Now()
	t.CompletedAt = &now
	t.UpdatedAt = now
}

// SetFailed stores the sanitized provider failure and marks the job failed.
func (t *Transcription) SetFailed(message string) {
	t.ErrorMessage = message
	t.Status = TranscriptionStatusFailed
	t.UpdatedAt = time.Now()
}

// Clone returns an independent copy of mutable transcription fields.
func (t *Transcription) Clone() *Transcription {
	cloned := *t
	cloned.Languages = append([]string(nil), t.Languages...)
	cloned.VocabularyPacks = append([]string(nil), t.VocabularyPacks...)
	cloned.ContentEncrypted = cloneBytes(t.ContentEncrypted)
	cloned.ContentNonce = cloneBytes(t.ContentNonce)
	cloned.WrappedDEK = cloneBytes(t.WrappedDEK)
	cloned.WrappingNonce = cloneBytes(t.WrappingNonce)
	cloned.Utterances = cloneBytes(t.Utterances)
	cloned.CompletedAt = cloneTimePtr(t.CompletedAt)
	cloned.SpeechmaticsDeletedAt = cloneTimePtr(t.SpeechmaticsDeletedAt)
	if t.SpeakerMap != nil {
		cloned.SpeakerMap = make(map[string]string, len(t.SpeakerMap))
		for key, value := range t.SpeakerMap {
			cloned.SpeakerMap[key] = value
		}
	}
	if t.SuggestedSpeakerMap != nil {
		cloned.SuggestedSpeakerMap = make(map[string]SpeakerSuggestion, len(t.SuggestedSpeakerMap))
		for key, value := range t.SuggestedSpeakerMap {
			cloned.SuggestedSpeakerMap[key] = value
		}
	}
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
