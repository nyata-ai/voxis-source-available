package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// summaryFanOutTypes is the number of summary types GenerateAll produces. It is
// derived from the canonical domain list so the billable token charge tracks the
// real fan-out (and stays ≤ the limiter burst) if a type is ever added.
var summaryFanOutTypes = len(domain.DefaultSummaryTypes)

// SummaryHandler handles summary-related HTTP requests.
type SummaryHandler struct {
	summarySvc             *service.SummaryService
	transSvc               *service.TranscriptionService
	orgService             *service.OrganizationService
	userRepo               port.UserRepository
	jobInserter            port.SummaryJobInserter // nil when Vertex AI not configured
	billLimiter            *middleware.RateLimiter
	summaryProfilesEnabled bool
	logger                 *slog.Logger
}

// NewSummaryHandler creates a new SummaryHandler.
// jobInserter may be nil if AI summarization is unavailable (e.g., no Vertex AI configured).
func NewSummaryHandler(
	summarySvc *service.SummaryService,
	transSvc *service.TranscriptionService,
	orgService *service.OrganizationService,
	userRepo port.UserRepository,
	jobInserter port.SummaryJobInserter,
	logger *slog.Logger,
) *SummaryHandler {
	if summarySvc == nil {
		panic("summary handler: summary service cannot be nil")
	}
	if transSvc == nil {
		panic("summary handler: transcription service cannot be nil")
	}
	if orgService == nil {
		panic("summary handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SummaryHandler{
		summarySvc:  summarySvc,
		transSvc:    transSvc,
		orgService:  orgService,
		userRepo:    userRepo,
		jobInserter: jobInserter,
		logger:      logger,
	}
}

// SetBillableRateLimiter wires a limiter for AI-backed summary operations.
func (h *SummaryHandler) SetBillableRateLimiter(limiter *middleware.RateLimiter) {
	h.billLimiter = limiter
}

// SetSummaryProfilesEnabled controls whether new summary rows use the
// initiating user's selected professional profile.
func (h *SummaryHandler) SetSummaryProfilesEnabled(enabled bool) {
	h.summaryProfilesEnabled = enabled
}

// CreateSummaryRequest is the JSON request body for creating a summary.
type CreateSummaryRequest struct {
	TranscriptionID string  `json:"transcription_id" binding:"required"`
	SummaryType     string  `json:"summary_type" binding:"required"`
	HighStakes      *bool   `json:"high_stakes,omitempty"`
	SummaryProfile  *string `json:"summary_profile,omitempty"`
}

// RegenerateSummaryRequest is the JSON request body for regenerating a summary.
type RegenerateSummaryRequest struct {
	HighStakes     *bool   `json:"high_stakes,omitempty"`
	SummaryProfile *string `json:"summary_profile,omitempty"`
}

// GenerateAllSummariesRequest is the optional JSON request body for generate-all.
// The endpoint stays callable with no body at all.
type GenerateAllSummariesRequest struct {
	SummaryProfile *string `json:"summary_profile,omitempty"`
}

// SummaryResponse is the JSON response for a single summary (list view).
type SummaryResponse struct {
	ID                            string                             `json:"id"`
	OrganizationID                string                             `json:"organization_id"`
	TranscriptionID               string                             `json:"transcription_id"`
	TranscriptionMediaFilename    string                             `json:"transcription_media_filename,omitempty"`
	TranscriptionAudioName        string                             `json:"transcription_audio_name,omitempty"`
	TranscriptionMediaDescription string                             `json:"transcription_media_description,omitempty"`
	SummaryType                   string                             `json:"summary_type"`
	SummaryProfile                string                             `json:"summary_profile"`
	Status                        string                             `json:"status"`
	WordCount                     int                                `json:"word_count"`
	PromptTokens                  int                                `json:"prompt_tokens"`
	CompletionTokens              int                                `json:"completion_tokens"`
	HighStakes                    bool                               `json:"high_stakes"`
	ReviewStatus                  string                             `json:"review_status"`
	ErrorMessage                  string                             `json:"error_message,omitempty"`
	CreatedAt                     string                             `json:"created_at"`
	CompletedAt                   string                             `json:"completed_at,omitempty"`
	StructuredContent             json.RawMessage                    `json:"structured_content,omitempty"`
	Citations                     []SummaryCitationResponse          `json:"citations,omitempty"`
	GenerationMetadata            *SummaryGenerationMetadataResponse `json:"generation_metadata"`
}

// SummaryCitationResponse is the compact source location for a structured item.
type SummaryCitationResponse struct {
	ID           string   `json:"id"`
	Speaker      *string  `json:"speaker,omitempty"`
	StartSeconds *float64 `json:"start_seconds,omitempty"`
	EndSeconds   *float64 `json:"end_seconds,omitempty"`
}

// SummaryGenerationMetadataResponse records structured-summary provenance.
type SummaryGenerationMetadataResponse struct {
	PromptVersion           string   `json:"prompt_version"`
	Model                   string   `json:"model"`
	EndpointLocation        string   `json:"endpoint_location"`
	SourceVersion           string   `json:"source_version"`
	SourceHash              string   `json:"source_hash"`
	StructuredSchemaVersion string   `json:"structured_schema_version"`
	DegradationCodes        []string `json:"degradation_codes"`
}

// SummaryDetailResponse extends SummaryResponse with decrypted content.
type SummaryDetailResponse struct {
	SummaryResponse
	Content string `json:"content,omitempty"`
}

// ListSummaryResponse is the JSON response for listing summaries.
type ListSummaryResponse struct {
	Items []SummaryResponse `json:"items"`
	Total int64             `json:"total"`
}

// toSummaryResponse converts a domain.Summary to a SummaryResponse.
func toSummaryResponse(s *domain.Summary) SummaryResponse {
	resp := SummaryResponse{
		ID:                            s.ID,
		OrganizationID:                s.OrganizationID,
		TranscriptionID:               s.TranscriptionID,
		TranscriptionMediaFilename:    s.TranscriptionMediaFilename,
		TranscriptionAudioName:        s.TranscriptionAudioName,
		TranscriptionMediaDescription: s.TranscriptionMediaDescription,
		SummaryType:                   s.SummaryType,
		SummaryProfile:                domain.NormalizeSummaryProfile(s.SummaryProfile),
		Status:                        s.Status,
		WordCount:                     s.WordCount,
		PromptTokens:                  s.PromptTokens,
		CompletionTokens:              s.CompletionTokens,
		HighStakes:                    s.HighStakes,
		ReviewStatus:                  s.ReviewStatus,
		ErrorMessage:                  s.ErrorMessage,
		CreatedAt:                     s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if s.CompletedAt != nil {
		resp.CompletedAt = s.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	resp.GenerationMetadata = summaryGenerationMetadataResponse(service.BuildSummaryStructuredPresentation(s, nil).GenerationMetadata)
	return resp
}

// toSummaryDetailResponse converts a domain.Summary to a SummaryDetailResponse.
func toSummaryDetailResponse(s *domain.Summary) SummaryDetailResponse {
	return SummaryDetailResponse{
		SummaryResponse: toSummaryResponse(s),
		Content:         s.Content,
	}
}

func (h *SummaryHandler) hydrateSummaryStructuredResponse(
	ctx context.Context,
	orgID string,
	summary *domain.Summary,
	response *SummaryResponse,
	transcriptions map[string]*domain.Transcription,
) {
	if len(summary.StructuredContent) == 0 {
		return
	}
	transcription, found := transcriptions[summary.TranscriptionID]
	if !found {
		var err error
		transcription, err = h.transSvc.GetByID(ctx, orgID, summary.TranscriptionID)
		if err != nil {
			return
		}
		if transcriptions != nil {
			transcriptions[summary.TranscriptionID] = transcription
		}
	}
	applySummaryStructuredPresentation(response, service.BuildSummaryStructuredPresentation(summary, transcription))
}

func applySummaryStructuredPresentation(response *SummaryResponse, presentation service.SummaryStructuredPresentation) {
	response.StructuredContent = presentation.StructuredContent
	response.GenerationMetadata = summaryGenerationMetadataResponse(presentation.GenerationMetadata)
	if len(presentation.Citations) == 0 {
		response.Citations = nil
		return
	}
	response.Citations = make([]SummaryCitationResponse, len(presentation.Citations))
	for i, citation := range presentation.Citations {
		response.Citations[i] = SummaryCitationResponse{
			ID:           citation.ID,
			Speaker:      citation.Speaker,
			StartSeconds: citation.StartSeconds,
			EndSeconds:   citation.EndSeconds,
		}
	}
}

func summaryGenerationMetadataResponse(metadata *service.SummaryGenerationMetadata) *SummaryGenerationMetadataResponse {
	if metadata == nil {
		return nil
	}
	return &SummaryGenerationMetadataResponse{
		PromptVersion:           metadata.PromptVersion,
		Model:                   metadata.Model,
		EndpointLocation:        metadata.EndpointLocation,
		SourceVersion:           metadata.SourceVersion,
		SourceHash:              metadata.SourceHash,
		StructuredSchemaVersion: metadata.StructuredSchemaVersion,
		DegradationCodes:        append([]string{}, metadata.DegradationCodes...),
	}
}

// resolveOrgID handles both JWT and API key auth paths.
func (h *SummaryHandler) resolveOrgID(c *gin.Context) string {
	orgID, _ := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	return orgID
}

func (h *SummaryHandler) resolveOrgAndSubject(c *gin.Context) (orgID, subject string) {
	orgID, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if claims == nil {
		return "", ""
	}
	return orgID, claims.Subject
}

// handleError maps domain errors to HTTP status codes.
func (h *SummaryHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "summary not found"})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "access denied"})
	case errors.Is(err, domain.ErrPrivilegeOnly):
		// iter 3 fix: privilege media require POST /privilege/:mediaId/summarize.
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "privilege_only",
			"message": "this media requires the privilege workspace to summarize",
		})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "an active summary of this type already exists"})
	case errors.Is(err, domain.ErrInvalidInput):
		msg := err.Error()
		sentinel := domain.ErrInvalidInput.Error()
		if i := strings.LastIndex(msg, ": "+sentinel); i >= 0 {
			msg = msg[:i]
		}
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": msg})
	default:
		h.logger.Error("summary handler error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
	}
}

