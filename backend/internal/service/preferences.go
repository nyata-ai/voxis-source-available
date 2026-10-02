package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// ResolveHighStakesPreference reports whether the user opted into high-stakes
// summaries. Missing users or preferences default to false without error.
func ResolveHighStakesPreference(ctx context.Context, userRepo port.UserRepository, userID string) (bool, error) {
	if userRepo == nil || userID == "" {
		return false, nil
	}
	raw, err := userRepo.GetPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load summary preference: %w", err)
	}
	var prefs domain.UserPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return false, fmt.Errorf("parse summary preference: %w", err)
	}
	return prefs.MergeWithDefaults().HighStakesSummaries, nil
}

// ResolveSummaryProfile returns the initiating user's selected profile when
// profiles are enabled. Missing, malformed, or invalid preferences safely use
// General Professional so summary creation remains available.
func ResolveSummaryProfile(ctx context.Context, userRepo port.UserRepository, userID string, enabled bool) (string, error) {
	if !enabled || userRepo == nil || userID == "" {
		return domain.SummaryProfileGeneralProfessional, nil
	}
	raw, err := userRepo.GetPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.SummaryProfileGeneralProfessional, nil
		}
		return "", fmt.Errorf("load summary profile: %w", err)
	}
	var prefs domain.UserPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		// Deliberate fail-soft (see doc comment): malformed stored preferences
		// must not block summary creation, so fall back to the default profile.
		return domain.SummaryProfileGeneralProfessional, nil //nolint:nilerr // merge-on-read defaults are intentional
	}
	return domain.NormalizeSummaryProfile(prefs.MergeWithDefaults().SummaryProfile), nil
}
