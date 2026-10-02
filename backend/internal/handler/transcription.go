package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
	"github.com/voxis/backend/internal/vocab"
)

// TranscriptionHandler handles transcription-related HTTP requests.
type TranscriptionHandler struct {
	transSvc                *service.TranscriptionService
	orgService              *service.OrganizationService
	jobInserter             port.TranscriptionJobInserter
	enhanceAvailable        bool
	customVocabularyEnabled bool
	logger                  *slog.Logger
}

// NewTranscriptionHandler creates a new TranscriptionHandler.
// jobInserter may be nil if transcription processing is unavailable (e.g., no Gladia API key).
// enhanceAvailable indicates whether audio preprocessing is available on this server.
func NewTranscriptionHandler(
	transSvc *service.TranscriptionService,
	orgService *service.OrganizationService,
	jobInserter port.TranscriptionJobInserter,
	enhanceAvailable bool,
	logger *slog.Logger,
) *TranscriptionHandler {
	if transSvc == nil {
		panic("transcription handler: transcription service cannot be nil")
	}
	if orgService == nil {
		panic("transcription handler: organization service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TranscriptionHandler{
		transSvc:         transSvc,
		orgService:       orgService,
		jobInserter:      jobInserter,
		enhanceAvailable: enhanceAvailable,
		logger:           logger,
	}
}

// SetCustomVocabularyEnabled controls whether vocabulary_packs on a submission
// is honored. When disabled the field is ignored rather than rejected: creation
// is the automatic path, so a client that always sends packs must keep working
// on a deployment that has the feature off.
func (h *TranscriptionHandler) SetCustomVocabularyEnabled(enabled bool) {
	h.customVocabularyEnabled = enabled
}

// TranscriptionResponse is the JSON response for a single transcription.
type TranscriptionResponse struct {
	ID               string   `json:"id"`
	OrganizationID   string   `json:"organization_id"`
	MediaID          string   `json:"media_id"`
	MediaFilename    string   `json:"media_filename"`
	MediaTitle       string   `json:"media_title"`
	MediaDescription string   `json:"media_description"`
	MediaStatus      string   `json:"media_status"`
	Status           string   `json:"status"`
	Languages        []string `json:"languages"`
	Diarization      bool     `json:"diarization"`
	EnhanceAudio     bool     `json:"enhance_audio"`
	PreprocessorUsed string   `json:"preprocessor_used,omitempty"`
	SpeakerCount     int      `json:"speaker_count"`
	WordCount        int      `json:"word_count"`
	DurationSeconds  float64  `json:"duration_seconds"`
	ErrorMessage     string   `json:"error_message,omitempty"`
	// VocabularyPacks echoes the pack ids applied to this transcription so a
	// client can tell what its submission actually selected. Omitted entirely
	// when no packs were applied.
	VocabularyPacks []string `json:"vocabulary_packs,omitempty"`
	// ExpectedSpeakers echoes the speaker count the submitter declared. Omitted
	// when the submission left it on auto-detect. This is the REQUEST value —
	// speaker_count above is what the provider actually found.
	ExpectedSpeakers int    `json:"expected_speakers,omitempty"`
	CreatedAt        string `json:"created_at"`
	CompletedAt      string `json:"completed_at,omitempty"`
}

// TranscriptionDetailResponse extends TranscriptionResponse with decrypted content.
type TranscriptionDetailResponse struct {
	TranscriptionResponse
	FullTranscript      string                              `json:"full_transcript,omitempty"`
	Utterances          json.RawMessage                     `json:"utterances,omitempty"`
	SpeakerMap          map[string]string                   `json:"speaker_map,omitempty"`
	SuggestedSpeakerMap map[string]domain.SpeakerSuggestion `json:"suggested_speaker_map,omitempty"`
	// suggestions_generated intentionally has NO omitempty: the frontend must
	// distinguish false (not yet generated → trigger auto-generation) from absent.
	// Do not "tidy" this into omitempty — it breaks auto-generation.
	SuggestionsGenerated bool `json:"suggestions_generated"`
}

// ListTranscriptionResponse is the JSON response for listing transcriptions.
type ListTranscriptionResponse struct {
	Items []TranscriptionResponse `json:"items"`
	Total int64                   `json:"total"`
}

// CreateTranscriptionRequest is the JSON request body for creating a transcription.
type CreateTranscriptionRequest struct {
	MediaID         string   `json:"media_id" binding:"required"`
	Languages       []string `json:"languages"`
	Diarization     *bool    `json:"diarization"`
	EnhanceAudio    *bool    `json:"enhance_audio"`
	VocabularyPacks []string `json:"vocabulary_packs"`
	// ExpectedSpeakers is the number of speakers the submitter declares.
	// Omit (or null) to let the provider decide. Only meaningful with
	// diarization on; ignored otherwise.
	ExpectedSpeakers *int `json:"expected_speakers"`
}

// resolveExpectedSpeakers validates the optional declared speaker count on a
// submission. Shared by the media and URL creation handlers.
//
// Returns the count (0 = auto-detect) and true on success; on failure it writes
// a 400 and returns false. With diarization off the field is silently ignored
// rather than rejected — it is a hint for speaker separation, and a client that
// always sends it must keep working when the user turns diarization off.
func resolveExpectedSpeakers(c *gin.Context, requested *int, diarization bool) (int, bool) {
	if requested == nil || !diarization {
		return 0, true
	}
	if err := domain.ValidateExpectedSpeakers(*requested); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": fmt.Sprintf("expected_speakers must be between 1 and %d", domain.MaxExpectedSpeakers),
		})
		return 0, false
	}
	return *requested, true
}

