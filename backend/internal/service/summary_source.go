package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/voxis/backend/internal/domain"
)

const (
	// SummarySourceVersion identifies the compact, citation-addressable source format.
	SummarySourceVersion = "summary-source-v1"

	// MaxSummarySourceChunkBytes matches the current anchored-summary transcript
	// limit. Each chunk is a complete JSONL source package and never ends
	// mid-record, preserving coverage for structured aggregation.
	MaxSummarySourceChunkBytes = 200_000
	// maxSummarySourceChunks stays at five because General Summary budgets up to
	// five paragraphs per chunk: five chunks keep the merged summary within its
	// 25-paragraph validation limit. The 1,000,000-byte serialized limit remains
	// above the legacy 500,000-character request limit.
	maxSummarySourceChunks    = 5
	maxSummarySourceRecords   = 10_000
	maxSourceRecordRunes      = 16_000
	maxSummarySourceTextBytes = MaxSummarySourceChunkBytes * maxSummarySourceChunks
	maxSummarySourceRawBytes  = maxSummarySourceTextBytes * 4
)

// SummarySource is the deterministic source package for one transcription.
// Text is the complete package; Chunks retain full coverage for structured
// aggregation when it cannot fit in one provider request.
type SummarySource struct {
	Version          string
	Language         string
	Segments         []SummarySourceSegment
	Text             string
	Hash             string
	Chunks           []SummarySourceChunk
	DegradationCodes []string
}

// SummarySourceSegment is one citation-addressable source record.
type SummarySourceSegment struct {
	ID           string
	SpeakerID    *string
	Speaker      *string
	StartSeconds *float64
	EndSeconds   *float64
	Text         string
}

// SummarySourceChunk is one complete, bounded JSONL request body.
type SummarySourceChunk struct {
	Index      int
	Text       string
	Hash       string
	SegmentIDs []string
}

type summarySourceHeader struct {
	Version  string  `json:"v"`
	Language *string `json:"lang"`
}

type sourceDraft struct {
	SpeakerID    *string
	Speaker      *string
	StartSeconds *float64
	EndSeconds   *float64
	Text         string
}

// BuildSummarySource creates one escaped JSONL source package for every
// summary type. Confirmed SpeakerMap values are used; suggestions are excluded.
func BuildSummarySource(content *domain.TranscriptionContent, language string) (*SummarySource, error) {
	return buildSummarySource(content, language, MaxSummarySourceChunkBytes)
}

// BuildSummarySourceWithChunkLimit creates the same deterministic source with
// a smaller per-request limit. Editions with a local model context limit use
// it to preserve complete records without sending an oversized prompt.
func BuildSummarySourceWithChunkLimit(
	content *domain.TranscriptionContent,
	language string,
	maxChunkBytes int,
) (*SummarySource, error) {
	if maxChunkBytes <= 0 || maxChunkBytes > MaxSummarySourceChunkBytes {
		return nil, fmt.Errorf("summary source chunk limit must be between 1 and %d bytes: %w", MaxSummarySourceChunkBytes, domain.ErrInvalidInput)
	}
	return buildSummarySource(content, language, maxChunkBytes)
}

