package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// SummaryRepository handles summary persistence.
type SummaryRepository interface {
	// Create persists a new summary record.
	Create(ctx context.Context, s *domain.Summary) error

	// GetByID retrieves a summary record by ID.
	// Returns domain.ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*domain.Summary, error)

	// GetActiveByTranscriptionAndType returns the active summary (status NOT IN
	// 'failed','deleted') for the given transcription and type.
	// Returns domain.ErrNotFound if no active summary exists.
	GetActiveByTranscriptionAndType(ctx context.Context, transcriptionID, summaryType string) (*domain.Summary, error)

	// ListByTranscription retrieves all summaries for a transcription
	// ordered by creation time descending.
	ListByTranscription(ctx context.Context, transcriptionID string) ([]*domain.Summary, error)

	// ListByOrganization retrieves summaries for an organization
	// ordered by creation time descending, with pagination.
	// If search is non-empty, filters by audio name (title fallback filename)
	// and media description.
	ListByOrganization(ctx context.Context, orgID string, limit, offset int, search string) ([]*domain.Summary, error)

	// CountByOrganization returns the total number of non-deleted summaries
	// for the given organization. If search is non-empty, counts only matching items.
	CountByOrganization(ctx context.Context, orgID string, search string) (int64, error)

	// Update updates an existing summary record.
	Update(ctx context.Context, s *domain.Summary) error

	// Delete soft-deletes a summary by setting status to "deleted".
	// Returns domain.ErrNotFound if the summary does not exist.
	Delete(ctx context.Context, id string) error

	// DeleteByTranscriptionID soft-deletes all non-deleted summaries for the given transcription.
	// Returns the number of summaries affected.
	DeleteByTranscriptionID(ctx context.Context, transcriptionID string) (int64, error)

	// DeleteByMediaID soft-deletes all non-deleted summaries whose transcription
	// belongs to the given media. Returns the number of summaries affected.
	DeleteByMediaID(ctx context.Context, mediaID string) (int64, error)

	// UndoDelete restores a soft-deleted summary to the given status.
	// Returns domain.ErrNotFound if no deleted summary with the given id and org exists.
	UndoDelete(ctx context.Context, id, orgID, restoreStatus string) error

	// ListSummaryStatusesByTranscriptionIDs returns summary type+status pairs
	// grouped by transcription ID. Only non-deleted summaries are included.
	// Used for summary status aggregation in get_recent_activity.
	ListSummaryStatusesByTranscriptionIDs(ctx context.Context, transcriptionIDs []string) (map[string][]domain.SummaryStatusInfo, error)

	// ListByOrganizationFiltered retrieves summaries (both source types)
	// with optional filters (date range, status, summary_type, search, sort).
	// Used by MCP list_summaries.
	ListByOrganizationFiltered(ctx context.Context, orgID string, filter SummaryListFilter) ([]*domain.Summary, error)

	// ClearArtifactsByMediaID clears high-stakes and structured-content blobs
	// for summaries attached to a privilege media item.
	ClearArtifactsByMediaID(ctx context.Context, mediaID string) (int64, error)
}

// SummaryProvider generates AI summaries from text.
type SummaryProvider interface {
	GenerateSummary(ctx context.Context, req SummaryRequest) (*SummaryResult, error)
}

// AnchoredSummaryProvider supports the high-stakes two-pass summary pipeline.
type AnchoredSummaryProvider interface {
	ExtractSummaryAnchors(ctx context.Context, req SummaryExtractionRequest) (*SummaryExtractionResult, error)
	GenerateSummaryFromAnchors(ctx context.Context, req AnchoredSummaryRequest) (*SummaryResult, error)
}

// SummaryRequest contains the input for an AI summary generation.
type SummaryRequest struct {
	Text             string
	SummaryType      string
	Language         string // hint from transcription language; empty string = omit hint
	SummaryProfile   string // snapshotted professional emphasis profile
	SourceVersion    string // deterministic source format version
	SourceHash       string // SHA-256 of the exact Text bytes sent to the provider
	SourceChunkIndex int
	SourceChunkCount int
	ForceLegacy      bool // bypass schema mode for a bounded degradation fallback
}

// SummaryExtractionRequest contains the input for source-anchor extraction.
type SummaryExtractionRequest struct {
	Text             string
	SummaryType      string
	Language         string
	SummaryProfile   string
	SourceVersion    string
	SourceHash       string
	SourceChunkIndex int
	SourceChunkCount int
}

// SummaryExtractionResult contains the durable extraction artifact.
type SummaryExtractionResult struct {
	ExtractionJSON   string
	PromptTokens     int
	CompletionTokens int
	ThinkingTokens   int
	// UsageAvailable distinguishes provider-reported zero token usage from an
	// unavailable usage report. Callers must never estimate unavailable usage.
	UsageAvailable bool
}

// AnchoredSummaryRequest contains the input for compression from anchors.
type AnchoredSummaryRequest struct {
	ExtractionJSON   string
	Transcript       string
	SummaryType      string
	Language         string
	SummaryProfile   string
	SourceVersion    string
	SourceHash       string
	SourceChunkIndex int
	SourceChunkCount int
}

// SummaryResult contains the output from an AI summary generation.
type SummaryResult struct {
	Content                 string
	StructuredContent       []byte
	StructuredSchemaVersion string
	PromptVersion           string
	Model                   string
	ModelRevision           string
	RuntimeRevision         string
	Quantization            string
	EndpointLocation        string
	SourceVersion           string
	SourceHash              string
	DegradationCodes        []string
	WordCount               int
	PromptTokens            int
	CompletionTokens        int
	ThinkingTokens          int
	// UsageAvailable distinguishes provider-reported zero token usage from an
	// unavailable usage report. Callers must never estimate unavailable usage.
	UsageAvailable bool
	ExtractionJSON string
}

// SummaryJobInserter enqueues summary processing jobs.
type SummaryJobInserter interface {
	InsertSummarizeJob(ctx context.Context, summaryID, transcriptionID, orgID string) error
}
