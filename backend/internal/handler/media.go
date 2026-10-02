package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// MediaHandler handles media-related HTTP requests.
type MediaHandler struct {
	mediaSvc      *service.MediaService
	orgService    *service.OrganizationService
	prober        port.AudioProber
	streamTok     port.StreamTokenSigner
	streamTTL     time.Duration
	streamLimiter *middleware.RateLimiter
	uploadLimiter *middleware.RateLimiter
	uploadSpace   uploadSpoolReservation
	userUploads   *userUploadSlots
	logger        *slog.Logger
}

// SetMaxConcurrentUploads bounds simultaneous upload spools process-wide
// (UPLOAD_MAX_CONCURRENT). Values below 1 fall back to the default; the disk
// budget still applies underneath whatever cap is chosen.
func (h *MediaHandler) SetMaxConcurrentUploads(n int) {
	h.uploadSpace = newUploadSpoolBudget(n)
	h.logger.Info("upload concurrency configured", "max_concurrent", clampMaxConcurrentUploads(n), "requested", n)
}

// ConfigureLocalUploadSpool applies the validated local storage directory to
// the OSS upload budget, which reserves both spool and encrypted media space.
func (h *MediaHandler) ConfigureLocalUploadSpool(mediaDir string, n int) {
	h.uploadSpace = newLocalUploadSpoolBudget(n, os.TempDir(), mediaDir)
	h.logger.Info("local upload concurrency configured", "max_concurrent", clampMaxConcurrentUploads(n), "requested", n)
}

