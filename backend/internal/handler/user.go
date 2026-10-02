package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/service"
)

// UserHandler handles user-related HTTP requests.
type UserHandler struct {
	orgService *service.OrganizationService
	logger     *slog.Logger
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(orgService *service.OrganizationService, logger *slog.Logger) *UserHandler {
	if orgService == nil {
		panic("user handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &UserHandler{orgService: orgService, logger: logger}
}

// MeResponse represents the response for GET /me endpoint.
type MeResponse struct {
	ID           string             `json:"id"`
	Email        string             `json:"email"`
	Name         string             `json:"name"`
	Organization *OrganizationBrief `json:"organization,omitempty"`
}

// OrganizationBrief is a brief organization info for user response.
type OrganizationBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Tier string `json:"tier"`
}

// Me returns the current authenticated user's profile.
// Creates the user and organization on first login.
func (h *UserHandler) Me(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "no authentication claims found",
		})
		return
	}

	// Validate required claims
	if claims.Subject == "" {
		h.logger.Error("missing subject in claims", "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_token",
			"message": "missing subject claim",
		})
		return
	}

	if claims.Email == "" {
		h.logger.Error("missing email in claims",
			"subject", claims.Subject,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_token",
			"message": "missing email claim",
		})
		return
	}

	// CreateUserWithOrg handles both first login and subsequent logins.
	// Returns user and org together to avoid redundant DB lookups.
	user, org, err := h.orgService.CreateUserWithOrg(c.Request.Context(), claims)
	if err != nil {
		h.logger.Error("failed to get or create user",
			"error", err,
			"subject", claims.Subject,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to retrieve user",
		})
		return
	}

	response := MeResponse{
		ID:    user.ID,
		Email: user.Email,
		Name:  user.Name,
	}

	if org != nil {
		response.Organization = &OrganizationBrief{
			ID:   org.ID,
			Name: org.Name,
			Tier: org.Tier,
		}
	}

	c.JSON(http.StatusOK, response)
}