// allowBillable charges n tokens to the caller's billable budget and reports
// whether the request may proceed. n matches the number of AI jobs the request
// fans out to (1 for a single summary, summaryFanOutTypes for generate-all), so
// both paths cost the same per enqueued job.
func (h *SummaryHandler) allowBillable(c *gin.Context, subject string, n int) bool {
	if h.billLimiter == nil {
		return true
	}
	key := c.GetString("api_key_id")
	if key == "" {
		key = subject
	}
	if key == "" {
		key = c.ClientIP()
	}
	if h.billLimiter.AllowN(key, n) {
		return true
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate_limited",
		"message": "summary rate limit exceeded, try again later",
	})
	return false
}

// Create handles POST /api/v1/summaries.
func (h *SummaryHandler) Create(c *gin.Context) {
	orgID, subject := h.resolveOrgAndSubject(c)
	if orgID == "" {
		return
	}

	var req CreateSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "transcription_id and summary_type are required"})
		return
	}

	// Reject early if AI summarization is unavailable.
	if h.jobInserter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service_unavailable", "message": "AI summarization is not configured"})
		return
	}
	// Validate the requested profile before charging: this check is purely
	// syntactic, so a request that can only 400 must not spend a token.
	if err := h.checkCreateSummaryProfile(req.SummaryProfile); err != nil {
		h.handleError(c, err)
		return
	}
	if !h.allowBillable(c, subject, 1) {
		return
	}

	highStakes, err := h.resolveHighStakes(c, subject, req.HighStakes)
	if err != nil {
		h.handleError(c, err)
		return
	}

	summaryProfile, err := h.resolveCreateSummaryProfile(c, subject, req.SummaryProfile)
	if err != nil {
		h.handleError(c, err)
		return
	}
	summary, err := h.summarySvc.CreateWithProfile(c.Request.Context(), orgID, req.TranscriptionID, req.SummaryType, highStakes, summaryProfile)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Enqueue background summarization job.
	if insertErr := h.jobInserter.InsertSummarizeJob(c.Request.Context(), summary.ID, req.TranscriptionID, orgID); insertErr != nil {
		h.logger.Error("failed to enqueue summarization job", "error", insertErr, "summary_id", summary.ID)
		// Clean up the orphaned summary to avoid blocking future retries.
		if delErr := h.summarySvc.Delete(c.Request.Context(), orgID, summary.ID); delErr != nil {
			h.logger.Error("failed to clean up orphaned summary", "error", delErr, "summary_id", summary.ID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to start summarization processing"})
		return
	}

	c.JSON(http.StatusAccepted, toSummaryResponse(summary))
}

// List handles GET /api/v1/summaries.
func (h *SummaryHandler) List(c *gin.Context) {
	orgID := h.resolveOrgID(c)
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

	items, total, err := h.summarySvc.List(c.Request.Context(), orgID, limit, offset, search)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response := ListSummaryResponse{
		Items: make([]SummaryResponse, 0, len(items)),
		Total: total,
	}
	for _, s := range items {
		response.Items = append(response.Items, toSummaryResponse(s))
	}

	c.JSON(http.StatusOK, response)
}

// GetByID handles GET /api/v1/summaries/:id.
func (h *SummaryHandler) GetByID(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	summary, err := h.summarySvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	response := toSummaryDetailResponse(summary)
	h.hydrateSummaryStructuredResponse(c.Request.Context(), orgID, summary, &response.SummaryResponse, nil)
	c.JSON(http.StatusOK, response)
}

// Delete handles DELETE /api/v1/summaries/:id.
func (h *SummaryHandler) Delete(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	if err := h.summarySvc.Delete(c.Request.Context(), orgID, c.Param("id")); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// Regenerate handles POST /api/v1/summaries/:id/regenerate.
// Deletes the old summary and creates a new pending one of the same type,
// then enqueues a new summarization job.
func (h *SummaryHandler) Regenerate(c *gin.Context) {
	orgID, subject := h.resolveOrgAndSubject(c)
	if orgID == "" {
		return
	}

	// Reject early if AI summarization is unavailable.
	if h.jobInserter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service_unavailable", "message": "AI summarization is not configured"})
		return
	}
	if !h.allowBillable(c, subject, 1) {
		return
	}

	summaryID := c.Param("id")
	var req RegenerateSummaryRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid JSON body"})
			return
		}
	}

	summaryProfile, err := h.resolveRegenerateSummaryProfile(c, subject, req.SummaryProfile)
	if err != nil {
		h.handleError(c, err)
		return
	}
	result, err := h.summarySvc.RegenerateWithProfile(c.Request.Context(), orgID, summaryID, req.HighStakes, summaryProfile)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Enqueue background summarization job for the new summary.
	if insertErr := h.jobInserter.InsertSummarizeJob(c.Request.Context(), result.NewSummary.ID, result.NewSummary.TranscriptionID, orgID); insertErr != nil {
		h.logger.Error("failed to enqueue regeneration job", "error", insertErr, "summary_id", result.NewSummary.ID)
		// Roll back: delete the orphaned new summary and restore the old one.
		if delErr := h.summarySvc.Delete(c.Request.Context(), orgID, result.NewSummary.ID); delErr != nil {
			h.logger.Error("failed to clean up orphaned summary after regeneration", "error", delErr, "summary_id", result.NewSummary.ID)
		}
		if undoErr := h.summarySvc.UndoDelete(c.Request.Context(), orgID, result.OldSummaryID, result.OldStatus); undoErr != nil {
			h.logger.Error("failed to restore old summary after regeneration rollback", "error", undoErr, "summary_id", result.OldSummaryID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to start summarization processing"})
		return
	}

	c.JSON(http.StatusAccepted, toSummaryResponse(result.NewSummary))
}

func (h *SummaryHandler) resolveHighStakes(c *gin.Context, subject string, override *bool) (bool, error) {
	if override != nil {
		return *override, nil
	}
	if !h.summarySvc.HighStakesEnabled() {
		return false, nil
	}
	return service.ResolveHighStakesPreference(c.Request.Context(), h.userRepo, subject)
}

func (h *SummaryHandler) resolveSummaryProfile(c *gin.Context, subject string) (string, error) {
	return service.ResolveSummaryProfile(c.Request.Context(), h.userRepo, subject, h.summaryProfilesEnabled)
}

// summaryProfileOverride normalizes a caller-supplied summary_profile and
// reports whether one was actually sent. A missing field, an explicit null and
// a blank string all count as absent, so `{"summary_profile": ""}` resolves the
// caller's saved preference instead of failing validation. This matches the MCP
// tool surface, which treats an empty string as unset.
func summaryProfileOverride(override *string) (string, bool) {
	if override == nil {
		return "", false
	}
	requested := strings.TrimSpace(*override)
	if requested == "" {
		return "", false
	}
	return requested, true
}

func (h *SummaryHandler) resolveRegenerateSummaryProfile(c *gin.Context, subject string, override *string) (string, error) {
	requested, sent := summaryProfileOverride(override)
	if !sent {
		return h.resolveSummaryProfile(c, subject)
	}
	if !h.summaryProfilesEnabled {
		return "", fmt.Errorf("summary profiles are disabled: %w", domain.ErrInvalidInput)
	}
	if err := validateSummaryProfileOverride(requested); err != nil {
		return "", err
	}
	return requested, nil
}

// resolveCreateSummaryProfile resolves the profile for a summary that is being
// generated for the first time (POST /summaries and generate-all).
//
// It deliberately differs from regenerate on one case: an override sent while
// the feature flag is off is ignored rather than rejected. Initial generation
// is the automatic path — the briefing pane fires it without user interaction —
// so a client that always sends its preferred profile must keep working against
// a deployment that has profiles turned off. Regenerate is always user-driven,
// so there a silently ignored choice would be the worse answer.
func (h *SummaryHandler) resolveCreateSummaryProfile(c *gin.Context, subject string, override *string) (string, error) {
	requested, sent := summaryProfileOverride(override)
	if !sent || !h.summaryProfilesEnabled {
		return h.resolveSummaryProfile(c, subject)
	}
	if err := validateSummaryProfileOverride(requested); err != nil {
		return "", err
	}
	return requested, nil
}

// checkCreateSummaryProfile runs only the cheap syntactic half of profile
// resolution. It exists so an unsupported summary_profile is rejected *before*
// the billable rate limiter is charged: validating a string costs nothing, and
// a request that can only 400 must not burn tokens. The preference lookup stays
// in resolveCreateSummaryProfile — it hits the user repository, so it belongs
// after the limiter with the rest of the real work.
func (h *SummaryHandler) checkCreateSummaryProfile(override *string) error {
	requested, sent := summaryProfileOverride(override)
	if !sent || !h.summaryProfilesEnabled {
		return nil
	}
	return validateSummaryProfileOverride(requested)
}

// validateSummaryProfileOverride checks a caller-supplied profile value against
// the canonical domain list, producing the shared 400 error shape.
func validateSummaryProfileOverride(profile string) error {
	if !domain.IsSummaryProfile(profile) {
		return fmt.Errorf("unsupported summary profile %q: %w", truncateForError(profile), domain.ErrInvalidInput)
	}
	return nil
}

// ListByTranscription handles GET /api/v1/transcriptions/:id/summaries.
func (h *SummaryHandler) ListByTranscription(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	items, err := h.summarySvc.ListByTranscription(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	response := make([]SummaryResponse, 0, len(items))
	for _, s := range items {
		response = append(response, toSummaryResponse(s))
	}

	c.JSON(http.StatusOK, response)
}

// GenerateAll handles POST /api/v1/transcriptions/:id/summaries.
// It generates all four summary types for a completed transcription in a single
// idempotent round-trip and returns the resulting summary collection. Every
// source type is accepted — media, recording and url alike — so both reader
// adapters share one "Generate all" path; per-type generation stays on POST
// /summaries. Mirrors Create's guards: 503 when AI is unconfigured and the
// billable rate limiter, charged summaryFanOutTypes tokens since one call fans
// out to four billable jobs. An optional JSON body may carry summary_profile
// for this run.
func (h *SummaryHandler) GenerateAll(c *gin.Context) {
	orgID, subject := h.resolveOrgAndSubject(c)
	if orgID == "" {
		return
	}

	// Reject early if AI summarization is unavailable. The reused service code
	// otherwise logs a warning and returns nil, which would be a false 202.
	if h.jobInserter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service_unavailable", "message": "AI summarization is not configured"})
		return
	}

	// The body is optional: this endpoint shipped bodyless, and a bare
	// ShouldBindJSON would fail every existing caller with io.EOF. Same guard
	// Regenerate uses.
	var req GenerateAllSummariesRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid JSON body"})
			return
		}
	}

	transcriptionID := c.Param("id")

	// Probe access before charging anything: this applies org isolation, the
	// deleted check and the privilege guard (all of them ErrNotFound) and keeps
	// unknown ids off the billable budget. ProbeAccess and not GetByID on
	// purpose — GetByID decrypts the whole stored transcript (up to 500KB of
	// AES-GCM plus a JSON parse) and loads media metadata, all of it discarded
	// here, and all of it paid for before the limiter had charged a thing.
	// Every source type is accepted — the service is source-type-agnostic and a
	// media session's briefing pane uses this same endpoint for its "Generate
	// all" control.
	if err := h.transSvc.ProbeAccess(c.Request.Context(), orgID, transcriptionID); err != nil {
		h.respondGenerateError(c, err, transcriptionID)
		return
	}

	// Charge the billable limiter only after the cheap not-found check and the
	// syntactic profile check, so probes for missing ids and requests carrying
	// an unsupported profile don't burn the fan-out budget. One call fans out to
	// summaryFanOutTypes billable jobs; charge accordingly.
	if profileErr := h.checkCreateSummaryProfile(req.SummaryProfile); profileErr != nil {
		h.handleError(c, profileErr)
		return
	}
	if !h.allowBillable(c, subject, summaryFanOutTypes) {
		return
	}

	// Generate all four types idempotently. Per-type conflicts are swallowed by
	// the service, so ErrConflict here means the transcription is not completed.
	summaryProfile, profileErr := h.resolveCreateSummaryProfile(c, subject, req.SummaryProfile)
	if profileErr != nil {
		h.handleError(c, profileErr)
		return
	}
	if genErr := h.transSvc.GenerateAllSummariesWithProfile(
		c.Request.Context(), orgID, transcriptionID, subject, summaryProfile,
	); genErr != nil {
		h.respondGenerateError(c, genErr, transcriptionID)
		return
	}

	// Return the resulting collection so the client can observe what was queued.
	items, err := h.summarySvc.ListByTranscription(c.Request.Context(), orgID, transcriptionID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response := make([]SummaryResponse, 0, len(items))
	for _, s := range items {
		response = append(response, toSummaryResponse(s))
	}

	c.JSON(http.StatusAccepted, response)
}

