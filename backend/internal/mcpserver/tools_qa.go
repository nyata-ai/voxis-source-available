package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// AskTranscriptInput is the input for the ask_transcript tool.
type AskTranscriptInput struct {
	TranscriptionID string `json:"transcription_id" jsonschema:"The UUID of the transcription"`
	Question        string `json:"question" jsonschema:"Question to answer from cited transcript evidence, max 1000 chars"`
	MaxSegments     int    `json:"max_segments,omitempty" jsonschema:"Evidence segment cap (default 20, max 60)"`
}

func handleAskTranscript(
	ctx context.Context,
	deps Dependencies,
	input AskTranscriptInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScopes(ctx, []string{"transcription:read", "analysis:write"}); denied != nil {
		return denied, nil, nil
	}
	if input.Question == "" {
		return validationError(ctx, fmt.Errorf("question cannot be empty")), nil, nil
	}
	if len([]rune(input.Question)) > 1000 {
		return validationError(ctx, fmt.Errorf("question cannot exceed 1000 characters")), nil, nil
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if deps.TranscriptAnswerer == nil {
		return toolError(ctx, deps.Logger, "ask_transcript", "ask transcript", fmt.Errorf("answerer missing: %w", domain.ErrInvalidInput)), nil, nil
	}

	qa := service.NewTranscriptQAService(deps.TranscriptionService, deps.TranscriptAnswerer)
	result, err := qa.AskTranscript(ctx, orgID, port.AskTranscriptRequest{
		TranscriptionID: input.TranscriptionID,
		Question:        input.Question,
		MaxSegments:     input.MaxSegments,
	})
	if err != nil {
		return toolError(ctx, deps.Logger, "ask_transcript", "ask transcript", err), nil, nil
	}

	output := struct {
		Answer        string        `json:"answer"`
		Citations     []MCPCitation `json:"citations"`
		EvidenceCount int           `json:"evidence_count"`
		Refusal       bool          `json:"refusal"`
	}{
		Answer:        result.Answer,
		Citations:     citationsFromEvidence(result.Citations),
		EvidenceCount: result.EvidenceCount,
		Refusal:       result.Refusal,
	}

	r, jErr := jsonResult(output)
	return r, nil, jErr
}

func citationsFromEvidence(evidence []port.TranscriptEvidence) []MCPCitation {
	citations := make([]MCPCitation, len(evidence))
	for i, item := range evidence {
		citations[i] = MCPCitation{
			TranscriptionID: item.TranscriptionID,
			MediaID:         item.MediaID,
			SourceType:      item.SourceType,
			DisplayName:     item.DisplayName,
			Speaker:         item.Speaker,
			StartSeconds:    item.StartSeconds,
			EndSeconds:      item.EndSeconds,
			Text:            item.Text,
		}
	}
	return citations
}
