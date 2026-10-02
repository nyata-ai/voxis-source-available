package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

// RecordingHandler handles recording-related HTTP requests.
// All endpoints require JWT auth only (no API key auth).
type RecordingHandler struct {
	recordingSvc            *service.RecordingService
	orgService              *service.OrganizationService
	logger                  *slog.Logger
	allowPrivilegeRecording bool
}

// RecordingHandlerOption configures an optional recording API surface.
type RecordingHandlerOption func(*RecordingHandler)

// WithPrivilegeRecording controls whether the recording endpoint accepts the
// privilege mode. It defaults to true for existing deployments.
func WithPrivilegeRecording(enabled bool) RecordingHandlerOption {
	return func(h *RecordingHandler) { h.allowPrivilegeRecording = enabled }
}

// NewRecordingHandler creates a new RecordingHandler.
func NewRecordingHandler(
	recordingSvc *service.RecordingService,
	orgService *service.OrganizationService,
	logger *slog.Logger,
	opts ...RecordingHandlerOption,
) *RecordingHandler {
	if recordingSvc == nil {
		panic("recording handler: recording service cannot be nil")
	}
	if orgService == nil {
		panic("recording handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	handler := &RecordingHandler{
		recordingSvc:            recordingSvc,
		orgService:              orgService,
		logger:                  logger,
		allowPrivilegeRecording: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

// RecordingSessionResponse is the JSON response for a recording session.
type RecordingSessionResponse struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	MimeType        string  `json:"mime_type"`
	MicrophoneLabel string  `json:"microphone_label,omitempty"`
	MediaID         string  `json:"media_id,omitempty"`
	TotalDuration   float64 `json:"total_duration"`
	CaptureSource   string  `json:"capture_source"`
	ChunkCount      int     `json:"chunk_count,omitempty"`
	LastChunkAt     string  `json:"last_chunk_at,omitempty"`
	LastActivityAt  string  `json:"last_activity_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	CompletedAt     string  `json:"completed_at,omitempty"`
}

func toRecordingResponse(s *domain.RecordingSession) RecordingSessionResponse {
	resp := RecordingSessionResponse{
		ID:              s.ID,
		Status:          s.Status,
		MimeType:        s.MimeType,
		MicrophoneLabel: s.MicrophoneLabel,
		MediaID:         s.MediaID,
		TotalDuration:   s.TotalDuration,
		CaptureSource:   string(s.CaptureSource),
		ChunkCount:      s.ChunkCount,
		CreatedAt:       s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       s.UpdatedAt.Format(time.RFC3339),
	}
	if s.CompletedAt != nil {
		resp.CompletedAt = s.CompletedAt.Format(time.RFC3339)
	}
	if s.LastChunkAt != nil {
		resp.LastChunkAt = s.LastChunkAt.Format(time.RFC3339)
	}
	if s.LastActivityAt != nil {
		resp.LastActivityAt = s.LastActivityAt.Format(time.RFC3339)
	}
	return resp
}

type createRecordingRequest struct {
	MimeType        string `json:"mime_type" binding:"required"`
	MicrophoneLabel string `json:"microphone_label"`
	// RecordingMode is "regular" (default) or "privilege". Privilege mode
	// triggers the Privilege Recording lifecycle on stitch completion.
	RecordingMode string `json:"recording_mode,omitempty"`
	CaptureSource string `json:"capture_source,omitempty"`
}

// CreateSession handles POST /api/v1/recordings.
func (h *RecordingHandler) CreateSession(c *gin.Context) {
	orgID, claims := resolveOrgID(c, h.orgService, h.logger)
	if orgID == "" {
		return
	}

	var req createRecordingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid request body"})
		return
	}

	mode := domain.RecordingMode(req.RecordingMode)
	if mode == "" {
		mode = domain.RecordingModeRegular
	}
	if !mode.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid recording_mode"})
		return
	}
	if mode == domain.RecordingModePrivilege && !h.allowPrivilegeRecording {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "privilege recording is not available"})
		return
	}

	captureSource := domain.RecordingCaptureSource(req.CaptureSource)
	session, err := h.recordingSvc.CreateSessionWithModeAndCaptureSource(c.Request.Context(), orgID, claims.Subject, req.MimeType, req.MicrophoneLabel, mode, captureSource)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, toRecordingResponse(session))
}

// GetSession handles GET /api/v1/recordings/:id.
func (h *RecordingHandler) GetSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	session, err := h.recordingSvc.GetSession(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, toRecordingResponse(session))
}

// GetActiveSession handles GET /api/v1/recordings/active.
// Returns 404 when the caller has no session in recording or paused state.
func (h *RecordingHandler) GetActiveSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	session, err := h.recordingSvc.GetActiveSession(c.Request.Context(), claims.Subject)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, toRecordingResponse(session))
}

// ReleaseSession handles POST /api/v1/recordings/:id/release.
// Transitions the caller's active session to interrupted so the recovery flow
// can offer recover/discard immediately.
func (h *RecordingHandler) ReleaseSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.ReleaseSession(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": domain.RecordingStatusInterrupted})
}

// UploadChunk handles POST /api/v1/recordings/:id/chunks.
// Recording chunks have purpose-built validation (magic bytes, sequential ordering,
// MIME consistency, size limits) and are exempt from media upload guardrails
// (rate limiting, extension/MIME check, min size). The stitched output goes
// through MediaService.Upload which enqueues ClamAV scanning automatically.
func (h *RecordingHandler) UploadChunk(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	sessionID := c.Param("id")

	// Apply max bytes limit (25 MB + overhead).
	const maxChunkUpload = domain.MaxChunkSize + 1<<20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChunkUpload)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "file_too_large",
				"message": fmt.Sprintf("chunk exceeds maximum size of %d MB", domain.MaxChunkSize/(1024*1024)),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "file is required"})
		return
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // multipart file close

	// Parse sequence number.
	seqStr := c.PostForm("seq")
	seq, parseErr := strconv.Atoi(seqStr)
	if seqStr == "" || parseErr != nil || seq < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "seq is required and must be non-negative"})
		return
	}

	contentType := header.Header.Get("Content-Type")

	if err := h.recordingSvc.UploadChunk(c.Request.Context(), sessionID, claims.Subject, seq, file, header.Size, contentType); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "seq": seq})
}

// CompleteSession handles POST /api/v1/recordings/:id/complete.
func (h *RecordingHandler) CompleteSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.CompleteSession(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "completing"})
}

// PauseSession handles PATCH /api/v1/recordings/:id/pause.
func (h *RecordingHandler) PauseSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.PauseSession(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "paused"})
}

// ResumeSession handles PATCH /api/v1/recordings/:id/resume.
func (h *RecordingHandler) ResumeSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.ResumeSession(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "recording"})
}

// HeartbeatSession handles POST /api/v1/recordings/:id/heartbeat.
func (h *RecordingHandler) HeartbeatSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.Heartbeat(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListInterrupted handles GET /api/v1/recordings/interrupted.
func (h *RecordingHandler) ListInterrupted(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	sessions, err := h.recordingSvc.GetInterrupted(c.Request.Context(), claims.Subject)
	if err != nil {
		h.handleError(c, err)
		return
	}

	items := make([]RecordingSessionResponse, 0, len(sessions))
	for i := range sessions {
		items = append(items, toRecordingResponse(&sessions[i]))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RecoverSession handles POST /api/v1/recordings/:id/recover.
func (h *RecordingHandler) RecoverSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.RecoverSession(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "completing"})
}

// AbandonSession handles DELETE /api/v1/recordings/:id.
func (h *RecordingHandler) AbandonSession(c *gin.Context) {
	_, claims := resolveOrgID(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	if err := h.recordingSvc.AbandonSession(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// handleError maps domain errors to HTTP status codes.
func (h *RecordingHandler) handleError(c *gin.Context, err error) {
	var quotaErr *domain.StorageQuotaExceededError
	if errors.As(err, &quotaErr) {
		writeStorageQuotaExceeded(c, quotaErr)
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "recording not found"})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "access denied"})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": extractMessage(err)})
	case errors.Is(err, domain.ErrRateLimited):
		// Transient capacity limit (recording lock slots). The chunk outbox
		// retries 429 automatically; 409 would be treated as permanent.
		c.Header("Retry-After", "1")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited", "message": "server busy, please retry"})
	case errors.Is(err, domain.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": extractMessage(err)})
	default:
		h.logger.Error("recording handler error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
	}
}

func writeStorageQuotaExceeded(c *gin.Context, err *domain.StorageQuotaExceededError) {
	c.JSON(http.StatusConflict, gin.H{
		"error":           "storage_quota_exceeded",
		"message":         "storage quota exceeded",
		"requested_bytes": err.RequestedBytes,
		"used_bytes":      err.UsedBytes,
		"limit_bytes":     err.LimitBytes,
		"remaining_bytes": err.RemainingBytes,
	})
}

// extractMessage extracts the user-facing message from a wrapped error.
func extractMessage(err error) string {
	msg := err.Error()
	// Strip the sentinel error suffix.
	for _, sentinel := range []error{domain.ErrInvalidInput, domain.ErrConflict} {
		s := sentinel.Error()
		if i := strings.LastIndex(msg, ": "+s); i >= 0 {
			msg = msg[:i]
		}
	}
	// Strip outer service-layer prefix.
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
}