// respondGenerateError maps a transcription-load or generation error to an HTTP
// response for the generate-all endpoint: a per-type fan-out failure → 500,
// not found / wrong org / deleted / privilege-blocked → 404, privilege media →
// 403, not completed (ErrConflict) → 409, anything else → 500.
//
// The fan-out case is checked first and by type, not by sentinel. Once the load
// phase has passed, a joined per-type failure can contain ErrConflict or
// ErrNotFound describing one summary type; matching those would answer "3 of 4
// were queued" with a 409 claiming the transcription is not completed.
//
// The ErrPrivilegeOnly branch is defense in depth, not the live path: the probe
// runs guardPrivilegeRead, which maps a privilege refusal to ErrNotFound so the
// endpoint hides the existence of privilege transcripts. A 404 is therefore the
// expected answer for privilege media today. The branch stays so that a future
// change to the service-layer ordering surfaces the refusal as a 403 rather
// than an opaque 500.
func (h *SummaryHandler) respondGenerateError(c *gin.Context, err error, transcriptionID string) {
	var fanOut *service.SummaryFanOutError
	switch {
	case errors.As(err, &fanOut):
		h.logger.Error("generate all summaries: some types failed",
			"error", err, "transcription_id", transcriptionID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "some summaries could not be generated",
		})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "transcription not found"})
	case errors.Is(err, domain.ErrPrivilegeOnly):
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "privilege_only",
			"message": "this media requires the privilege workspace to summarize",
		})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "transcription is not completed"})
	default:
		h.logger.Error("generate all summaries failed", "error", err, "transcription_id", transcriptionID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to generate summaries"})
	}
}
