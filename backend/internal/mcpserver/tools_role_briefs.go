package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/port"
)

type roleBriefInput struct {
	TranscriptionID string `json:"transcription_id,omitempty" jsonschema:"Optional transcription UUID source"`
	CollectionID    string `json:"collection_id,omitempty" jsonschema:"Optional collection UUID source"`
	MaxSegments     int    `json:"max_segments,omitempty" jsonschema:"Evidence segment cap (default 20, max 60)"`
}

func registerRoleBriefTools(server *toolRegistry, deps Dependencies) {
	addRoleBriefTool(server, deps, "build_story_brief", "journalist", "Build a journalist story brief from cited transcript evidence.")
	addRoleBriefTool(server, deps, "create_case_timeline", "law_enforcement", "Create a law enforcement case timeline from cited transcript evidence.")
	addRoleBriefTool(server, deps, "build_diligence_memo", "investment_banker", "Build an investment banking diligence memo from cited transcript evidence.")
	addRoleBriefTool(server, deps, "create_study_guide", "student", "Create a student study guide from cited transcript evidence.")
}

func addRoleBriefTool(server *toolRegistry, deps Dependencies, name, role, description string) {
	registerTool(server, &mcp.Tool{Name: name, Description: description},
		wrapTool[roleBriefInput, any](name, heavyToolTimeout, deps,
			func(ctx context.Context, _ *mcp.CallToolRequest, input roleBriefInput) (*mcp.CallToolResult, any, error) {
				return handleRoleBrief(ctx, deps, name, role, input)
			}))
}

func handleRoleBrief(
	ctx context.Context,
	deps Dependencies,
	toolName, role string,
	input roleBriefInput,
) (*mcp.CallToolResult, any, error) {
	required := []string{"analysis:write", "transcription:read"}
	if input.CollectionID != "" {
		required = append(required, "collection:read")
	}
	if denied := checkScopes(ctx, required); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := deps.MCPService.BuildRoleBrief(ctx, orgID, port.RoleBriefRequest{
		Role:            role,
		TranscriptionID: input.TranscriptionID,
		CollectionID:    input.CollectionID,
		MaxSegments:     input.MaxSegments,
	})
	if err != nil {
		return toolError(ctx, deps.Logger, toolName, "build role brief", err), nil, nil
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
