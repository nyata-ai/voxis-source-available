package port

import "context"

// SearchAllResult represents a single retained media result.
type SearchAllResult struct {
	ID                    string
	EntityType            string // always "media" in Voxis-OSS
	Title                 string
	Description           string
	Filename              string
	SourceURL             string
	Status                string
	CreatedAt             string
	LatestTranscriptionID string // empty string when not available
	Score                 int
	MatchFields           []string
	Snippet               string
}

// SearchAllPage represents a paginated metadata search result set.
type SearchAllPage struct {
	Items             []SearchAllResult
	Total             int64
	SearchMode        string
	SemanticAvailable bool
}

// TranscriptionWithoutSummary represents a transcription missing a summary.
type TranscriptionWithoutSummary struct {
	ID              string
	SourceType      string
	Status          string
	WordCount       int
	SpeakerCount    int
	DurationSeconds float64
	CreatedAt       string
	DisplayName     string
	Description     string
}

// SearchTranscriptSegmentsRequest configures bounded lexical transcript search.
type SearchTranscriptSegmentsRequest struct {
	Search          string
	TranscriptionID string
	Limit           int
	Offset          int
}

// TranscriptSegmentMatch represents a cited transcript segment search result.
type TranscriptSegmentMatch struct {
	TranscriptionID string
	MediaID         string
	SourceType      string
	DisplayName     string
	CreatedAt       string
	Speaker         string
	StartSeconds    float64
	EndSeconds      float64
	Text            string
	Score           int
}

// MCPCollection represents a named transcript collection for MCP workflows.
type MCPCollection struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	CreatedBy      string
	CreatedAt      string
	UpdatedAt      string
	ItemCount      int
}

// MCPCollectionItem represents a transcription in a collection.
type MCPCollectionItem struct {
	CollectionID    string
	TranscriptionID string
	AddedAt         string
}

// MCPRepository provides MCP-specific cross-entity queries.
type MCPRepository interface {
	SearchAllEntities(ctx context.Context, orgID, search string, limit, offset int) ([]SearchAllResult, error)
	CountSearchAllEntities(ctx context.Context, orgID, search string) (int64, error)
	ListTranscriptionsWithoutSummaries(ctx context.Context, orgID, summaryType string, limit, offset int) ([]TranscriptionWithoutSummary, error)
	CreateCollection(ctx context.Context, orgID, name, description, createdBy string) (*MCPCollection, error)
	ListCollections(ctx context.Context, orgID string, limit, offset int) ([]MCPCollection, error)
	GetCollection(ctx context.Context, orgID, collectionID string) (*MCPCollection, error)
	AddCollectionItem(ctx context.Context, orgID, collectionID, transcriptionID string) error
	RemoveCollectionItem(ctx context.Context, orgID, collectionID, transcriptionID string) error
	ListCollectionItems(ctx context.Context, orgID, collectionID string, limit, offset int) ([]MCPCollectionItem, error)
}