// NewMediaHandler creates a new MediaHandler.
func NewMediaHandler(
	mediaSvc *service.MediaService,
	orgService *service.OrganizationService,
	prober port.AudioProber,
	streamTok port.StreamTokenSigner,
	streamTTL time.Duration,
	streamLimiter *middleware.RateLimiter,
	uploadLimiter *middleware.RateLimiter,
	logger *slog.Logger,
) *MediaHandler {
	if mediaSvc == nil {
		panic("media handler: media service cannot be nil")
	}
	if orgService == nil {
		panic("media handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if streamTTL <= 0 {
		streamTTL = 5 * time.Minute
	}
	return &MediaHandler{
		mediaSvc:      mediaSvc,
		orgService:    orgService,
		prober:        prober,
		streamTok:     streamTok,
		streamTTL:     streamTTL,
		streamLimiter: streamLimiter,
		uploadLimiter: uploadLimiter,
		uploadSpace:   newUploadSpoolBudget(defaultMaxConcurrentUploads),
		userUploads:   newUserUploadSlots(),
		logger:        logger,
	}
}

// AudioAnalysisResponse is the API DTO for audio forensic data.
// Excludes FormatTags/StreamTags to prevent PII/device metadata leakage.
type AudioAnalysisResponse struct {
	RecordingDate   string           `json:"recording_date,omitempty"`
	RecordingSource string           `json:"recording_source,omitempty"`
	Encoder         string           `json:"encoder,omitempty"`
	TrustLevel      string           `json:"trust_level"`
	Findings        []domain.Finding `json:"findings"`
	FormatName      string           `json:"format_name,omitempty"`
	CodecName       string           `json:"codec_name,omitempty"`
	SampleRate      int              `json:"sample_rate,omitempty"`
	Channels        int              `json:"channels,omitempty"`
	Bitrate         int64            `json:"bitrate,omitempty"`
	Disclaimer      string           `json:"disclaimer"`
}

// MediaResponse is the JSON response for a single media item.
type MediaResponse struct {
	ID                    string                 `json:"id"`
	Title                 *string                `json:"title"`
	Description           *string                `json:"description"`
	Filename              string                 `json:"filename"`
	ContentType           string                 `json:"content_type"`
	Size                  int64                  `json:"size"`
	Duration              float64                `json:"duration"`
	Status                string                 `json:"status"`
	ScanStatus            string                 `json:"scan_status,omitempty"`
	FileHash              string                 `json:"file_hash,omitempty"`
	AudioAnalysis         *AudioAnalysisResponse `json:"audio_analysis"`
	EncryptionAlgo        string                 `json:"encryption_algo,omitempty"`
	LatestTranscriptionID *string                `json:"latest_transcription_id"`
	AudioDeletedAt        *string                `json:"audio_deleted_at,omitempty"`
	AudioAvailable        bool                   `json:"audio_available"`
	CreatedAt             string                 `json:"created_at"`
}

// ListMediaResponse is the JSON response for listing media items.
type ListMediaResponse struct {
	Items []MediaResponse `json:"items"`
	Total int64           `json:"total"`
}

// toMediaResponse converts a domain.Media to a MediaResponse.
func toMediaResponse(m *domain.Media) MediaResponse {
	resp := MediaResponse{
		ID:             m.ID,
		Filename:       m.Filename,
		ContentType:    m.ContentType,
		Size:           m.Size,
		Duration:       m.Duration,
		Status:         m.Status,
		ScanStatus:     m.ScanStatus,
		FileHash:       m.FileHash,
		EncryptionAlgo: m.EncryptionAlgo,
		AudioAvailable: m.Status == domain.MediaStatusReady && m.StorageKey != "" && m.AudioDeletedAt == nil,
		CreatedAt:      m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.AudioDeletedAt != nil {
		formatted := m.AudioDeletedAt.Format(time.RFC3339)
		resp.AudioDeletedAt = &formatted
	}
	if m.Title != "" {
		resp.Title = &m.Title
	}
	if m.Description != "" {
		resp.Description = &m.Description
	}
	if m.LatestTranscriptionID != "" {
		resp.LatestTranscriptionID = &m.LatestTranscriptionID
	}
	return resp
}

// toAudioAnalysisResponse converts domain AudioAnalysis to the API response DTO.
func toAudioAnalysisResponse(a *domain.AudioAnalysis) *AudioAnalysisResponse {
	if a == nil {
		return nil
	}
	findings := a.Findings
	if findings == nil {
		findings = []domain.Finding{}
	}
	return &AudioAnalysisResponse{
		RecordingDate:   a.RecordingDate,
		RecordingSource: a.RecordingSource,
		Encoder:         a.Encoder,
		TrustLevel:      a.TrustLevel,
		Findings:        findings,
		FormatName:      a.FormatName,
		CodecName:       a.CodecName,
		SampleRate:      a.SampleRate,
		Channels:        a.Channels,
		Bitrate:         a.Bitrate,
		Disclaimer:      a.Disclaimer,
	}
}

// resolveOrgID handles both JWT and API key auth paths.
func (h *MediaHandler) resolveOrgID(c *gin.Context) (string, *port.TokenClaims) {
	return resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
}

// handleError maps domain errors to HTTP status codes.
func (h *MediaHandler) handleError(c *gin.Context, err error) {
	var quotaErr *domain.StorageQuotaExceededError
	if errors.As(err, &quotaErr) {
		writeStorageQuotaExceeded(c, quotaErr)
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "media not found"})
	case errors.Is(err, domain.ErrAudioUnavailable):
		c.JSON(http.StatusGone, gin.H{"error": "audio_unavailable", "message": "audio has been deleted by retention policy"})
	case errors.Is(err, domain.ErrMediaTooLong):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "media_too_long",
			"message": strings.TrimSuffix(err.Error(), ": "+domain.ErrMediaTooLong.Error())})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "access denied"})
	case errors.Is(err, domain.ErrInvalidInput):
		// Extract user-facing message, stripping service-layer wrapping
		msg := err.Error()
		sentinel := domain.ErrInvalidInput.Error()
		if i := strings.LastIndex(msg, ": "+sentinel); i >= 0 {
			msg = msg[:i]
		}
		// Strip outer service-layer prefix (e.g., "create media: ...")
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": msg})
	default:
		h.logger.Error("media handler error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
	}
}

