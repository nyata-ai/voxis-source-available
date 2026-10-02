package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/handler/middleware"
)

// AdminAuthHandler handles admin authentication introspection.
type AdminAuthHandler struct{}

// NewAdminAuthHandler creates an AdminAuthHandler.
func NewAdminAuthHandler() *AdminAuthHandler {
	return &AdminAuthHandler{}
}

// AdminMeResponse is the safe response for GET /api/v1/admin/me.
type AdminMeResponse struct {
	Admin bool `json:"admin"`
}

// Me returns safe admin claim fields without user or organization content.
func (h *AdminAuthHandler) Me(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "no authentication claims found",
		})
		return
	}
	if !claims.HasRole(middleware.AdminRole) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "insufficient permissions",
		})
		return
	}

	c.JSON(http.StatusOK, AdminMeResponse{
		Admin: true,
	})
}