func buildSummarySource(content *domain.TranscriptionContent, language string, maxChunkBytes int) (*SummarySource, error) {
	if content == nil {
		return nil, fmt.Errorf("summary source content is required: %w", domain.ErrInvalidInput)
	}
	maxSourceBytes := maxChunkBytes * maxSummarySourceChunks
	if len(content.FullTranscript) > maxSourceBytes {
		return nil, fmt.Errorf("summary source exceeds supported input size")
	}

	attributionFallback := len(content.Utterances) > maxSourceBytes*4
	drafts := fallbackSourceDrafts(strings.TrimSpace(content.FullTranscript))
	if !attributionFallback {
		drafts, attributionFallback = sourceDrafts(content)
	}
	if len(drafts) == 0 {
		return nil, fmt.Errorf("summary source contains no usable transcript text: %w", domain.ErrInvalidInput)
	}

	segments, err := sourceSegments(drafts, maxSourceBytes)
	if err != nil && strings.TrimSpace(content.FullTranscript) != "" && !attributionFallback {
		drafts = fallbackSourceDrafts(strings.TrimSpace(content.FullTranscript))
		attributionFallback = true
		segments, err = sourceSegments(drafts, maxSourceBytes)
	}
	if err != nil {
		return nil, err
	}
	header, lang := summarySourceHeaderLine(language)
	text, chunks, err := serializeSummarySource(header, segments, maxChunkBytes)
	if err != nil {
		return nil, err
	}

	source := &SummarySource{
		Version:  SummarySourceVersion,
		Language: lang,
		Segments: segments,
		Text:     text,
		Hash:     sourceHash(text),
		Chunks:   chunks,
	}
	if attributionFallback {
		source.DegradationCodes = []string{domain.SummaryDegradationSourceAttributionFallback}
	}
	return source, nil
}

func sourceDrafts(content *domain.TranscriptionContent) ([]sourceDraft, bool) {
	drafts, utteranceText := sourceUtteranceDrafts(content.Utterances, content.SpeakerMap)
	fullTranscript := strings.TrimSpace(content.FullTranscript)
	if fullTranscript == "" {
		return drafts, false
	}
	if len(drafts) == 0 || !sameSourceText(utteranceText, fullTranscript) {
		return fallbackSourceDrafts(fullTranscript), len(content.Utterances) != 0
	}
	return drafts, false
}

func sourceUtteranceDrafts(raw json.RawMessage, speakerMap map[string]string) (drafts []sourceDraft, joinedText string) {
	if len(raw) == 0 {
		return nil, ""
	}

	var utterances []providerUtterance
	if err := json.Unmarshal(raw, &utterances); err != nil || len(utterances) == 0 {
		return nil, ""
	}

	drafts = make([]sourceDraft, 0, len(utterances))
	texts := make([]string, 0, len(utterances))
	for _, utterance := range utterances {
		text := resolvedProviderUtteranceText(utterance)
		if text == "" {
			continue
		}
		speakerID, speaker := resolveProviderSpeaker(utterance.Speaker, speakerMap)
		draft := sourceDraft{Text: text}
		if speakerID != "" {
			draft.SpeakerID = stringPointer(speakerID)
			draft.Speaker = stringPointer(speaker)
		}
		if utterance.Start != nil {
			draft.StartSeconds = floatPointer(*utterance.Start)
		}
		if utterance.End != nil {
			draft.EndSeconds = floatPointer(*utterance.End)
		}
		drafts = append(drafts, draft)
		texts = append(texts, text)
	}
	return drafts, strings.Join(texts, " ")
}

func fallbackSourceDrafts(transcript string) []sourceDraft {
	paragraphs := splitTranscriptParagraphs(transcript)
	drafts := make([]sourceDraft, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if text := strings.TrimSpace(paragraph); text != "" {
			drafts = append(drafts, sourceDraft{Text: text})
		}
	}
	return drafts
}

func sourceSegments(drafts []sourceDraft, maxSourceBytes int) ([]SummarySourceSegment, error) {
	segments := make([]SummarySourceSegment, 0, len(drafts))
	totalTextBytes := 0
	for _, draft := range drafts {
		totalTextBytes += len(draft.Text)
		if totalTextBytes > maxSourceBytes {
			return nil, fmt.Errorf("summary source exceeds %d bytes", maxSourceBytes)
		}
		parts := splitSourceRecordText(draft.Text)
		for _, text := range parts {
			if len(segments) >= maxSummarySourceRecords {
				return nil, fmt.Errorf("summary source exceeds %d records", maxSummarySourceRecords)
			}
			segments = append(segments, SummarySourceSegment{
				ID:           fmt.Sprintf("u%06d", len(segments)+1),
				SpeakerID:    draft.SpeakerID,
				Speaker:      draft.Speaker,
				StartSeconds: draft.StartSeconds,
				EndSeconds:   draft.EndSeconds,
				Text:         text,
			})
		}
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("summary source contains no usable transcript text: %w", domain.ErrInvalidInput)
	}
	return segments, nil
}

