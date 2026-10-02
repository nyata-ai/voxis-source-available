package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// DashboardHandler handles dashboard-related HTTP endpoints.
type DashboardHandler struct {
	dashRepo   port.DashboardRepository
	orgService *service.OrganizationService
	logger     *slog.Logger
}

// NewDashboardHandler creates a new dashboard handler.
// Panics if dashRepo or orgService is nil (programming error).
func NewDashboardHandler(dashRepo port.DashboardRepository, orgService *service.OrganizationService, logger *slog.Logger) *DashboardHandler {
	if dashRepo == nil {
		panic("handler: dashRepo must not be nil")
	}
	if orgService == nil {
		panic("handler: orgService must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DashboardHandler{
		dashRepo:   dashRepo,
		orgService: orgService,
		logger:     logger,
	}
}

// GetStats handles GET /api/v1/dashboard/stats.
// Returns aggregated dashboard statistics for the authenticated user's organization.
func (h *DashboardHandler) GetStats(c *gin.Context) {
	orgID, _ := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if orgID == "" {
		return // error already written by resolveOrgIDOrAPIKey
	}

	stats, err := h.dashRepo.GetStats(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to get dashboard stats",
			"error", err,
			"org_id", orgID,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to retrieve dashboard statistics",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
