package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/voxis/backend/internal/domain"
)

const maxTranscriptSegmentTextChars = 2000

// TranscriptSegment is a bounded, timestamp-aware transcript excerpt.
type TranscriptSegment struct {
	Speaker      string
	SpeakerID    string
	StartSeconds float64
	EndSeconds   float64
	HasStart     bool
	HasEnd       bool
	Text         string
}

type providerUtterance struct {
	Speaker any      `json:"speaker"`
	Text    string   `json:"text"`
	Start   *float64 `json:"start"`
	End     *float64 `json:"end"`
	Words   []struct {
		Word string `json:"word"`
	} `json:"words"`
}

// ParseTranscriptSegments converts provider utterances into bounded transcript
// segments. Empty utterances fall back to paragraph-like full transcript chunks.
func ParseTranscriptSegments(raw []byte, fullTranscript string, speakerMap map[string]string) ([]TranscriptSegment, error) {
	if len(raw) == 0 {
		return fallbackTranscriptSegments(fullTranscript), nil
	}

	var utterances []providerUtterance
	if err := json.Unmarshal(raw, &utterances); err != nil {
		return nil, fmt.Errorf("parse transcript utterances: %w", domain.ErrInvalidInput)
	}
	if len(utterances) == 0 {
		return fallbackTranscriptSegments(fullTranscript), nil
	}

	segments := make([]TranscriptSegment, 0, len(utterances))
	for _, u := range utterances {
		if u.Start == nil || u.End == nil {
			continue
		}
		text := boundSegmentText(resolvedProviderUtteranceText(u))
		if text == "" {
			continue
		}
		speakerID, speaker := resolveProviderSpeaker(u.Speaker, speakerMap)
		segments = append(segments, TranscriptSegment{
			Speaker:      speaker,
			SpeakerID:    speakerID,
			StartSeconds: *u.Start,
			EndSeconds:   *u.End,
			HasStart:     true,
			HasEnd:       true,
			Text:         text,
		})
	}
	if len(segments) == 0 {
		return fallbackTranscriptSegments(fullTranscript), nil
	}
	return segments, nil
}

func fallbackTranscriptSegments(fullTranscript string) []TranscriptSegment {
	fullTranscript = strings.TrimSpace(fullTranscript)
	if fullTranscript == "" {
		return nil
	}

	parts := splitTranscriptParagraphs(fullTranscript)
	segments := make([]TranscriptSegment, 0, len(parts))
	for _, part := range parts {
		text := boundSegmentText(strings.TrimSpace(part))
		if text == "" {
			continue
		}
		segments = append(segments, TranscriptSegment{Text: text})
	}
	return segments
}

func splitTranscriptParagraphs(text string) []string {
	rawParts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return []string{text}
	}
	return parts
}

func resolveProviderSpeaker(raw any, speakerMap map[string]string) (speakerID, label string) {
	key := speakerKey(raw)
	if key == "" {
		return "", ""
	}
	speakerID = "speaker-" + key
	if mapped := strings.TrimSpace(speakerMap[key]); mapped != "" {
		return speakerID, mapped
	}
	if n, err := strconv.Atoi(key); err == nil && n >= 0 {
		return speakerID, domain.DefaultSpeakerLabel(n)
	}
	return speakerID, key
}

func resolvedProviderUtteranceText(u providerUtterance) string {
	if text := strings.TrimSpace(u.Text); text != "" {
		return text
	}

	words := make([]string, 0, len(u.Words))
	for _, word := range u.Words {
		text := strings.TrimSpace(word.Word)
		if text != "" {
			words = append(words, text)
		}
	}
	return strings.Join(words, " ")
}

func speakerKey(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func boundSegmentText(text string) string {
	runes := []rune(text)
	if len(runes) <= maxTranscriptSegmentTextChars {
		return text
	}
	return string(runes[:maxTranscriptSegmentTextChars])
}
