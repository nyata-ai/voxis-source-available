package domain

import (
	"fmt"
	"time"
)

// Summary status constants.
const (
	SummaryStatusPending   = "pending"
	SummaryStatusCompleted = "completed"
	SummaryStatusFailed    = "failed"
	SummaryStatusDeleted   = "deleted"
)

// Summary review status constants.
const (
	SummaryReviewStatusSkipped       = "skipped"
	SummaryReviewStatusPending       = "pending"
	SummaryReviewStatusRetrying      = "retrying"
	SummaryReviewStatusPassed        = "passed"
	SummaryReviewStatusFlagged       = "flagged"
	SummaryReviewStatusFailed        = "failed"
	SummaryReviewStatusParseFailed   = "parse_failed"
	SummaryReviewStatusSafetyBlocked = "safety_blocked"
)

// Summary type constants.
const (
	SummaryTypeGeneral     = "general"
	SummaryTypeKeyPoints   = "key_points"
	SummaryTypeActionItems = "action_items"
	SummaryTypeQAndA       = "q_and_a"

	// SummaryProfileGeneralProfessional is the default profile for new summaries.
	SummaryProfileGeneralProfessional = "general_professional"
	SummaryProfileLegal               = "legal"
	SummaryProfileInvestmentAnalysis  = "investment_analysis"
	SummaryProfileJournalism          = "journalism"
	SummaryProfileNegotiation         = "negotiation"
	SummaryProfileDecisionCommittee   = "decision_committee"
	SummaryProfileInvestigation       = "investigation"
)

// Summary degradation codes describe a bounded, content-free reduction in a
// completed summary. They are persisted for release analysis, never model text.
const (
	SummaryDegradationStructuredValidationFallback = "structured_validation_fallback"
	SummaryDegradationStructuredChunkFallback      = "structured_chunk_fallback"
	SummaryDegradationSourceAttributionFallback    = "source_attribution_fallback"
	SummaryDegradationUnansweredAnswerNormalized   = "unanswered_answer_normalized"
	SummaryDegradationExactQuoteDropped            = "exact_quote_dropped"
	SummaryDegradationItemLimitApplied             = "item_limit_applied"
	SummaryDegradationStructuredSourceFallback     = "structured_source_fallback"
)

const maxSummaryDegradationCodes = 8

var allowedSummaryDegradationCodes = map[string]bool{
	SummaryDegradationStructuredValidationFallback: true,
	SummaryDegradationStructuredChunkFallback:      true,
	SummaryDegradationSourceAttributionFallback:    true,
	SummaryDegradationUnansweredAnswerNormalized:   true,
	SummaryDegradationExactQuoteDropped:            true,
	SummaryDegradationItemLimitApplied:             true,
	SummaryDegradationStructuredSourceFallback:     true,
}

