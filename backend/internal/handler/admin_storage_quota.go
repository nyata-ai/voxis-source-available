package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
)

// AdminStorageQuotaHandler manages the single global storage quota policy.
type AdminStorageQuotaHandler struct {
	repo    port.StorageQuotaRepository
	backend string
}

// NewAdminStorageQuotaHandler builds the handler; the repository is required.
func NewAdminStorageQuotaHandler(repo port.StorageQuotaRepository, backend string) *AdminStorageQuotaHandler {
	if repo == nil {
		panic("handler: storage quota repository must not be nil")
	}
	return &AdminStorageQuotaHandler{repo: repo, backend: backend}
}

type storageQuotaPolicyResponse struct {
	domain.StorageQuotaPolicy
	Backend string `json:"backend"`
	Applies bool   `json:"applies"`
}

// Get returns the current storage quota policy with backend applicability.
func (h *AdminStorageQuotaHandler) Get(c *gin.Context) {
	policy, err := h.repo.GetStorageQuotaPolicy(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to retrieve storage quota policy"})
		return
	}
	c.JSON(http.StatusOK, h.response(policy))
}

type updateStorageQuotaRequest struct {
	Enabled           *bool `json:"enabled"`
	DefaultLimitBytes int64 `json:"default_limit_bytes"`
}

// Update validates and stores a new storage quota policy, attributing the
// change to the authenticated admin.
func (h *AdminStorageQuotaHandler) Update(c *gin.Context) {
	var req updateStorageQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil || req.DefaultLimitBytes <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "enabled and a positive default_limit_bytes are required"})
		return
	}
	claims := middleware.GetClaims(c)
	if claims == nil || claims.Subject == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "authentication required"})
		return
	}
	policy, err := h.repo.UpdateStorageQuotaPolicy(c.Request.Context(), domain.StorageQuotaPolicy{
		Enabled:                 *req.Enabled,
		DefaultLimitBytes:       req.DefaultLimitBytes,
		WarningThresholdPercent: domain.StorageQuotaWarningThresholdPercent,
	}, claims.Subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to update storage quota policy"})
		return
	}
	c.JSON(http.StatusOK, h.response(policy))
}

func (h *AdminStorageQuotaHandler) response(policy domain.StorageQuotaPolicy) storageQuotaPolicyResponse {
	return storageQuotaPolicyResponse{
		StorageQuotaPolicy: policy,
		Backend:            h.backend,
		Applies:            domain.IsBucketStorageBackend(h.backend),
	}
}
