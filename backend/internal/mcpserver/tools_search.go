package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// SearchAllInput is the input for the search_all tool.
type SearchAllInput struct {
	Search string `json:"search" jsonschema:"Search query to match against media filenames, titles, descriptions, and URL transcription metadata (max 200 chars)"`
	Mode   string `json:"mode,omitempty" jsonschema:"Search mode: lexical or semantic (default lexical). Semantic search returns semantic_search_unavailable until an opt-in encrypted index is configured."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of items to return (default 20, max 100)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Number of items to skip for pagination"`
}

// ListTranscriptionsWithoutSummariesInput is the input for the
// list_transcriptions_without_summaries tool.
type ListTranscriptionsWithoutSummariesInput struct {
	SummaryType string `json:"summary_type,omitempty" jsonschema:"Filter by summary type: general, key_points, action_items, or q_and_a (Questions & Answers). Empty returns transcriptions missing any summary."`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of items to return (default 20, max 100)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"Number of items to skip for pagination"`
}

// SearchTranscriptSegmentsInput is the input for the search_transcript_segments tool.
type SearchTranscriptSegmentsInput struct {
	Search          string `json:"search" jsonschema:"Search query, max 200 chars"`
	TranscriptionID string `json:"transcription_id,omitempty" jsonschema:"Optional transcription UUID to search within"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum matching segments (default 10, max 50)"`
	Offset          int    `json:"offset,omitempty" jsonschema:"Pagination offset, max 5000"`
}

