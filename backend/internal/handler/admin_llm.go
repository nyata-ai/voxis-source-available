package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/service"
)

// AdminLLMHandler handles admin-managed LLM prompt and model settings.
type AdminLLMHandler struct {
	promptSvc *service.LLMPromptService
	logger    *slog.Logger
}

// NewAdminLLMHandler creates an AdminLLMHandler.
func NewAdminLLMHandler(promptSvc *service.LLMPromptService, logger *slog.Logger) *AdminLLMHandler {
	if promptSvc == nil {
		panic("handler: promptSvc must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminLLMHandler{promptSvc: promptSvc, logger: logger}
}

// updatePromptRequest retains the existing wire names for compatibility.
// system_instruction is admin presentation guidance; code owns the invariant.
type updatePromptRequest struct {
	SystemInstruction string `json:"system_instruction"`
	UserPrompt        string `json:"user_prompt"`
	Model             string `json:"model"`
}

// ListPrompts handles GET /api/v1/admin/llm/prompts.
func (h *AdminLLMHandler) ListPrompts(c *gin.Context) {
	prompts, err := h.promptSvc.List(c.Request.Context())
	if err != nil {
		h.writePromptError(c, err, "failed to retrieve LLM prompts")
		return
	}
	c.JSON(http.StatusOK, gin.H{"prompts": prompts})
}

// UpdatePrompt handles PUT /api/v1/admin/llm/prompts/:key.
func (h *AdminLLMHandler) UpdatePrompt(c *gin.Context) {
	var req updatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "invalid prompt payload"})
		return
	}

	updatedBy := ""
	if claims := middleware.GetClaims(c); claims != nil {
		updatedBy = claims.Subject
	}
	prompt, err := h.promptSvc.Update(c.Request.Context(), c.Param("key"), service.LLMPromptUpdate{
		SystemInstruction: req.SystemInstruction,
		UserPrompt:        req.UserPrompt,
		Model:             req.Model,
	}, updatedBy)
	if err != nil {
		h.writePromptError(c, err, "failed to update LLM prompt")
		return
	}
	c.JSON(http.StatusOK, prompt)
}

// ResetPrompt handles DELETE /api/v1/admin/llm/prompts/:key.
func (h *AdminLLMHandler) ResetPrompt(c *gin.Context) {
	prompt, err := h.promptSvc.Reset(c.Request.Context(), c.Param("key"))
	if err != nil {
		h.writePromptError(c, err, "failed to reset LLM prompt")
		return
	}
	c.JSON(http.StatusOK, prompt)
}

func (h *AdminLLMHandler) writePromptError(c *gin.Context, err error, message string) {
	if errors.Is(err, domain.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": message})
		return
	}
	h.logger.Error(message, "error", err, "request_id", c.GetString("request_id"))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": message})
}
