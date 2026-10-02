package domain

import (
	"encoding/json"
	"strings"
)

// CountSpeakersAndWords parses utterances JSON to count unique speakers and total words.
// Prefers explicit words array if present; falls back to counting words in text.
func CountSpeakersAndWords(utterancesJSON json.RawMessage) (speakerCount, wordCount int) {
	if len(utterancesJSON) == 0 {
		return 0, 0
	}

	type utteranceEntry struct {
		Speaker int `json:"speaker"`
		Words   []struct {
			Word string `json:"word"`
		} `json:"words"`
		Text string `json:"text"`
	}

	var utterances []utteranceEntry
	if err := json.Unmarshal(utterancesJSON, &utterances); err != nil {
		return 0, 0
	}

	speakers := make(map[int]struct{})
	totalWords := 0

	for _, u := range utterances {
		speakers[u.Speaker] = struct{}{}

		if len(u.Words) > 0 {
			totalWords += len(u.Words)
		} else if u.Text != "" {
			totalWords += len(strings.Fields(u.Text))
		}
	}

	return len(speakers), totalWords
}
