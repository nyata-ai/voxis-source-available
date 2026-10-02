package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type ossUnifiedSearchService interface {
	SearchOSSPages(context.Context, string, string, int, int, string) (*port.SearchAllPage, *service.TranscriptSearchPage, error)
}

// OSSSearchHandler keeps metadata pagination separate from bounded decrypted
// transcript-content search. The response never claims a combined total.
type OSSSearchHandler struct {
	searchSvc  ossUnifiedSearchService
	orgService *service.OrganizationService
	logger     *slog.Logger
}

type ossUnifiedSearchResponse struct {
	Items                []service.UnifiedSearchItem `json:"items"`
	Total                int64                       `json:"total"`
	Query                string                      `json:"query"`
	SearchMode           string                      `json:"search_mode"`
	SemanticAvailable    bool                        `json:"semantic_available"`
	TranscriptItems      []service.UnifiedSearchItem `json:"transcript_items"`
	NextTranscriptCursor string                      `json:"next_transcript_cursor,omitempty"`
	TranscriptComplete   bool                        `json:"transcript_complete"`
}

// NewOSSSearchHandler creates the bounded metadata and transcript search route.
func NewOSSSearchHandler(searchSvc ossUnifiedSearchService, orgService *service.OrganizationService, logger *slog.Logger) *OSSSearchHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &OSSSearchHandler{searchSvc: searchSvc, orgService: orgService, logger: logger}
}

// Search handles the retained GET /api/v1/search contract.
func (h *OSSSearchHandler) Search(c *gin.Context) {
	orgID, _ := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if orgID == "" {
		return
	}
	query, mode, limit, offset, err := parseSearchRequest(c)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if mode == "semantic" {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "semantic_search_unavailable", "message": "semantic search requires an opt-in encrypted semantic index"})
		return
	}
	metadata, transcripts, err := h.searchSvc.SearchOSSPages(c.Request.Context(), orgID, query, limit, offset, c.Query("transcript_cursor"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ossSearchResponseFromPages(metadata, transcripts, query))
}

func (h *OSSSearchHandler) handleError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
		return
	}
	h.logger.Error("OSS search handler error", "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
}

func ossSearchResponseFromPages(metadata *port.SearchAllPage, transcripts *service.TranscriptSearchPage, query string) ossUnifiedSearchResponse {
	page := service.UnifiedSearchResponseFromPage(metadata, query)
	response := ossUnifiedSearchResponse{
		Items: page.Items, Total: page.Total, Query: page.Query, SearchMode: page.SearchMode, SemanticAvailable: page.SemanticAvailable,
		TranscriptItems: []service.UnifiedSearchItem{}, TranscriptComplete: true,
	}
	if transcripts == nil {
		return response
	}
	response.TranscriptItems = ossTranscriptSearchItems(transcripts.Matches)
	response.NextTranscriptCursor = transcripts.NextCursor
	response.TranscriptComplete = transcripts.Complete
	return response
}

func ossTranscriptSearchItems(matches []port.TranscriptSegmentMatch) []service.UnifiedSearchItem {
	items := make([]service.UnifiedSearchItem, len(matches))
	for index, match := range matches {
		items[index] = service.UnifiedSearchItem{
			ID: match.MediaID, EntityType: "media", Title: match.DisplayName, Filename: match.DisplayName,
			Status: "completed", CreatedAt: match.CreatedAt, LatestTranscriptionID: match.TranscriptionID,
			Score: match.Score, MatchFields: []string{"transcript"}, Snippet: ossSearchSnippet(match.Text),
		}
	}
	return items
}

func ossSearchSnippet(value string) string {
	value = strings.TrimSpace(value)
	end := 0
	for count := 0; end < len(value) && count < 160; count++ {
		_, size := utf8.DecodeRuneInString(value[end:])
		end += size
	}
	if end == len(value) {
		return value
	}
	return value[:end] + "..."
}
