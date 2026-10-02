package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/port"
)

// AdminOpsHandler handles zero-knowledge admin operations endpoints.
type AdminOpsHandler struct {
	adminRepo port.AdminRepository
	now       func() time.Time
	logger    *slog.Logger
}

// NewAdminOpsHandler creates an AdminOpsHandler.
func NewAdminOpsHandler(adminRepo port.AdminRepository, now func() time.Time, logger *slog.Logger) *AdminOpsHandler {
	if adminRepo == nil {
		panic("handler: adminRepo must not be nil")
	}
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminOpsHandler{
		adminRepo: adminRepo,
		now:       now,
		logger:    logger,
	}
}

// GetStats handles GET /api/v1/admin/ops/stats.
func (h *AdminOpsHandler) GetStats(c *gin.Context) {
	stats, err := h.adminRepo.GetOpsStats(c.Request.Context(), h.now().UTC())
	if err != nil {
		h.logger.Error("failed to get admin operations stats",
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to retrieve admin operations statistics",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}
