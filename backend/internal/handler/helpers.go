package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// resolveOrgID extracts claims and resolves the organization ID for the authenticated user.
// Returns the org ID and claims, or writes an error response and returns empty string.
func resolveOrgID(c *gin.Context, orgService *service.OrganizationService, logger *slog.Logger) (string, *port.TokenClaims) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "no authentication claims found",
		})
		return "", nil
	}

	if claims.Subject == "" {
		logger.Error("missing subject in claims", "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_token",
			"message": "missing subject claim",
		})
		return "", nil
	}

	if claims.Email == "" {
		logger.Error("missing email in claims",
			"subject", claims.Subject,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_token",
			"message": "missing email claim",
		})
		return "", nil
	}

	_, org, err := orgService.CreateUserWithOrg(c.Request.Context(), claims)
	if err != nil {
		logger.Error("failed to resolve organization",
			"error", err,
			"subject", claims.Subject,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to resolve user organization",
		})
		return "", nil
	}

	if org == nil {
		logger.Error("user has no organization",
			"subject", claims.Subject,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "user has no organization",
		})
		return "", nil
	}

	return org.ID, claims
}

// resolveOrgIDOrAPIKey handles both JWT and API key auth.
// JWT: delegates to resolveOrgID (validates email, creates user/org).
// API key: reads org_id from Gin context (set by auth middleware) — skips
// user creation since the API key already belongs to an org.
func resolveOrgIDOrAPIKey(c *gin.Context, orgService *service.OrganizationService, logger *slog.Logger) (string, *port.TokenClaims) {
	authMethod, _ := c.Get("auth_method")
	if authMethod == "api_key" {
		orgIDVal, exists := c.Get("org_id")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "org_id not set for API key auth",
			})
			return "", nil
		}
		orgID, ok := orgIDVal.(string)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "org_id not set for API key auth",
			})
			return "", nil
		}
		subjectVal, _ := c.Get("subject")
		subject, ok2 := subjectVal.(string)
		if !ok2 {
			subject = ""
		}
		return orgID, &port.TokenClaims{Subject: subject}
	}
	return resolveOrgID(c, orgService, logger)
}