// spoolProbeAndLimit is spoolAndProbe plus the MEDIA_MAX_DURATION check, so
// over-long audio is refused before anything is encrypted or stored.
func (h *MediaHandler) spoolProbeAndLimit(c *gin.Context, file io.Reader, filename, contentType string, size int64) (*spoolResult, error) {
	spool, err := h.spoolAndProbe(c, file, filename, contentType, size)
	if err != nil {
		return nil, err
	}
	if limitErr := h.mediaSvc.CheckDuration(spool.duration); limitErr != nil {
		spool.cleanup()
		return nil, limitErr
	}
	return spool, nil
}

// spoolResult holds the result of spooling an upload to a temp file.
type spoolResult struct {
	src      io.Reader
	size     int64
	duration float64
	fileHash string
	analysis *domain.AudioAnalysis
	cleanup  func()
}

// spoolAndProbe writes the upload to a temp file (hashing via TeeReader),
// probes for audio metadata if available, and always runs forensic analysis.
// The caller must call cleanup().
func (h *MediaHandler) spoolAndProbe(c *gin.Context, file io.Reader, filename, contentType string, size int64) (*spoolResult, error) {
	tmpFile, tmpErr := os.CreateTemp("", "voxis-upload-*.tmp")
	if tmpErr != nil {
		return nil, fmt.Errorf("create temp file: %w", tmpErr)
	}
	tmpName := tmpFile.Name()
	rmTmp := func() { _ = os.Remove(tmpName) } //nolint:errcheck // best-effort cleanup

	// Hash via TeeReader during spool (single-pass I/O)
	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)
	written, copyErr := copyUploadSpool(tmpFile, tee)
	if copyErr != nil {
		rmTmp()
		var maxBytesErr *http.MaxBytesError
		if errors.As(copyErr, &maxBytesErr) {
			return nil, maxBytesErr
		}
		return nil, fmt.Errorf("buffer upload: %w", copyErr)
	}

	fileHash := hex.EncodeToString(hasher.Sum(nil))
	if written > 0 {
		size = written
	}

	// Validate audio file signature (magic bytes)
	sigBuf := make([]byte, domain.MinSignatureBytes)
	sigFile, sigOpenErr := os.Open(tmpName) //nolint:gosec // path is our own temp file
	if sigOpenErr != nil {
		rmTmp()
		return nil, fmt.Errorf("open temp file for signature check: %w", sigOpenErr)
	}
	n, sigReadErr := io.ReadFull(sigFile, sigBuf)
	_ = sigFile.Close() //nolint:errcheck // best-effort close
	if sigReadErr != nil && sigReadErr != io.ErrUnexpectedEOF {
		rmTmp()
		return nil, fmt.Errorf("read file signature: %w", sigReadErr)
	}
	if sigErr := domain.ValidateAudioSignature(contentType, sigBuf[:n]); sigErr != nil {
		rmTmp()
		return nil, sigErr
	}

	probeResult, probeErr := h.probeUpload(c.Request.Context(), tmpName)
	if probeErr != nil {
		rmTmp()
		return nil, probeErr
	}
	duration := probeResult.Duration

	analysis := h.mediaSvc.BuildAnalysis(probeResult, filepath.Ext(filename))

	reopened, openErr := os.Open(tmpName) //nolint:gosec // path is our own temp file
	if openErr != nil {
		rmTmp()
		return nil, fmt.Errorf("reopen temp file: %w", openErr)
	}

	return &spoolResult{
		src:      reopened,
		size:     size,
		duration: duration,
		fileHash: fileHash,
		analysis: analysis,
		cleanup: func() {
			_ = reopened.Close() //nolint:errcheck // best-effort cleanup
			rmTmp()
		},
	}, nil
}

// Upload probe failures that are not a verdict on the file itself.
var (
	errUploadProbeTimeout = errors.New("audio analysis timed out")
	errUploadProbeFailed  = errors.New("audio analysis failed")
)

// uploadProbeTimeout bounds ffprobe on one spooled upload.
const uploadProbeTimeout = 30 * time.Second

