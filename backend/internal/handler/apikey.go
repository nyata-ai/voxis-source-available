package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

// APIKeyHandler handles API key CRUD HTTP requests.
type APIKeyHandler struct {
	apiKeySvc     *service.APIKeyService
	orgService    *service.OrganizationService
	publicBaseURL string
	logger        *slog.Logger
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(
	apiKeySvc *service.APIKeyService,
	orgService *service.OrganizationService,
	publicBaseURL string,
	logger *slog.Logger,
) *APIKeyHandler {
	if apiKeySvc == nil {
		panic("apikey handler: API key service cannot be nil")
	}
	if orgService == nil {
		panic("apikey handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &APIKeyHandler{
		apiKeySvc:     apiKeySvc,
		orgService:    orgService,
		publicBaseURL: publicBaseURL,
		logger:        logger,
	}
}

type createAPIKeyRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays *int     `json:"expires_in_days"`
}

type apiKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Key        string     `json:"key,omitempty"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     []string   `json:"scopes"`
	MCPURL     string     `json:"mcp_url,omitempty"`
	APIURL     string     `json:"api_url,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Create handles POST /api/v1/api-keys.
func (h *APIKeyHandler) Create(c *gin.Context) {
	orgID, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if claims == nil {
		return // error already written
	}

	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "invalid JSON body",
		})
		return
	}

	// Calculate expiry
	var expiresAt *time.Time
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		if *req.ExpiresInDays > 365 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "bad_request",
				"message": "expires_in_days must be at most 365",
			})
			return
		}
		t := time.Now().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &t
	}

	fullKey, key, err := h.apiKeySvc.Create(
		c.Request.Context(),
		orgID,
		claims.Subject,
		req.Name,
		req.Scopes,
		expiresAt,
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "bad_request",
				"message": err.Error(),
			})
		case errors.Is(err, domain.ErrConflict):
			c.JSON(http.StatusConflict, gin.H{
				"error":   "conflict",
				"message": err.Error(),
			})
		default:
			h.logger.Error("failed to create API key",
				"error", err,
				"org_id", orgID,
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "failed to create API key",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, apiKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Key:       fullKey,
		KeyPrefix: key.KeyPrefix,
		Scopes:    key.Scopes,
		MCPURL:    h.publicBaseURL + "/mcp",
		APIURL:    h.publicBaseURL + "/api/v1",
		ExpiresAt: key.ExpiresAt,
		CreatedAt: key.CreatedAt,
	})
}

// List handles GET /api/v1/api-keys.
func (h *APIKeyHandler) List(c *gin.Context) {
	orgID, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if claims == nil {
		return // error already written
	}

	keys, err := h.apiKeySvc.List(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to list API keys",
			"error", err,
			"org_id", orgID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to list API keys",
		})
		return
	}

	// Build response list, ensuring empty array not null
	items := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		items = append(items, apiKeyResponse{
			ID:         k.ID,
			Name:       k.Name,
			KeyPrefix:  k.KeyPrefix,
			Scopes:     k.Scopes,
			ExpiresAt:  k.ExpiresAt,
			CreatedAt:  k.CreatedAt,
			LastUsedAt: k.LastUsedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"keys": items,
	})
}

// Revoke handles DELETE /api/v1/api-keys/:id.
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	orgID, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if claims == nil {
		return // error already written
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "missing key id",
		})
		return
	}

	if err := h.apiKeySvc.Revoke(c.Request.Context(), id, orgID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "API key not found",
			})
			return
		}
		h.logger.Error("failed to revoke API key",
			"error", err,
			"key_id", id,
			"org_id", orgID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to revoke API key",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