// resolveVocabularyPacks validates the vocabulary pack ids on a submission.
// Shared by the media and URL creation handlers.
//
// Returns the deduplicated ids and true on success; on failure it writes a 400
// and returns false. When the feature is disabled the field is ignored rather
// than rejected — creation is an automatic path, so a client that always sends
// packs must keep working on a deployment that has the feature off.
func resolveVocabularyPacks(c *gin.Context, requested []string, enabled bool) ([]string, bool) {
	if !enabled || len(requested) == 0 {
		return nil, true
	}
	if len(requested) > vocab.MaxPacksPerRequest {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": fmt.Sprintf("at most %d vocabulary packs allowed", vocab.MaxPacksPerRequest),
		})
		return nil, false
	}

	seen := make(map[string]bool, len(requested))
	packs := make([]string, 0, len(requested))
	for _, raw := range requested {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !vocab.IsValidID(id) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "bad_request",
				"message": fmt.Sprintf("unknown vocabulary pack %q (valid: %s)",
					truncateForError(id), strings.Join(vocab.ValidIDs(), ", ")),
			})
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		packs = append(packs, id)
	}
	if len(packs) == 0 {
		return nil, true
	}
	return packs, true
}

// maxReflectedErrorRunes bounds how much caller-supplied text an error message
// may echo back.
const maxReflectedErrorRunes = 64

// truncateForError bounds a caller-supplied value that is about to be reflected
// in an error message. An unbounded echo lets a client inflate a 400 body with
// whatever it sends, and the value ends up in logs as well.
func truncateForError(value string) string {
	if utf8.RuneCountInString(value) <= maxReflectedErrorRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxReflectedErrorRunes]) + "…"
}

// UpdateSpeakersRequest is the JSON request body for updating speaker names.
type UpdateSpeakersRequest struct {
	SpeakerMap map[string]string `json:"speaker_map" binding:"required"`
}

// toTranscriptionResponse converts a domain.Transcription to a TranscriptionResponse.
func toTranscriptionResponse(t *domain.Transcription) TranscriptionResponse {
	languages := t.Languages
	if languages == nil {
		languages = []string{}
	}
	resp := TranscriptionResponse{
		ID:               t.ID,
		OrganizationID:   t.OrganizationID,
		MediaID:          t.MediaID,
		MediaFilename:    t.MediaFilename,
		MediaTitle:       t.MediaTitle,
		MediaDescription: t.MediaDescription,
		MediaStatus:      t.MediaStatus,
		Status:           t.Status,
		Languages:        languages,
		Diarization:      t.Diarization,
		EnhanceAudio:     t.EnhanceAudio,
		PreprocessorUsed: t.PreprocessorUsed,
		SpeakerCount:     t.SpeakerCount,
		WordCount:        t.WordCount,
		DurationSeconds:  t.DurationSeconds,
		ErrorMessage:     t.ErrorMessage,
		VocabularyPacks:  t.VocabularyPacks,
		ExpectedSpeakers: t.ExpectedSpeakers,
		CreatedAt:        t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if t.CompletedAt != nil {
		resp.CompletedAt = t.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

// toTranscriptionDetailResponse converts a domain.Transcription to a TranscriptionDetailResponse.
func toTranscriptionDetailResponse(t *domain.Transcription) TranscriptionDetailResponse {
	return TranscriptionDetailResponse{
		TranscriptionResponse: toTranscriptionResponse(t),
		FullTranscript:        t.FullTranscript,
		Utterances:            t.Utterances,
		SpeakerMap:            t.SpeakerMap,
		SuggestedSpeakerMap:   t.SuggestedSpeakerMap,
		SuggestionsGenerated:  t.SuggestionsGenerated,
	}
}

// resolveOrgID handles both JWT and API key auth paths.
// Returns just the org ID (claims are unused in transcription handlers).
func (h *TranscriptionHandler) resolveOrgID(c *gin.Context) string {
	orgID, _ := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	return orgID
}

// handleError maps domain errors to HTTP status codes.
func (h *TranscriptionHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "transcription not found"})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "access denied"})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "an active transcription already exists for this media"})
	case errors.Is(err, domain.ErrCreditsExpired):
		// Checked before the shortfall case: expiry demands a different user
		// action (buy more to restore access, not top up a shortfall).
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "credits_expired",
			"message": "your credits have expired — purchase more to restore access",
		})
	case errors.Is(err, domain.ErrInsufficientCredits):
		// Same shape as the URL transcription handler so one frontend branch
		// covers both submission paths.
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "insufficient_credits",
			"message": "your organization is out of credits — top up to start new transcriptions",
		})
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
		h.logger.Error("transcription handler error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "an internal error occurred"})
	}
}

