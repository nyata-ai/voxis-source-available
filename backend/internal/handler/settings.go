package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// SettingsHandler handles user settings HTTP requests.
type SettingsHandler struct {
	orgService *service.OrganizationService
	userRepo   port.UserRepository
	logger     *slog.Logger
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(orgService *service.OrganizationService, userRepo port.UserRepository, logger *slog.Logger) *SettingsHandler {
	if orgService == nil {
		panic("settings handler: organization service cannot be nil")
	}
	if userRepo == nil {
		panic("settings handler: user repository cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SettingsHandler{orgService: orgService, userRepo: userRepo, logger: logger}
}

// updatePreferencesRequest uses pointers for optional fields to distinguish
// "not provided" from "zero value".
type updatePreferencesRequest struct {
	Theme               *string   `json:"theme"`
	UILanguage          *string   `json:"ui_language"`
	DefaultLanguage     *string   `json:"default_language"`
	DefaultLanguages    *[]string `json:"default_languages"`
	DefaultDiarization  *bool     `json:"default_diarization"`
	DefaultSummaryType  *string   `json:"default_summary_type"`
	DefaultExportFmt    *string   `json:"default_export_format"`
	PlaybackSpeed       *float64  `json:"playback_speed"`
	HighStakesSummaries *bool     `json:"high_stakes_summaries"`
	SummaryProfile      *string   `json:"summary_profile"`
	AutoBriefings       *bool     `json:"auto_briefings"`
}

// Allowed values for validation. Summary types and profiles are derived from
// the domain's canonical ordered lists so this handler cannot drift from them.
var (
	validThemes         = map[string]bool{"light": true, "dark": true, "system": true}
	validSummaryType    = allowedSet(domain.DefaultSummaryTypes)
	validExportFmt      = map[string]bool{"pdf": true, "docx": true, "json": true}
	validSummaryProfile = allowedSet(domain.SummaryProfiles)

	summaryTypeDesc    = strings.Join(domain.DefaultSummaryTypes, ", ")
	summaryProfileDesc = strings.Join(domain.SummaryProfiles, ", ")
)

// allowedSet builds a membership map from an ordered allowlist.
func allowedSet(values []string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// GetPreferences returns the current user's preferences with defaults merged.
func (h *SettingsHandler) GetPreferences(c *gin.Context) {
	_, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if claims == nil {
		return // error already written
	}

	prefs, err := h.loadPreferences(c, claims.Subject)
	if err != nil {
		return // error already written
	}

	c.JSON(http.StatusOK, prefs)
}

// UpdatePreferences validates and stores user preferences.
func (h *SettingsHandler) UpdatePreferences(c *gin.Context) {
	_, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if claims == nil {
		return
	}

	var req updatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "invalid JSON body",
		})
		return
	}

	// Validate provided fields
	if err := validatePreferencesRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	// Load existing preferences to merge with
	existing, err := h.loadPreferences(c, claims.Subject)
	if err != nil {
		return
	}

	// Apply provided fields (only non-nil pointers)
	applyPreferenceUpdates(&existing, &req)

	// Marshal and persist
	data, err := json.Marshal(existing)
	if err != nil {
		h.logger.Error("failed to marshal preferences",
			"error", err,
			"subject", claims.Subject,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to save preferences",
		})
		return
	}

	if err := h.userRepo.UpdatePreferences(c.Request.Context(), claims.Subject, data); err != nil {
		h.logger.Error("failed to update preferences",
			"error", err,
			"subject", claims.Subject,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to save preferences",
		})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// loadPreferences fetches raw preferences from the repo, unmarshals, and merges with defaults.
func (h *SettingsHandler) loadPreferences(c *gin.Context, userID string) (domain.UserPreferences, error) {
	raw, err := h.userRepo.GetPreferences(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// User doesn't exist yet — return defaults
			return domain.DefaultPreferences(), nil
		}
		h.logger.Error("failed to get preferences",
			"error", err,
			"subject", userID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to load preferences",
		})
		return domain.UserPreferences{}, err
	}

	var prefs domain.UserPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		h.logger.Error("failed to unmarshal preferences",
			"error", err,
			"subject", userID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to load preferences",
		})
		return domain.UserPreferences{}, err
	}

	return prefs.MergeWithDefaults(), nil
}