// probeUpload runs ffprobe on the spooled file. Every failure rejects the
// upload: a file that could not be analyzed must not reach storage and the
// transcription pipeline unvalidated.
func (h *MediaHandler) probeUpload(ctx context.Context, path string) (*port.AudioProbeResult, error) {
	if h.prober == nil || !h.prober.Available() {
		return nil, fmt.Errorf("%w: ffprobe is unavailable", errUploadProbeFailed)
	}
	probeCtx, cancel := context.WithTimeout(ctx, uploadProbeTimeout)
	defer cancel()
	result, err := h.prober.Probe(probeCtx, path)
	switch {
	case err == nil && result != nil:
		return result, nil
	case errors.Is(err, domain.ErrInvalidInput):
		return nil, fmt.Errorf("file does not contain a valid audio stream: %w", domain.ErrInvalidInput)
	case errors.Is(err, context.DeadlineExceeded):
		return nil, fmt.Errorf("%w: %w", errUploadProbeTimeout, err)
	case err == nil:
		return nil, fmt.Errorf("%w: no probe result", errUploadProbeFailed)
	default:
		return nil, fmt.Errorf("%w: %w", errUploadProbeFailed, err)
	}
}

// reserveUploadSlot takes one of the caller's per-user upload slots, then one
// process-wide spool slot, so a single user cannot hold every spool. It
// writes the 429/503 response itself and returns ok=false when either is
// unavailable; otherwise the caller must run release.
func (h *MediaHandler) reserveUploadSlot(c *gin.Context, subject string) (release func(), ok bool) {
	if !h.userUploads.acquire(subject) {
		c.Header("Retry-After", "15")
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "upload_limit",
			"message": fmt.Sprintf("You already have %d uploads in progress. Wait for one to finish, then try again.", maxConcurrentUploadsPerUser),
		})
		return nil, false
	}
	if err := h.uploadSpace.reserve(); err != nil {
		h.userUploads.release(subject)
		h.logger.Warn("upload scratch space unavailable", "error", err)
		// Slots free up as soon as a spool finishes, usually within seconds;
		// the client waits this long between bounded automatic retries.
		c.Header("Retry-After", "15")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable", "message": "Upload capacity is temporarily full. Please try again shortly."})
		return nil, false
	}
	return func() {
		h.uploadSpace.release()
		h.userUploads.release(subject)
	}, true
}

// Upload handles POST /api/v1/media/upload.
func (h *MediaHandler) Upload(c *gin.Context) {
	handlerStart := time.Now()

	orgID, claims := h.resolveOrgID(c)
	if orgID == "" || claims == nil || claims.Subject == "" {
		return
	}

	if h.uploadRateLimited(c) {
		return
	}
	release, ok := h.reserveUploadSlot(c, claims.Subject)
	if !ok {
		return
	}
	defer release()
	deadline := time.Now().Add(20 * time.Minute)
	if err := http.NewResponseController(c.Writer).SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.logger.Warn("set upload read deadline", "error", err)
	}

	const maxUploadSize = domain.MaxMediaSize + 1<<20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	file, err := nextUploadPart(c.Request)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "file_too_large",
				"message": fmt.Sprintf("Upload exceeds maximum size of %d MB", domain.MaxMediaSize/(1024*1024)),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "file is required"})
		return
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // multipart file close

	filename := sanitizeFilename(file.FileName())
	contentType := file.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		h.handleError(c, fmt.Errorf("content type is required: %w", domain.ErrInvalidInput))
		return
	}
	contentType = domain.NormalizeAudioContentType(contentType)

	if mimeErr := domain.ValidateExtensionMIME(filename, contentType); mimeErr != nil {
		h.handleError(c, mimeErr)
		return
	}

	var size int64 = -1

	h.logger.Info("upload handler entry", "content_type", contentType)
	spoolStart := time.Now()

	// Always spool to temp file for SHA-256 hashing and ffprobe forensic analysis.
	// Both require a seekable file: hashing needs full read, ffprobe needs file path.
	// The temp file is cleaned up immediately after the encrypted upload completes.
	spool, spoolErr := h.spoolProbeAndLimit(c, file, filename, contentType, size)
	if spoolErr != nil {
		h.respondSpoolError(c, spoolErr)
		return
	}
	defer spool.cleanup()

	h.logger.Info("spool and probe done", "size", spool.size, "content_type", contentType, "duration_ms", time.Since(spoolStart).Milliseconds())
	uploadStart := time.Now()

	media, err := h.mediaSvc.Upload(c.Request.Context(), orgID, claims.Subject, filename, contentType, spool.size, spool.src)
	if err != nil {
		h.handleError(c, err)
		return
	}

	h.logger.Info("media service upload done", "media_id", media.ID, "duration_ms", time.Since(uploadStart).Milliseconds())

	h.applySpoolMetadata(c.Request.Context(), orgID, media, spool)

	h.logger.Info("upload handler complete", "media_id", media.ID, "total_duration_ms", time.Since(handlerStart).Milliseconds())
	c.JSON(http.StatusCreated, toMediaResponse(media))
}