// Create handles POST /api/v1/transcriptions.
func (h *TranscriptionHandler) Create(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	var req CreateTranscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "media_id is required"})
		return
	}

	// Apply defaults.
	languages := req.Languages
	if len(languages) == 0 {
		languages = []string{"auto"}
	}

	diarization := true
	if req.Diarization != nil {
		diarization = *req.Diarization
	}

	enhanceAudio := false
	if req.EnhanceAudio != nil && *req.EnhanceAudio {
		if !h.enhanceAvailable {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "audio enhancement is not available on this server"})
			return
		}
		enhanceAudio = true
	}

	vocabularyPacks, ok := resolveVocabularyPacks(c, req.VocabularyPacks, h.customVocabularyEnabled)
	if !ok {
		return
	}

	expectedSpeakers, ok := resolveExpectedSpeakers(c, req.ExpectedSpeakers, diarization)
	if !ok {
		return
	}

	// Reject early if transcription processing is unavailable.
	if h.jobInserter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service_unavailable", "message": "transcription processing is not configured"})
		return
	}

	trans, err := h.transSvc.CreateWithOptions(c.Request.Context(), orgID, req.MediaID, languages, diarization, enhanceAudio,
		service.SubmissionOptions{VocabularyPacks: vocabularyPacks, ExpectedSpeakers: expectedSpeakers})
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Enqueue background transcription job.
	if insertErr := h.jobInserter.InsertTranscribeJob(c.Request.Context(), trans.ID, req.MediaID, orgID); insertErr != nil {
		h.logger.Error("failed to enqueue transcription job", "error", insertErr, "transcription_id", trans.ID)
		// Clean up the orphaned transcription to avoid blocking future retries.
		if delErr := h.transSvc.Delete(c.Request.Context(), orgID, trans.ID); delErr != nil {
			h.logger.Error("failed to clean up orphaned transcription", "error", delErr, "transcription_id", trans.ID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to start transcription processing"})
		return
	}

	c.JSON(http.StatusAccepted, toTranscriptionResponse(trans))
}

// List handles GET /api/v1/transcriptions.
func (h *TranscriptionHandler) List(c *gin.Context) {
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

	items, total, err := h.transSvc.List(c.Request.Context(), orgID, limit, offset, search)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response := ListTranscriptionResponse{
		Items: make([]TranscriptionResponse, 0, len(items)),
		Total: total,
	}
	for _, t := range items {
		response.Items = append(response.Items, toTranscriptionResponse(t))
	}

	c.JSON(http.StatusOK, response)
}

// GetByID handles GET /api/v1/transcriptions/:id.
func (h *TranscriptionHandler) GetByID(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	trans, err := h.transSvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, toTranscriptionDetailResponse(trans))
}

// UpdateSpeakers handles PATCH /api/v1/transcriptions/:id/speakers.
func (h *TranscriptionHandler) UpdateSpeakers(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	var req UpdateSpeakersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "speaker_map is required"})
		return
	}

	if err := h.transSvc.UpdateSpeakers(c.Request.Context(), orgID, c.Param("id"), req.SpeakerMap); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// GenerateSpeakerSuggestions handles POST /api/v1/transcriptions/:id/speakers/suggestions.
// It lazily produces AI speaker-name suggestions and returns the full transcription
// detail (including the suggestion fields). Idempotent: a repeat call returns the
// already-generated suggestions without re-invoking the model.
func (h *TranscriptionHandler) GenerateSpeakerSuggestions(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	trans, err := h.transSvc.GenerateSpeakerSuggestions(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "speaker suggestions are being updated concurrently; please retry"})
			return
		}
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, toTranscriptionDetailResponse(trans))
}

// DismissSpeakerSuggestion handles DELETE /api/v1/transcriptions/:id/speakers/suggestions/:index.
// It removes one AI suggestion by speaker index. Dismissing an absent index is a no-op.
func (h *TranscriptionHandler) DismissSpeakerSuggestion(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	if err := h.transSvc.DismissSpeakerSuggestion(c.Request.Context(), orgID, c.Param("id"), c.Param("index")); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "speaker suggestions are being updated concurrently; please retry"})
			return
		}
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// Delete handles DELETE /api/v1/transcriptions/:id.
func (h *TranscriptionHandler) Delete(c *gin.Context) {
	orgID := h.resolveOrgID(c)
	if orgID == "" {
		return
	}

	if err := h.transSvc.Delete(c.Request.Context(), orgID, c.Param("id")); err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
