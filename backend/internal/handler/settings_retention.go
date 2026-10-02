package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/service"
)

type recordingRetentionRequest struct {
	Enabled                       bool   `json:"enabled"`
	Days                          int    `json:"days"`
	ApplyToExisting               bool   `json:"apply_to_existing"`
	ConfirmedImmediateDeleteCount *int64 `json:"confirmed_immediate_delete_count,omitempty"`
}

type recordingRetentionPolicyResponse struct {
	Enabled         bool    `json:"enabled"`
	Days            int     `json:"days"`
	EffectiveAt     *string `json:"effective_at,omitempty"`
	ApplyToExisting bool    `json:"apply_to_existing"`
	UpdatedAt       *string `json:"updated_at,omitempty"`
	UpdatedBy       string  `json:"updated_by,omitempty"`
}

type recordingRetentionPreviewResponse struct {
	ImmediateDeleteCount   int64   `json:"immediate_delete_count"`
	OldestCompletedAt      *string `json:"oldest_completed_at,omitempty"`
	ProspectiveEffectiveAt *string `json:"prospective_effective_at,omitempty"`
}

// AdminRecordingRetentionHandler handles platform-wide recording retention settings.
type AdminRecordingRetentionHandler struct {
	retentionSvc *service.RecordingRetentionPolicyService
	logger       *slog.Logger
}

// NewAdminRecordingRetentionHandler creates an AdminRecordingRetentionHandler.
func NewAdminRecordingRetentionHandler(retentionSvc *service.RecordingRetentionPolicyService, logger *slog.Logger) *AdminRecordingRetentionHandler {
	if retentionSvc == nil {
		panic("handler: retentionSvc must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminRecordingRetentionHandler{retentionSvc: retentionSvc, logger: logger}
}

// Get returns the platform-wide live recording audio retention policy.
func (h *AdminRecordingRetentionHandler) Get(c *gin.Context) {
	policy, err := h.retentionSvc.Get(c.Request.Context())
	if err != nil {
		h.handleRetentionError(c, err)
		return
	}
	c.JSON(http.StatusOK, toRecordingRetentionPolicyResponse(policy))
}

// Preview reports the immediate-delete impact of a proposed platform-wide policy.
func (h *AdminRecordingRetentionHandler) Preview(c *gin.Context) {
	policy, ok := h.bindRecordingRetentionPolicy(c)
	if !ok {
		return
	}
	prepared, err := h.retentionSvc.Prepare(c.Request.Context(), policy)
	if err != nil {
		h.handleRetentionError(c, err)
		return
	}
	preview, err := h.retentionSvc.Preview(c.Request.Context(), prepared)
	if err != nil {
		h.handleRetentionError(c, err)
		return
	}
	c.JSON(http.StatusOK, toRecordingRetentionPreviewResponse(preview))
}

// Update stores the platform-wide live recording audio retention policy.
func (h *AdminRecordingRetentionHandler) Update(c *gin.Context) {
	policy, ok := h.bindRecordingRetentionPolicy(c)
	if !ok {
		return
	}
	prepared, err := h.retentionSvc.Prepare(c.Request.Context(), policy)
	if err != nil {
		h.handleRetentionError(c, err)
		return
	}
	preview, err := h.retentionSvc.Preview(c.Request.Context(), prepared)
	if err != nil {
		h.handleRetentionError(c, err)
		return
	}
	if policy.ApplyToExisting || preview.ImmediateDeleteCount > 0 {
		confirmed := recordingRetentionConfirmedCount(c)
		if confirmed == nil || *confirmed != preview.ImmediateDeleteCount {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "retention_confirmation_required",
				"message": "recording retention confirmation count is missing or stale",
				"preview": toRecordingRetentionPreviewResponse(preview),
			})
			return
		}
	}
	updatedBy := ""
	if claims := middleware.GetClaims(c); claims != nil {
		updatedBy = claims.Subject
	}
	updated, err := h.retentionSvc.Update(c.Request.Context(), prepared, updatedBy)
	if err != nil {
		h.handleRetentionError(c, err)
		return
	}
	c.JSON(http.StatusOK, toRecordingRetentionPolicyResponse(updated))
}

func (h *AdminRecordingRetentionHandler) bindRecordingRetentionPolicy(c *gin.Context) (domain.LiveRecordingRetentionPolicy, bool) {
	var req recordingRetentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid request body"})
		return domain.LiveRecordingRetentionPolicy{}, false
	}
	c.Set("recording_retention_confirmed_count", req.ConfirmedImmediateDeleteCount)
	return domain.LiveRecordingRetentionPolicy{
		Enabled:         req.Enabled,
		Days:            req.Days,
		ApplyToExisting: req.ApplyToExisting,
	}, true
}

func recordingRetentionConfirmedCount(c *gin.Context) *int64 {
	value, ok := c.Get("recording_retention_confirmed_count")
	if !ok {
		return nil
	}
	confirmed, ok := value.(*int64)
	if !ok {
		return nil
	}
	return confirmed
}

func (h *AdminRecordingRetentionHandler) handleRetentionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "recording retention policy not found"})
	default:
		h.logger.Error("recording retention admin error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to update recording retention policy"})
	}
}

func toRecordingRetentionPolicyResponse(policy domain.LiveRecordingRetentionPolicy) recordingRetentionPolicyResponse {
	return recordingRetentionPolicyResponse{
		Enabled:         policy.Enabled,
		Days:            policy.Days,
		EffectiveAt:     formatTimePtr(policy.EffectiveAt),
		ApplyToExisting: policy.ApplyToExisting,
		UpdatedAt:       formatTimePtr(policy.UpdatedAt),
		UpdatedBy:       policy.UpdatedBy,
	}
}

func toRecordingRetentionPreviewResponse(preview domain.LiveRecordingRetentionPreview) recordingRetentionPreviewResponse {
	return recordingRetentionPreviewResponse{
		ImmediateDeleteCount:   preview.ImmediateDeleteCount,
		OldestCompletedAt:      formatTimePtr(preview.OldestCompletedAt),
		ProspectiveEffectiveAt: formatTimePtr(preview.ProspectiveEffectiveAt),
	}
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format(time.RFC3339)
	return &formatted
}