// uploadRateLimited applies the per-user upload rate limit. It reports true
// when the request was rejected (the 429 response has already been written).
func (h *MediaHandler) uploadRateLimited(c *gin.Context) bool {
	if h.uploadLimiter == nil {
		return false
	}
	// Key by authenticated user subject — upload requires auth, so subject
	// should always be present. No IP fallback (prevents bypass via IP rotation).
	rlClaims := middleware.GetClaims(c)
	if rlClaims == nil || rlClaims.Subject == "" {
		return false
	}
	if h.uploadLimiter.Allow(rlClaims.Subject) {
		return false
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate_limited",
		"message": "upload rate limit exceeded, try again later",
	})
	return true
}

// respondSpoolError maps spool/probe failures to their HTTP responses.
func (h *MediaHandler) respondSpoolError(c *gin.Context, spoolErr error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(spoolErr, &maxBytesErr) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error":   "file_too_large",
			"message": fmt.Sprintf("Upload exceeds maximum size of %d MB", domain.MaxMediaSize/(1024*1024)),
		})
		return
	}
	// Route domain validation errors (magic bytes, ffprobe) as 400, and the
	// duration limit as 422 media_too_long.
	if errors.Is(spoolErr, domain.ErrInvalidInput) || errors.Is(spoolErr, domain.ErrMediaTooLong) {
		h.handleError(c, spoolErr)
		return
	}
	if errors.Is(spoolErr, errUploadProbeTimeout) {
		h.logger.Warn("upload rejected: audio analysis timed out", "error", spoolErr)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "The audio file could not be checked in time. Try a shorter file or a common audio format."})
		return
	}
	h.logger.Error("failed to spool upload", "error", spoolErr)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to buffer upload"})
}

// applySpoolMetadata persists probed duration and forensic data after the
// encrypted upload completes. Failures are logged, never fatal.
func (h *MediaHandler) applySpoolMetadata(ctx context.Context, orgID string, media *domain.Media, spool *spoolResult) {
	if spool.duration > 0 {
		if durErr := h.mediaSvc.UpdateDuration(ctx, orgID, media.ID, spool.duration); durErr != nil {
			h.logger.Warn("failed to update media duration", "media_id", media.ID, "duration", spool.duration, "error", durErr)
		} else {
			media.Duration = spool.duration
		}
	}

	// Store forensic data (hash + analysis) -- non-blocking on failure
	if spool.fileHash != "" || spool.analysis != nil {
		if forErr := h.mediaSvc.StoreForensics(ctx, orgID, media.ID, spool.fileHash, spool.analysis); forErr != nil {
			h.logger.Warn("failed to store forensic data", "media_id", media.ID, "error", forErr)
		} else {
			media.FileHash = spool.fileHash
		}
	}
}

