package mcpserver

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

// validateSortBy checks that field is in the allowlist or empty.
func validateSortBy(field string, allowlist []string) error {
	if field == "" {
		return nil
	}
	for _, a := range allowlist {
		if field == a {
			return nil
		}
	}
	return fmt.Errorf("invalid sort_by %q: allowed values are %v", field, allowlist)
}

// validateSortOrder checks that order is "asc", "desc", or empty (defaults to desc).
func validateSortOrder(order string) error {
	switch order {
	case "", "asc", "desc":
		return nil
	default:
		return fmt.Errorf("invalid sort_order %q: must be \"asc\" or \"desc\"", order)
	}
}

// validateDateRange validates RFC 3339 date strings and ensures to >= from.
func validateDateRange(from, to string) error {
	var fromTime, toTime time.Time
	var err error

	if from != "" {
		fromTime, err = time.Parse(time.RFC3339, from)
		if err != nil {
			return fmt.Errorf("invalid date_from: must be RFC 3339 format (e.g. 2026-01-01T00:00:00Z)")
		}
	}

	if to != "" {
		toTime, err = time.Parse(time.RFC3339, to)
		if err != nil {
			return fmt.Errorf("invalid date_to: must be RFC 3339 format (e.g. 2026-12-31T23:59:59Z)")
		}
	}

	if from != "" && to != "" && toTime.Before(fromTime) {
		return fmt.Errorf("date_to must be greater than or equal to date_from")
	}

	return nil
}

// validateIntRange checks that val is within [lo, hi].
func validateIntRange(val, lo, hi int, name string) error {
	if val < lo || val > hi {
		return fmt.Errorf("invalid %s: must be between %d and %d, got %d", name, lo, hi, val)
	}
	return nil
}

// validateFloatRange checks that val is within [lo, hi].
func validateFloatRange(val, lo, hi float64, name string) error {
	if val < lo || val > hi {
		return fmt.Errorf("invalid %s: must be between %.0f and %.0f, got %.2f", name, lo, hi, val)
	}
	return nil
}

// validateMinMaxFloat checks that lower <= upper for a float pair.
func validateMinMaxFloat(lower, upper float64, name string) error {
	if lower > upper {
		return fmt.Errorf("min_%s must be less than or equal to max_%s", name, name)
	}
	return nil
}

// validateMinMaxInt checks that lower <= upper for an int pair.
func validateMinMaxInt(lower, upper int, name string) error {
	if lower > upper {
		return fmt.Errorf("min_%s must be less than or equal to max_%s", name, name)
	}
	return nil
}

// maxOffsetCap caps deep pagination at the MCP boundary so an LLM cannot
// drive unbounded sequential scans. Callers that need more depth must narrow
// the query with filters or search instead.
const maxOffsetCap = service.MaxSearchOffset

// validateOffsetCap rejects offsets above maxOffsetCap. Callers should run
// this before forwarding to service/repository layers.
func validateOffsetCap(offset int) error {
	if offset > maxOffsetCap {
		return fmt.Errorf("offset exceeds maximum of %d; narrow the query with filters or search", maxOffsetCap)
	}
	return nil
}

// validateSearch checks that the search string does not exceed maxLen.
func validateSearch(q string, maxLen int) error {
	if utf8.RuneCountInString(strings.TrimSpace(q)) > maxLen {
		return fmt.Errorf("search query exceeds maximum length of %d characters", maxLen)
	}
	return nil
}

// validateStatus checks that status is in the allowlist or empty.
func validateStatus(s string, allowlist []string) error {
	if s == "" {
		return nil
	}
	for _, a := range allowlist {
		if s == a {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q: allowed values are %v", s, allowlist)
}

// bcp47Re intentionally accepts 2-letter ISO 639-1 codes only because Gladia
// accepts these. Widen to full BCP 47 only when regional variants are required.
var bcp47Re = regexp.MustCompile(`^[a-z]{2}$`)

// validateLanguages validates a list of BCP 47 language codes (2-letter).
func validateLanguages(langs []string) error {
	if len(langs) == 0 {
		return nil
	}
	if len(langs) > 10 {
		return fmt.Errorf("languages: maximum 10 elements allowed, got %d", len(langs))
	}
	for _, l := range langs {
		if !bcp47Re.MatchString(l) {
			return fmt.Errorf("invalid language code %q: must be a 2-letter BCP 47 code (e.g. \"en\", \"id\")", l)
		}
	}
	return nil
}

// validSummaryTypes is the set of valid summary type values.
var validSummaryTypes = []string{"general", "key_points", "action_items", "q_and_a"}

// validateSummaryType checks that the summary type is valid or empty.
func validateSummaryType(st string) error {
	if st == "" {
		return nil
	}
	for _, v := range validSummaryTypes {
		if st == v {
			return nil
		}
	}
	return fmt.Errorf("invalid summary_type %q: must be one of general, key_points, action_items, q_and_a", st)
}

// validateSummaryProfile checks that the professional summary profile is valid
// or empty, against the canonical domain list.
func validateSummaryProfile(profile string) error {
	if profile == "" {
		return nil
	}
	if !domain.IsSummaryProfile(profile) {
		return fmt.Errorf("invalid summary_profile %q: must be one of %s", profile, strings.Join(domain.SummaryProfiles, ", "))
	}
	return nil
}
