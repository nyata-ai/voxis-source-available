package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// FeatureFlags represents the current feature flag state.
type FeatureFlags struct {
	EnhanceAudio            bool `json:"enhance_audio"`             // audio preprocessing available
	DeepFilter              bool `json:"deep_filter"`               // DeepFilterNet enhancement available
	SummaryHighStakes       bool `json:"summary_high_stakes"`       // high-fidelity two-pass summaries available
	SummaryStructuredOutput bool `json:"summary_structured_output"` // schema-validated summaries available
	SummaryProfiles         bool `json:"summary_profiles"`          // user-selected summary profiles available
	SummarySharedAnalysis   bool `json:"summary_shared_analysis"`   // reusable transcript analysis available
	AccountSecurity         bool `json:"account_security"`          // Keycloak-backed account security API available
	Billing                 bool `json:"billing"`                   // credit purchases and billing routes registered
	CustomVocabulary        bool `json:"custom_vocabulary"`         // built-in vocabulary packs selectable at submission
	BAPExport               bool `json:"bap_export"`                // Berita Acara Pemeriksaan draft DOCX export available
}

// FeaturesHandler serves feature flag state to the frontend.
type FeaturesHandler struct {
	flags FeatureFlags
}

// NewFeaturesHandler creates a new FeaturesHandler with the given flag state.
func NewFeaturesHandler(flags FeatureFlags) *FeaturesHandler {
	return &FeaturesHandler{flags: flags}
}

// GetFeatures handles GET /api/v1/features.
func (h *FeaturesHandler) GetFeatures(c *gin.Context) {
	c.JSON(http.StatusOK, h.flags)
}