// sanitizeFilename strips path components and limits length for safe storage.
func sanitizeFilename(name string) string {
	// Strip path components -- filepath.Base handles OS-native separators,
	// but uploads from Windows clients may contain backslashes on a Linux server.
	// Replace backslashes with forward slashes before calling Base.
	name = strings.ReplaceAll(name, `\`, "/")
	name = filepath.Base(name)
	// Reject if Base returned "." (empty or only dots)
	if name == "." {
		name = "unnamed"
	}
	// Limit to 255 characters (filesystem-safe)
	if len(name) > 255 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ext[:20]
		}
		name = name[:255-len(ext)] + ext
	}
	return name
}

// List handles GET /api/v1/media.
func (h *MediaHandler) List(c *gin.Context) {
	orgID, _ := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	limit := parseIntQuery(c, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	offset := parseIntQuery(c, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	if err := service.ValidateSearchOffset(offset); err != nil {
		h.handleError(c, err)
		return
	}

	search, err := service.NormalizeSearchQuery(c.Query("search"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	status := c.Query("status")

	// Validate status filter against known values
	if status != "" {
		validStatuses := map[string]bool{
			domain.MediaStatusPending:    true,
			domain.MediaStatusEncrypting: true,
			domain.MediaStatusReady:      true,
			domain.MediaStatusFailed:     true,
		}
		if !validStatuses[status] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid status filter"})
			return
		}
	}

	var items []*domain.Media
	var total int64
	if search != "" || status != "" {
		items, total, err = h.mediaSvc.Search(c.Request.Context(), orgID, search, status, limit, offset)
		if err != nil {
			h.handleError(c, err)
			return
		}
	} else {
		items, total, err = h.mediaSvc.List(c.Request.Context(), orgID, limit, offset)
		if err != nil {
			h.handleError(c, err)
			return
		}
	}

	response := ListMediaResponse{
		Items: make([]MediaResponse, 0, len(items)),
		Total: total,
	}
	for _, m := range items {
		response.Items = append(response.Items, toMediaResponse(m))
	}

	c.JSON(http.StatusOK, response)
}

// GetByID handles GET /api/v1/media/:id.
func (h *MediaHandler) GetByID(c *gin.Context) {
	orgID, _ := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	media, err := h.mediaSvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	resp := toMediaResponse(media)

	// Decrypt and attach audio analysis if present (detail endpoint only)
	if len(media.AudioMetadata) > 0 {
		analysis, analysisErr := h.mediaSvc.GetAudioAnalysis(c.Request.Context(), orgID, media)
		if analysisErr != nil {
			h.logger.Warn("failed to decrypt audio analysis", "media_id", media.ID, "error", analysisErr)
		} else {
			resp.AudioAnalysis = toAudioAnalysisResponse(analysis)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// Delete handles DELETE /api/v1/media/:id.
func (h *MediaHandler) Delete(c *gin.Context) {
	orgID, _ := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	if err := h.mediaSvc.Delete(c.Request.Context(), orgID, c.Param("id")); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// updateMetadataRequest is the JSON request body for PATCH /api/v1/media/:id.
type updateMetadataRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

// UpdateMetadata handles PATCH /api/v1/media/:id.
func (h *MediaHandler) UpdateMetadata(c *gin.Context) {
	orgID, _ := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	var req updateMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid request body"})
		return
	}

	if req.Title == nil && req.Description == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "at least one field must be provided"})
		return
	}

	media, err := h.mediaSvc.UpdateMetadata(c.Request.Context(), orgID, c.Param("id"), req.Title, req.Description)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, toMediaResponse(media))
}

// StreamURL handles GET /api/v1/media/:id/stream-url.
// Requires auth; returns a short-lived URL for range-capable streaming.
func (h *MediaHandler) StreamURL(c *gin.Context) {
	orgID, claims := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	media, err := h.mediaSvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	// Quarantine: reject token generation for media pending or failing scan.
	switch media.ScanStatus {
	case domain.ScanStatusPending:
		c.JSON(http.StatusConflict, gin.H{"error": "scan_pending", "message": "file is being scanned for security"})
		return
	case domain.ScanStatusInfected, domain.ScanStatusError:
		c.JSON(http.StatusForbidden, gin.H{"error": "scan_failed", "message": "file failed security scan"})
		return
	}
	if media.Status != domain.MediaStatusReady {
		h.handleError(c, fmt.Errorf("media is not ready for streaming: %w", domain.ErrInvalidInput))
		return
	}

	if h.streamTok == nil {
		h.handleError(c, fmt.Errorf("stream token signer unavailable: %w", domain.ErrInvalidInput))
		return
	}

	exp := time.Now().Add(h.streamTTL)
	token, err := h.streamTok.Sign(port.StreamTokenPayload{
		MediaID: media.ID,
		OrgID:   orgID,
		Sub:     claims.Subject,
		Exp:     exp,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.Header("Cache-Control", "no-store, private")
	c.JSON(http.StatusOK, gin.H{
		"url":        fmt.Sprintf("/api/v1/media/%s/stream?token=%s", media.ID, token),
		"expires_at": exp.Format(time.RFC3339),
	})
}

// Stream handles GET /api/v1/media/:id/stream.
// Only accepts signed ?token= query params (issued by StreamURL).
func (h *MediaHandler) Stream(c *gin.Context) {
	payload, ok := h.authorizeStream(c)
	if !ok {
		return
	}

	h.logger.Info("stream token verified", "media_id", payload.MediaID, "sub", payload.Sub, "org_id", payload.OrgID)

	orgID := payload.OrgID
	// Single GetByID for scan check + range parsing (avoids duplicate DB queries).
	start, end, ok := h.precheckStreamRange(c, orgID)
	if !ok {
		return
	}

	media, reader, rng, streamErr := h.mediaSvc.Stream(c.Request.Context(), orgID, c.Param("id"), start, end)
	if streamErr != nil {
		h.handleError(c, streamErr)
		return
	}
	defer func() { _ = reader.Close() }() //nolint:errcheck // response already committed

	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", media.Filename))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", media.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", rng.Length))
	c.Header("Cache-Control", "no-store, private")

	if rng.Partial {
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rng.Start, rng.End, media.Size))
		c.Status(http.StatusPartialContent)
	} else {
		c.Status(http.StatusOK)
	}

	if _, err := io.Copy(c.Writer, reader); err != nil {
		h.logger.Warn("stream copy failed", "error", err)
	}
}

// authorizeStream verifies the signed stream token and applies the stream rate
// limit. It reports ok=false when the request was rejected (response written).
func (h *MediaHandler) authorizeStream(c *gin.Context) (*port.StreamTokenPayload, bool) {
	token := c.Query("token")
	if token == "" || h.streamTok == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "missing stream token"})
		return nil, false
	}
	payload, err := h.streamTok.Verify(token)
	if err != nil {
		h.logger.Warn("stream token verification failed",
			"reason", classifyStreamTokenError(err),
			"error", err,
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "invalid stream token"})
		return nil, false
	}
	if payload.MediaID != c.Param("id") {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "token does not match media"})
		return nil, false
	}

	if h.streamLimiter != nil {
		key := payload.Sub
		if key == "" {
			key = payload.OrgID
		}
		if key == "" {
			key = c.ClientIP()
		}
		if !h.streamLimiter.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limited",
				"message": "too many requests",
			})
			return nil, false
		}
	}

	return payload, true
}

// precheckStreamRange fetches the media once to quarantine unscanned/infected
// files and to parse the Range header against the real size. It reports
// ok=false when the request was rejected (response written).
func (h *MediaHandler) precheckStreamRange(c *gin.Context, orgID string) (start, end *int64, ok bool) {
	prefetch, getErr := h.mediaSvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if getErr != nil {
		h.handleError(c, getErr)
		return nil, nil, false
	}
	// Quarantine infected/pending media.
	switch prefetch.ScanStatus {
	case domain.ScanStatusPending:
		c.JSON(http.StatusConflict, gin.H{"error": "scan_pending", "message": "file is being scanned for security"})
		return nil, nil, false
	case domain.ScanStatusInfected, domain.ScanStatusError:
		c.JSON(http.StatusForbidden, gin.H{"error": "scan_failed", "message": "file failed security scan"})
		return nil, nil, false
	}
	// Parse range header using prefetched media size.
	if rangeHeader := c.GetHeader("Range"); rangeHeader != "" {
		var rangeErr error
		start, end, rangeErr = parseRangeHeader(rangeHeader, prefetch.Size)
		if rangeErr != nil {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", prefetch.Size))
			c.JSON(http.StatusRequestedRangeNotSatisfiable, gin.H{"error": "invalid_range"})
			return nil, nil, false
		}
	}
	return start, end, true
}

// Download handles GET /api/v1/media/:id/download.
// Requires auth; returns original audio as attachment in its original format.
func (h *MediaHandler) Download(c *gin.Context) {
	orgID, _ := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	media, err := h.mediaSvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Quarantine: block downloads while scan is pending or failed.
	switch media.ScanStatus {
	case domain.ScanStatusPending:
		c.JSON(http.StatusConflict, gin.H{"error": "scan_pending", "message": "file is being scanned for security"})
		return
	case domain.ScanStatusInfected, domain.ScanStatusError:
		c.JSON(http.StatusForbidden, gin.H{"error": "scan_failed", "message": "file failed security scan"})
		return
	}

	media, reader, rng, streamErr := h.mediaSvc.Stream(c.Request.Context(), orgID, c.Param("id"), nil, nil)
	if streamErr != nil {
		h.handleError(c, streamErr)
		return
	}
	defer func() { _ = reader.Close() }() //nolint:errcheck // response already committed

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", media.Filename))
	c.Header("Content-Type", media.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", rng.Length))
	c.Header("Cache-Control", "no-store, private")
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, reader); err != nil {
		h.logger.Warn("download copy failed", "error", err)
	}
}

func classifyStreamTokenError(err error) string {
	if err == nil {
		return "unknown"
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "expired"):
		return "expired"
	case strings.Contains(msg, "signature"):
		return "bad_signature"
	case strings.Contains(msg, "format"),
		strings.Contains(msg, "base64"),
		strings.Contains(msg, "unmarshal"),
		strings.Contains(msg, "json"):
		return "malformed"
	default:
		return "invalid"
	}
}

// parseRangeHeader parses an HTTP Range header value for byte ranges.
func parseRangeHeader(header string, size int64) (start, end *int64, err error) {
	if header == "" {
		return nil, nil, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return nil, nil, domain.ErrInvalidInput
	}

	parts := strings.Split(strings.TrimPrefix(header, "bytes="), "-")
	if len(parts) != 2 {
		return nil, nil, domain.ErrInvalidInput
	}

	// Suffix range: "-500"
	if parts[0] == "" {
		suffix, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil || suffix <= 0 {
			return nil, nil, domain.ErrInvalidInput
		}
		if suffix > size {
			suffix = size
		}
		startVal := size - suffix
		endVal := size - 1
		return &startVal, &endVal, nil
	}

	startVal, parseErr := strconv.ParseInt(parts[0], 10, 64)
	if parseErr != nil || startVal < 0 {
		return nil, nil, domain.ErrInvalidInput
	}

	// Open-ended range: "100-"
	if parts[1] == "" {
		if startVal >= size {
			return nil, nil, domain.ErrInvalidInput
		}
		endVal := size - 1
		return &startVal, &endVal, nil
	}

	endVal, parseErr := strconv.ParseInt(parts[1], 10, 64)
	if parseErr != nil || endVal < startVal || endVal >= size {
		return nil, nil, domain.ErrInvalidInput
	}

	return &startVal, &endVal, nil
}

// parseIntQuery reads an integer query parameter with a default value.
func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	s := c.Query(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
