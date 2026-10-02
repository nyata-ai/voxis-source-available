package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type unifiedSearchService interface {
	SearchAllPage(ctx context.Context, orgID, search string, limit, offset int) (*port.SearchAllPage, error)
}

// SearchHandler handles unified cross-entity search.
type SearchHandler struct {
	searchSvc  unifiedSearchService
	orgService *service.OrganizationService
	logger     *slog.Logger
}

// SearchResponse is the JSON response for the unified search endpoint.
type SearchResponse = service.UnifiedSearchResponse

// NewSearchHandler creates a SearchHandler.
func NewSearchHandler(searchSvc unifiedSearchService, orgService *service.OrganizationService, logger *slog.Logger) *SearchHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SearchHandler{searchSvc: searchSvc, orgService: orgService, logger: logger}
}

// Search handles GET /api/v1/search.
func (h *SearchHandler) Search(c *gin.Context) {
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
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "semantic_search_unavailable",
			"message": "semantic search requires an opt-in encrypted semantic index",
		})
		return
	}
	page, err := h.searchSvc.SearchAllPage(c.Request.Context(), orgID, query, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, service.UnifiedSearchResponseFromPage(page, query))
}

func (h *SearchHandler) handleError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
		return
	}
	h.logger.Error("search handler error", "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
}

func parseSearchRequest(c *gin.Context) (query, mode string, limit, offset int, err error) {
	query, err = service.NormalizeSearchQuery(c.Query("search"))
	if err != nil {
		return "", "", 0, 0, err
	}
	if query == "" {
		return "", "", 0, 0, fmt.Errorf("search query cannot be empty: %w", domain.ErrInvalidInput)
	}

	mode = normalizedSearchMode(c.DefaultQuery("mode", "lexical"))
	if mode != "lexical" && mode != "semantic" {
		return "", "", 0, 0, fmt.Errorf("invalid search mode: %w", domain.ErrInvalidInput)
	}

	limit = parseIntQuery(c, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	offset = parseIntQuery(c, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	if offsetErr := service.ValidateSearchOffset(offset); offsetErr != nil {
		return "", "", 0, 0, offsetErr
	}
	return query, mode, limit, offset, nil
}

func normalizedSearchMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "lexical"
	}
	return mode
}
