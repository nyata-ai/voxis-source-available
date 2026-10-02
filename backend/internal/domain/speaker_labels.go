package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SpeakerIndexesFromUtterances returns the stable speaker indexes present in a
// provider utterance array. It accepts numeric IDs, numeric strings, and
// rendered default labels such as "Speaker 2".
func SpeakerIndexesFromUtterances(raw json.RawMessage) ([]int, error) {
	var utterances []struct {
		Speaker json.RawMessage `json:"speaker"`
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &utterances); err != nil {
		return nil, fmt.Errorf("parse utterances: %w", err)
	}
	seen := make(map[int]bool)
	for _, utterance := range utterances {
		if index, ok := SpeakerIndexFromRaw(utterance.Speaker); ok {
			seen[index] = true
		}
	}
	indexes := make([]int, 0, len(seen))
	for index := range seen {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes, nil
}

// SpeakerIndexFromRaw parses one raw JSON speaker value.
func SpeakerIndexFromRaw(raw json.RawMessage) (int, bool) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return speakerIndexFromNumber(number)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	return SpeakerIndexFromIdentifier(text)
}

// SpeakerIndexFromIdentifier parses a numeric speaker identifier or a rendered
// default speaker label such as "Speaker 2".
func SpeakerIndexFromIdentifier(value string) (int, bool) {
	trimmed := strings.TrimSpace(value)
	if index, ok := parseNumericSpeakerIndex(trimmed); ok {
		return index, true
	}
	if !strings.HasPrefix(trimmed, "Speaker ") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "Speaker ")))
	if err != nil || n <= 0 || n > 100 {
		return 0, false
	}
	return n - 1, true
}

func speakerIndexFromNumber(number json.Number) (int, bool) {
	f, err := number.Float64()
	if err != nil || f < 0 || f > 99 {
		return 0, false
	}
	index := int(f)
	return index, f == float64(index)
}

func parseNumericSpeakerIndex(value string) (int, bool) {
	index, err := strconv.Atoi(strings.TrimSpace(value))
	return index, err == nil && index >= 0 && index <= 99
}

// DefaultSpeakerLabel renders the canonical fallback label for a zero-based
// speaker index.
func DefaultSpeakerLabel(index int) string {
	return fmt.Sprintf("Speaker %d", index+1)
}

// ValidateSpeakerLabel checks one human-entered speaker label.
func ValidateSpeakerLabel(index int, label string) error {
	clean := strings.TrimSpace(label)
	if clean == "" {
		return fmt.Errorf("speaker label for %d is required", index)
	}
	if utf8.RuneCountInString(clean) > MaxSpeakerNameLength {
		return fmt.Errorf("speaker label for %d exceeds %d characters", index, MaxSpeakerNameLength)
	}
	if hasUnsafeSpeakerLabelContent(clean) {
		return fmt.Errorf("speaker label for %d contains unsupported control or delimiter characters", index)
	}
	return nil
}

// ValidateUniqueSpeakerLabels rejects duplicate non-empty labels after trimming
// and case-folding.
func ValidateUniqueSpeakerLabels(speakerMap map[string]string) error {
	seen := make(map[string]string, len(speakerMap))
	for key, label := range speakerMap {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized == "" {
			continue
		}
		if previous, ok := seen[normalized]; ok {
			return fmt.Errorf("speaker labels for %s and %s must be unique", previous, key)
		}
		seen[normalized] = key
	}
	return nil
}

func hasUnsafeSpeakerLabelContent(label string) bool {
	if strings.Contains(label, "```") || strings.Contains(label, "{{") || strings.Contains(label, "}}") {
		return true
	}
	if strings.Contains(label, "<") || strings.Contains(label, ">") {
		return true
	}
	if strings.Contains(label, "<|") || strings.Contains(label, "|>") {
		return true
	}
	lower := strings.ToLower(label)
	for _, blocked := range unsafeSpeakerLabelPhrases() {
		if strings.Contains(lower, blocked) {
			return true
		}
	}
	for _, r := range label {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}

func unsafeSpeakerLabelPhrases() []string {
	return []string{
		"system:",
		"assistant:",
		"developer:",
		"user:",
		"tool:",
		"system prompt",
		"developer message",
		"tool call",
		"ignore previous",
		"ignore prior",
		"ignore earlier",
		"ignore above",
		"ignore all earlier",
		"disregard previous",
		"disregard prior",
		"disregard earlier",
		"disregard above",
		"forget previous",
		"forget prior",
		"forget earlier",
		"reveal prompt",
		"hidden prompt",
		"act as",
		"follow these",
		"new instruction",
		"instructions",
		"instruction",
	}
}
