package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
)

// MaxSearchQueryRunes is the maximum accepted search query length.
const MaxSearchQueryRunes = 200

// MaxSearchOffset caps deep pagination for shared search surfaces.
const MaxSearchOffset = 10_000

// NormalizeSearchQuery applies the shared search-query contract used by REST,
// MCP, and future semantic search providers.
func NormalizeSearchQuery(search string) (string, error) {
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > MaxSearchQueryRunes {
		return "", fmt.Errorf("search query exceeds %d characters: %w", MaxSearchQueryRunes, domain.ErrInvalidInput)
	}
	return search, nil
}

// ValidateSearchOffset caps deep pagination for shared search surfaces.
func ValidateSearchOffset(offset int) error {
	if offset > MaxSearchOffset {
		return fmt.Errorf("offset exceeds maximum of %d; narrow the query with filters or search: %w", MaxSearchOffset, domain.ErrInvalidInput)
	}
	return nil
}
