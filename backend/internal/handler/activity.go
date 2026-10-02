package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

type activityService interface {
	GetActivity(ctx context.Context, orgID, userID string) ([]domain.ActivityItem, error)
}

// ActivityHandler handles activity feed HTTP requests.
type ActivityHandler struct {
	activitySvc activityService
	orgService  *service.OrganizationService
	logger      *slog.Logger
}

// NewActivityHandler creates a new ActivityHandler.
func NewActivityHandler(
	activitySvc activityService,
	orgService *service.OrganizationService,
	logger *slog.Logger,
) *ActivityHandler {
	if activitySvc == nil {
		panic("activity handler: activity service cannot be nil")
	}
	if orgService == nil {
		panic("activity handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ActivityHandler{
		activitySvc: activitySvc,
		orgService:  orgService,
		logger:      logger,
	}
}

type activityDetailResponse struct {
	Terminal int `json:"terminal"`
	Total    int `json:"total"`
}

type activityItemResponse struct {
	Kind      string                  `json:"kind"`
	RefID     string                  `json:"ref_id"`
	Link      string                  `json:"link,omitempty"`
	Title     string                  `json:"title,omitempty"`
	Stage     string                  `json:"stage"`
	Status    string                  `json:"status"`
	ErrorMsg  string                  `json:"error_message,omitempty"`
	Detail    *activityDetailResponse `json:"detail,omitempty"`
	StartedAt string                  `json:"started_at"`
}

// GetActivity handles GET /api/v1/activity.
func (h *ActivityHandler) GetActivity(c *gin.Context) {
	orgID, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	items, err := h.activitySvc.GetActivity(c.Request.Context(), orgID, claims.Subject)
	if err != nil {
		h.handleError(c, err)
		return
	}

	out := make([]activityItemResponse, 0, len(items))
	for i := range items {
		out = append(out, toActivityResponse(items[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func toActivityResponse(item domain.ActivityItem) activityItemResponse {
	resp := activityItemResponse{
		Kind:      item.Kind,
		RefID:     item.RefID,
		Link:      item.Link,
		Title:     item.Title,
		Stage:     item.Stage,
		Status:    item.Status,
		ErrorMsg:  item.ErrorMessage,
		StartedAt: item.StartedAt.Format(time.RFC3339),
	}
	if item.Detail != nil {
		resp.Detail = &activityDetailResponse{
			Terminal: item.Detail.Terminal,
			Total:    item.Detail.Total,
		}
	}
	return resp
}

// handleError maps domain errors to HTTP status codes.
func (h *ActivityHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "activity not found"})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "access denied"})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": extractMessage(err)})
	case errors.Is(err, domain.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": extractMessage(err)})
	default:
		h.logger.Error("activity handler error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
	}
}
