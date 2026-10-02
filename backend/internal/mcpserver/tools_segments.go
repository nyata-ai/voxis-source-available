package mcpserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

const (
	defaultSegmentLimit = 20
	maxSegmentLimit     = 100
)

// MCPCitation is the evidence shape returned by transcript-aware MCP tools.
type MCPCitation struct {
	TranscriptionID string  `json:"transcription_id"`
	MediaID         string  `json:"media_id,omitempty"`
	SourceType      string  `json:"source_type"`
	DisplayName     string  `json:"display_name"`
	Speaker         string  `json:"speaker,omitempty"`
	StartSeconds    float64 `json:"start_seconds"`
	EndSeconds      float64 `json:"end_seconds"`
	Text            string  `json:"text"`
}

// GetTranscriptSegmentsInput is the input for the get_transcript_segments tool.
type GetTranscriptSegmentsInput struct {
	TranscriptionID string   `json:"transcription_id" jsonschema:"The UUID of the transcription"`
	StartSeconds    *float64 `json:"start_seconds,omitempty" jsonschema:"Optional start timestamp in seconds"`
	EndSeconds      *float64 `json:"end_seconds,omitempty" jsonschema:"Optional end timestamp in seconds"`
	Speaker         string   `json:"speaker,omitempty" jsonschema:"Optional speaker name or index"`
	MaxSegments     int      `json:"max_segments,omitempty" jsonschema:"Maximum segments to return (default 20, max 100)"`
}

func handleGetTranscriptSegments(
	ctx context.Context,
	deps Dependencies,
	input GetTranscriptSegmentsInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "transcription:read"); denied != nil {
		return denied, nil, nil
	}
	if err := validateSegmentRange(input); err != nil {
		return validationError(ctx, err), nil, nil
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	trans, err := deps.TranscriptionService.GetByID(ctx, orgID, input.TranscriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "get_transcript_segments", "get transcription", err), nil, nil
	}

	segments, err := service.ParseTranscriptSegments(trans.Utterances, trans.FullTranscript, trans.SpeakerMap)
	if err != nil {
		return validationError(ctx, err), nil, nil
	}

	limit := clampSegmentLimit(input.MaxSegments)
	filtered := filterTranscriptSegments(segments, input)
	truncated := len(filtered) > limit
	if truncated {
		filtered = filtered[:limit]
	}

	result := struct {
		Items     []MCPCitation `json:"items"`
		Count     int           `json:"count"`
		Truncated bool          `json:"truncated"`
	}{
		Items:     make([]MCPCitation, len(filtered)),
		Count:     len(filtered),
		Truncated: truncated,
	}
	for i, seg := range filtered {
		result.Items[i] = citationFromSegment(trans, seg)
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func validateSegmentRange(input GetTranscriptSegmentsInput) error {
	if input.StartSeconds != nil {
		if err := validateFloatRange(*input.StartSeconds, 0, 86400, "start_seconds"); err != nil {
			return err
		}
	}
	if input.EndSeconds != nil {
		if err := validateFloatRange(*input.EndSeconds, 0, 86400, "end_seconds"); err != nil {
			return err
		}
	}
	if input.StartSeconds != nil && input.EndSeconds != nil && *input.EndSeconds < *input.StartSeconds {
		return fmt.Errorf("end_seconds must be greater than or equal to start_seconds")
	}
	return nil
}

func clampSegmentLimit(limit int) int {
	if limit <= 0 {
		return defaultSegmentLimit
	}
	if limit > maxSegmentLimit {
		return maxSegmentLimit
	}
	return limit
}

func filterTranscriptSegments(segments []service.TranscriptSegment, input GetTranscriptSegmentsInput) []service.TranscriptSegment {
	filtered := make([]service.TranscriptSegment, 0, len(segments))
	for _, seg := range segments {
		if input.StartSeconds != nil && seg.EndSeconds < *input.StartSeconds {
			continue
		}
		if input.EndSeconds != nil && seg.StartSeconds > *input.EndSeconds {
			continue
		}
		if input.Speaker != "" && !speakerMatches(input.Speaker, seg.Speaker) {
			continue
		}
		filtered = append(filtered, seg)
	}
	return filtered
}

func speakerMatches(filter, speaker string) bool {
	filter = strings.TrimSpace(filter)
	if strings.EqualFold(filter, speaker) {
		return true
	}
	n, err := strconv.Atoi(filter)
	if err != nil || n < 0 {
		return false
	}
	return strings.EqualFold(domain.DefaultSpeakerLabel(n), speaker)
}

func citationFromSegment(trans *domain.Transcription, seg service.TranscriptSegment) MCPCitation {
	return MCPCitation{
		TranscriptionID: trans.ID,
		MediaID:         trans.MediaID,
		SourceType:      "media",
		DisplayName:     transcriptionDisplayName(trans),
		Speaker:         seg.Speaker,
		StartSeconds:    seg.StartSeconds,
		EndSeconds:      seg.EndSeconds,
		Text:            seg.Text,
	}
}

func transcriptionDisplayName(trans *domain.Transcription) string {
	switch {
	case strings.TrimSpace(trans.MediaTitle) != "":
		return trans.MediaTitle
	case strings.TrimSpace(trans.MediaFilename) != "":
		return trans.MediaFilename
	default:
		return trans.ID
	}
}
