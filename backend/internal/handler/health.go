package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/port"
)

const healthCheckTimeout = 5 * time.Second

// HealthHandler handles health check endpoints
type HealthHandler struct {
	db     port.HealthChecker
	vault  port.HealthChecker
	logger *slog.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(db, vault port.HealthChecker, logger *slog.Logger) *HealthHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthHandler{db: db, vault: vault, logger: logger}
}

// Liveness handles GET /health - liveness probe
// Used by orchestration to determine if container should be restarted
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":   "alive",
		"instance": config.GetInstanceID(),
	})
}

// Readiness handles GET /ready - readiness probe
// Used by load balancers to determine if instance should receive traffic
func (h *HealthHandler) Readiness(c *gin.Context) {
	checks := make(map[string]string)
	status := http.StatusOK
	statusText := "ready"

	// Create context with timeout for health checks
	ctx, cancel := context.WithTimeout(c.Request.Context(), healthCheckTimeout)
	defer cancel()

	// Check if any health checkers are configured
	hasCheckers := false

	// Check database connectivity
	if h.db != nil {
		hasCheckers = true
		if err := h.db.Ping(ctx); err != nil {
			h.logger.Error("database health check failed",
				"error", err,
				"request_id", c.GetString("request_id"),
			)
			checks["database"] = "unhealthy"
			status = http.StatusServiceUnavailable
			statusText = "not_ready"
		} else {
			checks["database"] = "healthy"
		}
	}

	// Check Vault connectivity
	if h.vault != nil {
		hasCheckers = true
		if err := h.vault.Ping(ctx); err != nil {
			h.logger.Error("vault health check failed",
				"error", err,
				"request_id", c.GetString("request_id"),
			)
			checks["vault"] = "unhealthy"
			status = http.StatusServiceUnavailable
			statusText = "not_ready"
		} else {
			checks["vault"] = "healthy"
		}
	}

	// If no health checkers configured, report degraded status
	if !hasCheckers {
		status = http.StatusServiceUnavailable
		statusText = "degraded"
		checks["_warning"] = "no health checkers configured"
	}

	c.JSON(status, gin.H{
		"status":   statusText,
		"instance": config.GetInstanceID(),
		"checks":   checks,
	})
}