// registerSearchTools adds MCP search and cross-entity tools.
func registerSearchTools(server *toolRegistry, deps Dependencies) {
	registerTool(server, &mcp.Tool{
		Name: "search_all",
		Description: "Search across all media files and URL transcriptions. " +
			"Returns a mixed list with entity type, title, description, status, " +
			"and latest completed transcription ID for media results.",
	}, wrapTool[SearchAllInput, any]("search_all", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input SearchAllInput) (*mcp.CallToolResult, any, error) {
			return handleSearchAll(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "search_transcript_segments",
		Description: "Search bounded transcript segments lexically and return cited snippets with timestamps. Does not persist plaintext search indexes.",
	}, wrapTool[SearchTranscriptSegmentsInput, any]("search_transcript_segments", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input SearchTranscriptSegmentsInput) (*mcp.CallToolResult, any, error) {
			return handleSearchTranscriptSegments(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name: "list_transcriptions_without_summaries",
		Description: "List completed transcriptions (media and URL) that are missing " +
			"a completed summary of a given type. Useful for finding recordings that " +
			"need summarization.",
	}, wrapTool[ListTranscriptionsWithoutSummariesInput, any]("list_transcriptions_without_summaries", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input ListTranscriptionsWithoutSummariesInput) (*mcp.CallToolResult, any, error) {
			return handleListTranscriptionsWithoutSummaries(ctx, deps, input)
		}))

	registerCollectionTools(server, deps)
	registerRoleBriefTools(server, deps)
}

func handleSearchAll(ctx context.Context, deps Dependencies, input SearchAllInput) (*mcp.CallToolResult, any, error) {
	// Require both media:read AND transcription:read (AND semantics).
	if denied := checkScopes(ctx, []string{"media:read", "transcription:read"}); denied != nil {
		return denied, nil, nil
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	search, mode, limit, offset, err := normalizeSearchAllInput(input)
	if err != nil {
		return validationError(ctx, err), nil, nil
	}
	if mode == "semantic" {
		return errorResult(fmt.Errorf("semantic_search_unavailable: semantic search requires an opt-in encrypted semantic index")), nil, nil
	}
	if deps.MCPService == nil {
		return errorResult(fmt.Errorf("search service not available")), nil, nil
	}

	page, err := deps.MCPService.SearchAllPage(ctx, orgID, search, limit, offset)
	if err != nil {
		return toolError(ctx, deps.Logger, "search_all", "search all", err), nil, nil
	}

	r, jErr := jsonResult(service.UnifiedSearchResponseFromPage(page, search))
	return r, nil, jErr
}

func normalizeSearchAllInput(input SearchAllInput) (search, mode string, limit, offset int, err error) {
	search, err = service.NormalizeSearchQuery(input.Search)
	if err != nil {
		return "", "", 0, 0, err
	}
	if search == "" {
		return "", "", 0, 0, fmt.Errorf("search query cannot be empty")
	}
	mode = strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "lexical"
	}
	if mode != "lexical" && mode != "semantic" {
		return "", "", 0, 0, fmt.Errorf("invalid search mode")
	}
	if err := validateOffsetCap(input.Offset); err != nil {
		return "", "", 0, 0, err
	}
	return search, mode, clampLimit(input.Limit), clampOffset(input.Offset), nil
}

func handleSearchTranscriptSegments(
	ctx context.Context,
	deps Dependencies,
	input SearchTranscriptSegmentsInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "transcription:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	search, err := service.NormalizeSearchQuery(input.Search)
	if err != nil {
		return validationError(ctx, err), nil, nil
	}
	if search == "" {
		return validationError(ctx, fmt.Errorf("search query cannot be empty")), nil, nil
	}
	if input.Offset > 5000 {
		return validationError(ctx, fmt.Errorf("offset exceeds maximum of 5000")), nil, nil
	}
	if vErr := validateOffsetCap(input.Offset); vErr != nil {
		return validationError(ctx, vErr), nil, nil
	}
	if deps.MCPService == nil {
		return errorResult(fmt.Errorf("search service not available")), nil, nil
	}

	matches, err := deps.MCPService.SearchTranscriptSegments(ctx, orgID, port.SearchTranscriptSegmentsRequest{
		Search:          search,
		TranscriptionID: input.TranscriptionID,
		Limit:           input.Limit,
		Offset:          clampOffset(input.Offset),
	})
	if err != nil {
		return toolError(ctx, deps.Logger, "search_transcript_segments", "search transcript segments", err), nil, nil
	}

	result := struct {
		Items []MCPCitation `json:"items"`
		Count int           `json:"count"`
	}{
		Items: make([]MCPCitation, len(matches)),
		Count: len(matches),
	}
	for i, match := range matches {
		result.Items[i] = citationFromSegmentMatch(match)
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func handleListTranscriptionsWithoutSummaries(
	ctx context.Context,
	deps Dependencies,
	input ListTranscriptionsWithoutSummariesInput,
) (*mcp.CallToolResult, any, error) {
	// Require both transcription:read AND summary:read (AND semantics).
	if denied := checkScopes(ctx, []string{"transcription:read", "summary:read"}); denied != nil {
		return denied, nil, nil
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	// Validate summary_type if provided.
	if vErr := validateSummaryType(input.SummaryType); vErr != nil {
		return validationError(ctx, vErr), nil, nil
	}
	if vErr := validateOffsetCap(input.Offset); vErr != nil {
		return validationError(ctx, vErr), nil, nil
	}

	if deps.MCPService == nil {
		return errorResult(fmt.Errorf("search service not available")), nil, nil
	}

	results, err := deps.MCPService.ListTranscriptionsWithoutSummaries(
		ctx, orgID, input.SummaryType, clampLimit(input.Limit), clampOffset(input.Offset),
	)
	if err != nil {
		return toolError(ctx, deps.Logger, "list_transcriptions_without_summaries", "list transcriptions without summaries", err), nil, nil
	}

	type transItem struct {
		ID              string  `json:"id"`
		SourceType      string  `json:"source_type"`
		Status          string  `json:"status"`
		WordCount       int     `json:"word_count"`
		SpeakerCount    int     `json:"speaker_count"`
		DurationSeconds float64 `json:"duration_seconds"`
		CreatedAt       string  `json:"created_at"`
		DisplayName     string  `json:"display_name"`
		Description     string  `json:"description,omitempty"`
	}

	items := make([]transItem, len(results))
	for i, r := range results {
		items[i] = transItem{
			ID:              r.ID,
			SourceType:      r.SourceType,
			Status:          r.Status,
			WordCount:       r.WordCount,
			SpeakerCount:    r.SpeakerCount,
			DurationSeconds: r.DurationSeconds,
			CreatedAt:       r.CreatedAt,
			DisplayName:     r.DisplayName,
			Description:     r.Description,
		}
	}

	result := struct {
		Items []transItem `json:"items"`
	}{
		Items: items,
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func citationFromSegmentMatch(match port.TranscriptSegmentMatch) MCPCitation {
	return MCPCitation{
		TranscriptionID: match.TranscriptionID,
		MediaID:         match.MediaID,
		SourceType:      match.SourceType,
		DisplayName:     match.DisplayName,
		Speaker:         match.Speaker,
		StartSeconds:    match.StartSeconds,
		EndSeconds:      match.EndSeconds,
		Text:            match.Text,
	}
}
