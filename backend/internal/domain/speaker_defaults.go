package domain

import (
	"strconv"
	"strings"
)

// MaxSpeakerNameLength bounds stored speaker labels and provider suggestions.
const MaxSpeakerNameLength = 120

// IsDefaultSpeakerName reports whether name is an automatic Speaker N label.
func IsDefaultSpeakerName(name string) bool {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) != 2 || parts[0] != "Speaker" {
		return len(parts) == 0
	}
	n, err := strconv.Atoi(parts[1])
	return err == nil && n >= 0
}