// NormalizeSummaryDegradationCodes validates and deduplicates persisted codes.
func NormalizeSummaryDegradationCodes(codes []string) ([]string, error) {
	capacity := min(len(codes), maxSummaryDegradationCodes)
	result := make([]string, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	for _, code := range codes {
		if !allowedSummaryDegradationCodes[code] {
			return nil, fmt.Errorf("unsupported degradation code %q: %w", code, ErrInvalidInput)
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
		if len(result) > maxSummaryDegradationCodes {
			return nil, fmt.Errorf("too many degradation codes: %w", ErrInvalidInput)
		}
	}
	return result, nil
}

// SummaryProfiles is the canonical ordered set of professional summary
// profiles, in UI display order. Validation maps and the settings handler's
// allowed-values message are derived from it, so a new profile is added here
// (and in the frontend SUMMARY_PROFILES list) exactly once.
var SummaryProfiles = []string{
	SummaryProfileGeneralProfessional,
	SummaryProfileLegal,
	SummaryProfileInvestmentAnalysis,
	SummaryProfileJournalism,
	SummaryProfileNegotiation,
	SummaryProfileDecisionCommittee,
	SummaryProfileInvestigation,
}

// allowedSummaryProfiles for validation, derived from SummaryProfiles.
var allowedSummaryProfiles = func() map[string]bool {
	m := make(map[string]bool, len(SummaryProfiles))
	for _, p := range SummaryProfiles {
		m[p] = true
	}
	return m
}()

// IsSummaryProfile reports whether profile is one of the supported
// professional profiles.
func IsSummaryProfile(profile string) bool {
	return allowedSummaryProfiles[profile]
}

// NormalizeSummaryProfile preserves supported values and falls back to the
// safe General Professional default for absent or invalid persisted values.
func NormalizeSummaryProfile(profile string) string {
	if IsSummaryProfile(profile) {
		return profile
	}
	return SummaryProfileGeneralProfessional
}

// DefaultSummaryTypes is the canonical ordered set of summary types produced by
// "generate all" flows. The fan-out token charge and the billable rate-limiter
// burst are both sized from len(DefaultSummaryTypes), so adding a type here keeps
// validation, the generate-all fan-out, and the limiter in lockstep.
var DefaultSummaryTypes = []string{
	SummaryTypeGeneral,
	SummaryTypeKeyPoints,
	SummaryTypeActionItems,
	SummaryTypeQAndA,
}

// allowedSummaryTypes for validation, derived from DefaultSummaryTypes.
var allowedSummaryTypes = func() map[string]bool {
	m := make(map[string]bool, len(DefaultSummaryTypes))
	for _, t := range DefaultSummaryTypes {
		m[t] = true
	}
	return m
}()

// Summary represents an AI-generated summary of a transcription.
type Summary struct {
	ID                             string
	OrganizationID                 string
	TranscriptionID                string
	SummaryType                    string // general | key_points | action_items | q_and_a
	Status                         string
	ContentEncrypted               []byte
	ContentNonce                   []byte
	WrappedDEK                     []byte
	WrappingNonce                  []byte
	WordCount                      int
	ErrorMessage                   string
	PromptTokens                   int
	CompletionTokens               int
	ThinkingTokens                 int
	HighStakes                     bool
	ReviewStatus                   string
	ReviewFindingsEncrypted        []byte
	ReviewFindingsNonce            []byte
	ReviewWrappedDEK               []byte
	ReviewWrappingNonce            []byte
	ExtractionEncrypted            []byte
	ExtractionNonce                []byte
	ExtractionWrappedDEK           []byte
	ExtractionWrappingNonce        []byte
	SummaryProfile                 string
	PromptVersion                  string
	Model                          string
	ModelMetadata                  []byte
	EndpointLocation               string
	SourceVersion                  string
	SourceHash                     string
	StructuredContentCiphertext    []byte
	StructuredContentNonce         []byte
	StructuredContentWrappedDEK    []byte
	StructuredContentWrappingNonce []byte
	StructuredSchemaVersion        string
	DegradationCodes               []string
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
	CompletedAt                    *time.Time

	// Transient fields — populated by the service layer, not persisted in DB.
	// Content is the decrypted summary text (from OpenField).
	Content string `json:"-"`
	// StructuredContent is the decrypted, provider-shaped structured JSON root.
	StructuredContent []byte `json:"-"`
	// TranscriptionMediaFilename is joined from the media table for API responses.
	TranscriptionMediaFilename string `json:"-"`
	// TranscriptionAudioName is media title fallback filename for executive-friendly display.
	TranscriptionAudioName string `json:"-"`
	// TranscriptionMediaDescription is media description joined for list search/display.
	TranscriptionMediaDescription string `json:"-"`
}

// NewSummary creates a new Summary with validated fields.
// ID is left empty (assigned by the database).
func NewSummary(orgID, transcriptionID, summaryType string) (*Summary, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty: %w", ErrInvalidInput)
	}
	if transcriptionID == "" {
		return nil, fmt.Errorf("transcription ID cannot be empty: %w", ErrInvalidInput)
	}
	if !allowedSummaryTypes[summaryType] {
		return nil, fmt.Errorf("unsupported summary type %q: %w", summaryType, ErrInvalidInput)
	}
	return &Summary{
		OrganizationID:  orgID,
		TranscriptionID: transcriptionID,
		SummaryType:     summaryType,
		Status:          SummaryStatusPending,
		ReviewStatus:    SummaryReviewStatusSkipped,
		SummaryProfile:  SummaryProfileGeneralProfessional,
	}, nil
}

// SetCompleted marks the summary as completed with extracted metadata.
func (s *Summary) SetCompleted(wordCount, promptTokens, completionTokens, thinkingTokens int) {
	s.WordCount = wordCount
	s.PromptTokens = promptTokens
	s.CompletionTokens = completionTokens
	s.ThinkingTokens = thinkingTokens
	s.Status = SummaryStatusCompleted
	now := time.Now()
	s.CompletedAt = &now
	s.UpdatedAt = now
}

// SetFailed marks the summary as failed with an error message.
func (s *Summary) SetFailed(errMsg string) {
	s.ErrorMessage = errMsg
	s.Status = SummaryStatusFailed
	s.UpdatedAt = time.Now()
}

// SummaryStatusInfo is a lightweight struct for summary status aggregation
// (no content, just type/status). Used by get_recent_activity.
type SummaryStatusInfo struct {
	SummaryType string
	Status      string
}

// RecentActivityItem pairs a transcription with its summary statuses.
// Used by get_recent_activity to return combined data in a single call.
type RecentActivityItem struct {
	Transcription   *Transcription
	SummaryStatuses []SummaryStatusInfo
}

// Clone returns a deep copy of the summary.
func (s *Summary) Clone() *Summary {
	c := &Summary{
		ID:                            s.ID,
		OrganizationID:                s.OrganizationID,
		TranscriptionID:               s.TranscriptionID,
		SummaryType:                   s.SummaryType,
		Status:                        s.Status,
		WordCount:                     s.WordCount,
		ErrorMessage:                  s.ErrorMessage,
		PromptTokens:                  s.PromptTokens,
		CompletionTokens:              s.CompletionTokens,
		ThinkingTokens:                s.ThinkingTokens,
		HighStakes:                    s.HighStakes,
		ReviewStatus:                  s.ReviewStatus,
		SummaryProfile:                s.SummaryProfile,
		PromptVersion:                 s.PromptVersion,
		Model:                         s.Model,
		ModelMetadata:                 cloneBytes(s.ModelMetadata),
		EndpointLocation:              s.EndpointLocation,
		SourceVersion:                 s.SourceVersion,
		SourceHash:                    s.SourceHash,
		StructuredSchemaVersion:       s.StructuredSchemaVersion,
		DegradationCodes:              append([]string(nil), s.DegradationCodes...),
		CreatedAt:                     s.CreatedAt,
		UpdatedAt:                     s.UpdatedAt,
		Content:                       s.Content,
		TranscriptionMediaFilename:    s.TranscriptionMediaFilename,
		TranscriptionAudioName:        s.TranscriptionAudioName,
		TranscriptionMediaDescription: s.TranscriptionMediaDescription,
	}

	// Deep-copy CompletedAt pointer.
	if s.CompletedAt != nil {
		ca := *s.CompletedAt
		c.CompletedAt = &ca
	}

	// Deep-copy byte slices to prevent shared mutation (nil stays nil).
	c.ContentEncrypted = cloneBytes(s.ContentEncrypted)
	c.ContentNonce = cloneBytes(s.ContentNonce)
	c.WrappedDEK = cloneBytes(s.WrappedDEK)
	c.WrappingNonce = cloneBytes(s.WrappingNonce)
	c.ReviewFindingsEncrypted = cloneBytes(s.ReviewFindingsEncrypted)
	c.ReviewFindingsNonce = cloneBytes(s.ReviewFindingsNonce)
	c.ReviewWrappedDEK = cloneBytes(s.ReviewWrappedDEK)
	c.ReviewWrappingNonce = cloneBytes(s.ReviewWrappingNonce)
	c.ExtractionEncrypted = cloneBytes(s.ExtractionEncrypted)
	c.ExtractionNonce = cloneBytes(s.ExtractionNonce)
	c.ExtractionWrappedDEK = cloneBytes(s.ExtractionWrappedDEK)
	c.ExtractionWrappingNonce = cloneBytes(s.ExtractionWrappingNonce)
	c.StructuredContentCiphertext = cloneBytes(s.StructuredContentCiphertext)
	c.StructuredContentNonce = cloneBytes(s.StructuredContentNonce)
	c.StructuredContentWrappedDEK = cloneBytes(s.StructuredContentWrappedDEK)
	c.StructuredContentWrappingNonce = cloneBytes(s.StructuredContentWrappingNonce)
	c.StructuredContent = cloneBytes(s.StructuredContent)

	return c
}
