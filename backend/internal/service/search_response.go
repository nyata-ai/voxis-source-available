package service

import "github.com/voxis/backend/internal/port"

// UnifiedSearchItem is the serialized search item contract shared by REST and
// MCP so field parity is enforced in one mapper.
type UnifiedSearchItem struct {
	ID                    string   `json:"id"`
	EntityType            string   `json:"entity_type"`
	Title                 string   `json:"title"`
	Description           string   `json:"description,omitempty"`
	Filename              string   `json:"filename,omitempty"`
	SourceURL             string   `json:"source_url,omitempty"`
	Status                string   `json:"status"`
	CreatedAt             string   `json:"created_at"`
	LatestTranscriptionID string   `json:"latest_transcription_id,omitempty"`
	Score                 int      `json:"score"`
	MatchFields           []string `json:"match_fields,omitempty"`
	Snippet               string   `json:"snippet,omitempty"`
}

// UnifiedSearchResponse is the serialized paginated search contract shared by
// REST and MCP.
type UnifiedSearchResponse struct {
	Items             []UnifiedSearchItem `json:"items"`
	Total             int64               `json:"total"`
	Query             string              `json:"query"`
	SearchMode        string              `json:"search_mode"`
	SemanticAvailable bool                `json:"semantic_available"`
}

// UnifiedSearchResponseFromPage maps a repository search page into the shared
// REST/MCP response contract. A nil page yields an empty response.
func UnifiedSearchResponseFromPage(page *port.SearchAllPage, query string) UnifiedSearchResponse {
	if page == nil {
		return UnifiedSearchResponse{Items: []UnifiedSearchItem{}, Query: query}
	}
	items := make([]UnifiedSearchItem, len(page.Items))
	for i, item := range page.Items {
		items[i] = UnifiedSearchItem{
			ID:                    item.ID,
			EntityType:            item.EntityType,
			Title:                 item.Title,
			Description:           item.Description,
			Filename:              item.Filename,
			SourceURL:             item.SourceURL,
			Status:                item.Status,
			CreatedAt:             item.CreatedAt,
			LatestTranscriptionID: item.LatestTranscriptionID,
			Score:                 item.Score,
			MatchFields:           item.MatchFields,
			Snippet:               item.Snippet,
		}
	}
	return UnifiedSearchResponse{
		Items:             items,
		Total:             page.Total,
		Query:             query,
		SearchMode:        page.SearchMode,
		SemanticAvailable: page.SemanticAvailable,
	}
}
