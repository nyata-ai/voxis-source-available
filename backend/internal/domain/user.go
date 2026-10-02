package domain

import (
	"errors"
	"time"
)

// SupportedUILanguages is the canonical allowlist of UI locale codes Voxis ships
// translations for. It MUST stay in sync with the frontend's TRANSLATED_LANGUAGE_CODES
// in frontend/src/i18n/config.ts — there is no shared schema across the Go/TS boundary.
var SupportedUILanguages = map[string]bool{
	"de":    true,
	"en":    true,
	"es":    true,
	"fr":    true,
	"id":    true,
	"ja":    true,
	"ko":    true,
	"ru":    true,
	"zh-CN": true,
}

// SupportedUILanguagesDesc is the human-readable allowed-values list for validation
// error messages, kept next to SupportedUILanguages so the two do not drift.
const SupportedUILanguagesDesc = "de, en, es, fr, id, ja, ko, ru, zh-CN"

// UserPreferences holds user-configurable settings stored as JSONB.
type UserPreferences struct {
	Theme               string   `json:"theme"`
	UILanguage          string   `json:"ui_language,omitempty"`       // Chosen shipped UI locale
	DefaultLanguage     string   `json:"default_language,omitempty"`  // Deprecated: read-only for backward compat
	DefaultLanguages    []string `json:"default_languages,omitempty"` // Canonical field
	DefaultDiarization  *bool    `json:"default_diarization,omitempty"`
	DefaultSummaryType  string   `json:"default_summary_type"`
	DefaultExportFmt    string   `json:"default_export_format"`
	PlaybackSpeed       float64  `json:"playback_speed"`
	HighStakesSummaries bool     `json:"high_stakes_summaries"`
	SummaryProfile      string   `json:"summary_profile"`
	// AutoBriefings opts the session reader into generating every briefing type
	// as soon as a completed session is opened. Off by default and deliberately
	// so: generation is billable AI work, and not every recording wants all four
	// briefings. False is the zero value, so MergeWithDefaults needs no branch.
	AutoBriefings bool `json:"auto_briefings"`
}

// DefaultPreferences returns sensible defaults for new users.
func DefaultPreferences() UserPreferences {
	t := true
	return UserPreferences{
		Theme: "system",
		// UILanguage is intentionally unset. It means "the user chose this", so a
		// persisted default would be indistinguishable from an untouched account
		// and would override the language the client detected.
		DefaultLanguages:   []string{"auto"},
		DefaultDiarization: &t,
		DefaultSummaryType: "general",
		DefaultExportFmt:   "pdf",
		PlaybackSpeed:      1.0,
		SummaryProfile:     SummaryProfileGeneralProfessional,
	}
}

// MergeWithDefaults fills zero-value fields with default values.
func (p UserPreferences) MergeWithDefaults() UserPreferences {
	d := DefaultPreferences()
	if p.Theme == "" {
		p.Theme = d.Theme
	}
	// Backward compat: migrate old DefaultLanguage → DefaultLanguages
	if len(p.DefaultLanguages) == 0 {
		if p.DefaultLanguage != "" {
			p.DefaultLanguages = []string{p.DefaultLanguage}
		} else {
			p.DefaultLanguages = d.DefaultLanguages
		}
	}
	// Clear deprecated field so writes only use new field
	p.DefaultLanguage = ""
	if p.DefaultDiarization == nil {
		p.DefaultDiarization = d.DefaultDiarization
	}
	if p.DefaultSummaryType == "" {
		p.DefaultSummaryType = d.DefaultSummaryType
	}
	if p.DefaultExportFmt == "" {
		p.DefaultExportFmt = d.DefaultExportFmt
	}
	if p.PlaybackSpeed == 0 {
		p.PlaybackSpeed = d.PlaybackSpeed
	}
	p.SummaryProfile = NormalizeSummaryProfile(p.SummaryProfile)
	return p
}

// User represents a Voxis user.
// ID is the Keycloak subject (sub claim) - this ensures consistent identity
// across frontend, backend, and Keycloak.
type User struct {
	ID             string // Keycloak sub (primary key)
	OrganizationID string // UUID of the user's organization
	Email          string
	Name           string
	Role           string
	Preferences    UserPreferences
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewUser creates a new User from Keycloak claims.
// keycloakSub becomes the user ID directly.
func NewUser(keycloakSub, orgID, email, name string) (*User, error) {
	if keycloakSub == "" {
		return nil, errors.New("keycloak subject cannot be empty")
	}
	if orgID == "" {
		return nil, errors.New("organization ID cannot be empty")
	}
	if email == "" {
		return nil, errors.New("email cannot be empty")
	}

	now := time.Now()
	return &User{
		ID:             keycloakSub,
		OrganizationID: orgID,
		Email:          email,
		Name:           name,
		Role:           "user",
		Preferences:    DefaultPreferences(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// UpdateFromClaims updates user fields from fresh token claims.
// Returns true if any field was updated.
func (u *User) UpdateFromClaims(email, name string) bool {
	changed := false
	if email != "" && u.Email != email {
		u.Email = email
		changed = true
	}
	if u.Name != name {
		u.Name = name
		changed = true
	}
	if changed {
		u.UpdatedAt = time.Now()
	}
	return changed
}

// Clone returns a deep copy of the user.
func (u *User) Clone() *User {
	clone := &User{
		ID:             u.ID,
		OrganizationID: u.OrganizationID,
		Email:          u.Email,
		Name:           u.Name,
		Role:           u.Role,
		Preferences:    u.Preferences,
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
	}
	// Deep copy the pointer field
	if u.Preferences.DefaultDiarization != nil {
		v := *u.Preferences.DefaultDiarization
		clone.Preferences.DefaultDiarization = &v
	}
	// Deep copy the slice field
	if u.Preferences.DefaultLanguages != nil {
		clone.Preferences.DefaultLanguages = make([]string, len(u.Preferences.DefaultLanguages))
		copy(clone.Preferences.DefaultLanguages, u.Preferences.DefaultLanguages)
	}
	return clone
}