// applyPreferenceUpdates overlays the non-nil fields of req onto existing.
// Extracted from UpdatePreferences to keep that handler's control flow simple.
func applyPreferenceUpdates(existing *domain.UserPreferences, req *updatePreferencesRequest) {
	if req.Theme != nil {
		existing.Theme = *req.Theme
	}
	if req.UILanguage != nil {
		existing.UILanguage = *req.UILanguage
	}
	if req.DefaultLanguages != nil {
		existing.DefaultLanguages = *req.DefaultLanguages
		existing.DefaultLanguage = "" // clear deprecated field
	} else if req.DefaultLanguage != nil {
		// Backward compat: migrate deprecated field to new array field.
		existing.DefaultLanguages = []string{*req.DefaultLanguage}
		existing.DefaultLanguage = "" // clear deprecated field
	}
	if req.DefaultDiarization != nil {
		existing.DefaultDiarization = req.DefaultDiarization
	}
	if req.DefaultSummaryType != nil {
		existing.DefaultSummaryType = *req.DefaultSummaryType
	}
	if req.DefaultExportFmt != nil {
		existing.DefaultExportFmt = *req.DefaultExportFmt
	}
	if req.PlaybackSpeed != nil {
		existing.PlaybackSpeed = *req.PlaybackSpeed
	}
	if req.HighStakesSummaries != nil {
		existing.HighStakesSummaries = *req.HighStakesSummaries
	}
	if req.SummaryProfile != nil {
		existing.SummaryProfile = *req.SummaryProfile
	}
	if req.AutoBriefings != nil {
		existing.AutoBriefings = *req.AutoBriefings
	}
}

// validatePreferenceEnum returns an error if val is provided (non-nil) but is not
// a member of the allowed set.
func validatePreferenceEnum(val *string, allowed map[string]bool, field, allowedDesc string) error {
	if val != nil && !allowed[*val] {
		return fmt.Errorf("invalid %s: must be one of %s", field, allowedDesc)
	}
	return nil
}

// validatePreferencesRequest checks all provided fields for valid values.
func validatePreferencesRequest(req *updatePreferencesRequest) error {
	enumChecks := []struct {
		val         *string
		allowed     map[string]bool
		field       string
		allowedDesc string
	}{
		{req.Theme, validThemes, "theme", "light, dark, system"},
		{req.UILanguage, domain.SupportedUILanguages, "ui_language", domain.SupportedUILanguagesDesc},
		{req.DefaultSummaryType, validSummaryType, "default_summary_type", summaryTypeDesc},
		{req.DefaultExportFmt, validExportFmt, "default_export_format", "pdf, docx, json"},
		{req.SummaryProfile, validSummaryProfile, "summary_profile", summaryProfileDesc},
	}
	for _, chk := range enumChecks {
		if err := validatePreferenceEnum(chk.val, chk.allowed, chk.field, chk.allowedDesc); err != nil {
			return err
		}
	}
	if req.DefaultLanguage != nil && !domain.AllowedLanguages[*req.DefaultLanguage] {
		return fmt.Errorf("invalid default_language: must be a valid language code")
	}
	if req.DefaultLanguages != nil {
		if err := validateDefaultLanguages(*req.DefaultLanguages); err != nil {
			return err
		}
	}
	if req.PlaybackSpeed != nil && (*req.PlaybackSpeed < 0.5 || *req.PlaybackSpeed > 2.0) {
		return fmt.Errorf("invalid playback_speed: must be between 0.5 and 2.0")
	}
	return nil
}

// validateDefaultLanguages checks the default_languages array for valid values,
// duplicates, and auto-mixing constraints.
func validateDefaultLanguages(langs []string) error {
	if len(langs) == 0 {
		return fmt.Errorf("default_languages must contain at least one language")
	}
	if len(langs) > 5 {
		return fmt.Errorf("default_languages allows at most 5 languages")
	}
	hasAuto := false
	seen := make(map[string]bool, len(langs))
	for _, l := range langs {
		if !domain.AllowedLanguages[l] {
			return fmt.Errorf("invalid language code %q in default_languages", l)
		}
		if seen[l] {
			return fmt.Errorf("duplicate language %q in default_languages", l)
		}
		seen[l] = true
		if l == "auto" {
			hasAuto = true
		}
	}
	if hasAuto && len(langs) > 1 {
		return fmt.Errorf("\"auto\" cannot be combined with specific languages in default_languages")
	}
	return nil
}
