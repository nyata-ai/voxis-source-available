package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// TruncateResult holds the result of truncating a transcript.
type TruncateResult struct {
	Text              string
	Truncated         bool
	TotalWordCount    int
	ReturnedWordCount int
}

// TruncateTranscript truncates content to maxWords on word boundaries.
// It preserves the original whitespace between words up to the truncation point.
// Words are counted using Unicode whitespace splitting (matching strings.Fields).
func TruncateTranscript(content string, maxWords int) TruncateResult {
	if content == "" {
		return TruncateResult{}
	}

	// Count all words using Fields (splits on any whitespace).
	allWords := strings.Fields(content)
	totalCount := len(allWords)

	if totalCount == 0 {
		return TruncateResult{}
	}

	if totalCount <= maxWords {
		return TruncateResult{
			Text:              content,
			Truncated:         false,
			TotalWordCount:    totalCount,
			ReturnedWordCount: totalCount,
		}
	}

	// Walk through content preserving original whitespace, counting words.
	// We need to find the byte position after exactly maxWords words.
	// Use proper UTF-8 rune decoding to handle multi-byte characters.
	wordsSeen := 0
	i := 0

	// Skip leading whitespace.
	for i < len(content) {
		r, size := utf8.DecodeRuneInString(content[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i += size
	}

	for wordsSeen < maxWords && i < len(content) {
		// We're at the start of a word. Find the end.
		for i < len(content) {
			r, size := utf8.DecodeRuneInString(content[i:])
			if unicode.IsSpace(r) {
				break
			}
			i += size
		}
		wordsSeen++

		if wordsSeen == maxWords {
			break
		}

		// Skip whitespace between words.
		for i < len(content) {
			r, size := utf8.DecodeRuneInString(content[i:])
			if !unicode.IsSpace(r) {
				break
			}
			i += size
		}
	}

	return TruncateResult{
		Text:              content[:i],
		Truncated:         true,
		TotalWordCount:    totalCount,
		ReturnedWordCount: maxWords,
	}
}