func splitSourceRecordText(text string) []string {
	runes := []rune(text)
	if len(runes) <= maxSourceRecordRunes {
		return []string{text}
	}

	parts := make([]string, 0, len(runes)/maxSourceRecordRunes+1)
	for start := 0; start < len(runes); {
		end := min(start+maxSourceRecordRunes, len(runes))
		if end < len(runes) {
			end = sourceRecordBoundary(runes, start, end)
		}
		parts = append(parts, string(runes[start:end]))
		start = end
	}
	return parts
}

func sourceRecordBoundary(runes []rune, start, end int) int {
	for i := end; i > start; i-- {
		if unicode.IsSpace(runes[i-1]) {
			return i
		}
	}
	return end
}

func summarySourceHeaderLine(language string) (headerLine, resolvedLanguage string) {
	language = strings.TrimSpace(language)
	var lang *string
	if language != "" && language != "auto" {
		lang = &language
	}
	header, err := json.Marshal(summarySourceHeader{Version: SummarySourceVersion, Language: lang})
	if err != nil {
		return `{"v":"summary-source-v1","lang":null}`, ""
	}
	if lang == nil {
		return string(header), ""
	}
	return string(header), *lang
}

func serializeSummarySource(header string, segments []SummarySourceSegment, maxChunkBytes int) (string, []SummarySourceChunk, error) {
	var full strings.Builder
	full.Grow(len(header) + len(segments)*64)
	full.WriteString(header)

	chunk := strings.Builder{}
	chunk.Grow(maxChunkBytes)
	chunk.WriteString(header)
	chunkIDs := make([]string, 0, len(segments))
	chunks := make([]SummarySourceChunk, 0, 1)

	for _, segment := range segments {
		line, err := summarySourceTupleLine(segment)
		if err != nil {
			return "", nil, err
		}
		if len(header)+1+len(line) > maxChunkBytes {
			return "", nil, fmt.Errorf("summary source record %s exceeds %d bytes", segment.ID, maxChunkBytes)
		}
		if chunk.Len()+1+len(line) > maxChunkBytes {
			chunks = append(chunks, completeSummarySourceChunk(len(chunks), chunk.String(), chunkIDs))
			if len(chunks) >= maxSummarySourceChunks {
				return "", nil, fmt.Errorf("summary source exceeds %d chunks", maxSummarySourceChunks)
			}
			chunk.Reset()
			chunk.Grow(maxChunkBytes)
			chunk.WriteString(header)
			chunkIDs = make([]string, 0, len(segments))
		}
		full.WriteByte('\n')
		full.WriteString(line)
		chunk.WriteByte('\n')
		chunk.WriteString(line)
		chunkIDs = append(chunkIDs, segment.ID)
	}

	chunks = append(chunks, completeSummarySourceChunk(len(chunks), chunk.String(), chunkIDs))
	return full.String(), chunks, nil
}

func summarySourceTupleLine(segment SummarySourceSegment) (string, error) {
	tuple := [6]any{
		segment.ID,
		segment.SpeakerID,
		segment.Speaker,
		segment.StartSeconds,
		segment.EndSeconds,
		segment.Text,
	}
	line, err := json.Marshal(tuple)
	if err != nil {
		return "", fmt.Errorf("serialize summary source record %s: %w", segment.ID, err)
	}
	return string(line), nil
}

func completeSummarySourceChunk(index int, text string, segmentIDs []string) SummarySourceChunk {
	ids := append([]string(nil), segmentIDs...)
	return SummarySourceChunk{
		Index:      index,
		Text:       text,
		Hash:       sourceHash(text),
		SegmentIDs: ids,
	}
}

func sameSourceText(left, right string) bool {
	return strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ")
}

func sourceHash(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func stringPointer(value string) *string { return &value }

func floatPointer(value float64) *float64 { return &value }
