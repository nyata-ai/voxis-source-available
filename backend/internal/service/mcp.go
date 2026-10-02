package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// MCPService provides MCP-specific operations that compose
// cross-entity queries.
type MCPService struct {
	mcpRepo  port.MCPRepository
	transSvc *TranscriptionService
	answerer port.TranscriptAnswerer
	logger   *slog.Logger
}

// MCPServiceOption configures MCPService dependencies.
type MCPServiceOption func(*MCPService)

// WithMCPTranscriptionService enables transcript-content MCP operations.
func WithMCPTranscriptionService(transSvc *TranscriptionService) MCPServiceOption {
	return func(s *MCPService) {
		s.transSvc = transSvc
	}
}

// WithMCPTranscriptAnswerer enables model-backed MCP analysis operations.
func WithMCPTranscriptAnswerer(answerer port.TranscriptAnswerer) MCPServiceOption {
	return func(s *MCPService) {
		s.answerer = answerer
	}
}

// NewMCPService creates a new MCPService.
func NewMCPService(mcpRepo port.MCPRepository, logger *slog.Logger, opts ...MCPServiceOption) *MCPService {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &MCPService{
		mcpRepo: mcpRepo,
		logger:  logger,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// SearchAll performs a unified search across media and URL transcription metadata.
// Validates that search is non-empty and does not exceed maxLen.
func (s *MCPService) SearchAll(ctx context.Context, orgID, search string, limit, offset int) ([]port.SearchAllResult, error) {
	page, err := s.SearchAllPage(ctx, orgID, search, limit, offset)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// SearchAllPage performs paginated lexical search and returns result metadata
// shared by REST and MCP callers.
func (s *MCPService) SearchAllPage(ctx context.Context, orgID, search string, limit, offset int) (*port.SearchAllPage, error) {
	var err error
	search, err = NormalizeSearchQuery(search)
	if err != nil {
		return nil, err
	}
	if search == "" {
		return nil, fmt.Errorf("search query cannot be empty: %w", domain.ErrInvalidInput)
	}
	if s.mcpRepo == nil {
		return nil, fmt.Errorf("search repository not configured")
	}
	limit = clampUnifiedSearchLimit(limit)
	if offset < 0 {
		offset = 0
	}
	if offsetErr := ValidateSearchOffset(offset); offsetErr != nil {
		return nil, offsetErr
	}

	results, err := s.mcpRepo.SearchAllEntities(ctx, orgID, search, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search all: %w", err)
	}
	total, err := s.mcpRepo.CountSearchAllEntities(ctx, orgID, search)
	if err != nil {
		return nil, fmt.Errorf("count search all: %w", err)
	}
	return &port.SearchAllPage{
		Items:             enrichSearchResults(results, search),
		Total:             total,
		SearchMode:        "lexical",
		SemanticAvailable: false,
	}, nil
}

func clampUnifiedSearchLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func enrichSearchResults(results []port.SearchAllResult, query string) []port.SearchAllResult {
	out := make([]port.SearchAllResult, len(results))
	for i, r := range results {
		r.Score, r.MatchFields, r.Snippet = searchMetadata(r, query)
		out[i] = r
	}
	return out
}

func searchMetadata(r port.SearchAllResult, query string) (score int, matchFields []string, snippet string) {
	q := strings.ToLower(query)
	fields := make([]string, 0, 3)
	if fieldContains(r.Title, q) {
		fields = append(fields, "title")
		score += titleScore(r.Title, q)
	}
	if fieldContains(r.Filename, q) {
		fields = append(fields, "filename")
		score += 50
	}
	if fieldContains(r.Description, q) {
		fields = append(fields, "description")
		score += 25
	}
	if fieldContains(r.SourceURL, q) {
		fields = append(fields, "source_url")
		score += 50
	}
	return score, fields, firstSnippet(query, r.Title, r.Filename, r.Description, r.SourceURL)
}

func fieldContains(value, lowerQuery string) bool {
	return strings.Contains(strings.ToLower(value), lowerQuery)
}

func titleScore(title, lowerQuery string) int {
	lowerTitle := strings.ToLower(title)
	switch {
	case lowerTitle == lowerQuery:
		return 100
	case strings.HasPrefix(lowerTitle, lowerQuery):
		return 80
	default:
		return 50
	}
}

func firstSnippet(query string, values ...string) string {
	lowerQuery := strings.ToLower(query)
	for _, value := range values {
		if fieldContains(value, lowerQuery) {
			return trimSnippet(value, 160)
		}
	}
	if len(values) == 0 {
		return ""
	}
	return trimSnippet(values[0], 160)
}

func trimSnippet(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes-1]) + "..."
}

// ListTranscriptionsWithoutSummaries returns completed transcriptions
// that lack a completed summary of the given type.
func (s *MCPService) ListTranscriptionsWithoutSummaries(ctx context.Context, orgID, summaryType string, limit, offset int) ([]port.TranscriptionWithoutSummary, error) {
	results, err := s.mcpRepo.ListTranscriptionsWithoutSummaries(ctx, orgID, summaryType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list transcriptions without summaries: %w", err)
	}
	return results, nil
}
