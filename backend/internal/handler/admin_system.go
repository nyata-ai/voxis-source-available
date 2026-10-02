package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/port"
)

// AdminSystemHandler serves admin system observability stats.
type AdminSystemHandler struct {
	collector port.SystemStatsCollector
	logger    *slog.Logger
}

// NewAdminSystemHandler creates an AdminSystemHandler.
func NewAdminSystemHandler(collector port.SystemStatsCollector, logger *slog.Logger) *AdminSystemHandler {
	if collector == nil {
		panic("handler: system collector must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminSystemHandler{collector: collector, logger: logger}
}

// GetStats handles GET /api/v1/admin/system/stats.
func (h *AdminSystemHandler) GetStats(c *gin.Context) {
	stats, err := h.collector.GetSystemStats(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get admin system stats",
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to retrieve admin system statistics",
		})
		return
	}
	c.JSON(http.StatusOK, stats)
}
