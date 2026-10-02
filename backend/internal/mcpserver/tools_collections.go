package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/port"
)

type listCollectionsInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum number of collections to return (default 20, max 100)"`
	Offset int `json:"offset,omitempty" jsonschema:"Number of collections to skip for pagination"`
}

type createCollectionInput struct {
	Name        string `json:"name" jsonschema:"Collection name, max 120 characters"`
	Description string `json:"description,omitempty" jsonschema:"Optional collection description, max 1000 characters"`
}

type collectionItemInput struct {
	CollectionID    string `json:"collection_id" jsonschema:"The UUID of the collection"`
	TranscriptionID string `json:"transcription_id" jsonschema:"The UUID of the transcription"`
}

type askCollectionInput struct {
	CollectionID string `json:"collection_id" jsonschema:"The UUID of the collection"`
	Question     string `json:"question" jsonschema:"Question to answer from collection transcript evidence, max 1000 chars"`
	MaxSegments  int    `json:"max_segments,omitempty" jsonschema:"Evidence segment cap (default 20, max 60)"`
}

func registerCollectionTools(server *toolRegistry, deps Dependencies) {
	registerTool(server, &mcp.Tool{
		Name:        "list_collections",
		Description: "List MCP transcript collections in the authenticated organization.",
	}, wrapTool[listCollectionsInput, any]("list_collections", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input listCollectionsInput) (*mcp.CallToolResult, any, error) {
			return handleListCollections(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "create_collection",
		Description: "Create an MCP transcript collection for grouping transcripts.",
	}, wrapTool[createCollectionInput, any]("create_collection", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input createCollectionInput) (*mcp.CallToolResult, any, error) {
			return handleCreateCollection(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "add_to_collection",
		Description: "Add a transcription to an MCP collection. Idempotent.",
	}, wrapTool[collectionItemInput, any]("add_to_collection", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input collectionItemInput) (*mcp.CallToolResult, any, error) {
			return handleAddToCollection(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "remove_from_collection",
		Description: "Remove a transcription from an MCP collection. Idempotent.",
	}, wrapTool[collectionItemInput, any]("remove_from_collection", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input collectionItemInput) (*mcp.CallToolResult, any, error) {
			return handleRemoveFromCollection(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "ask_collection",
		Description: "Answer a question using only bounded, cited transcript evidence from a collection.",
	}, wrapTool[askCollectionInput, any]("ask_collection", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input askCollectionInput) (*mcp.CallToolResult, any, error) {
			return handleAskCollection(ctx, deps, input)
		}))
}

func handleListCollections(ctx context.Context, deps Dependencies, input listCollectionsInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "collection:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	items, err := deps.MCPService.ListCollections(ctx, orgID, input.Limit, clampOffset(input.Offset))
	if err != nil {
		return toolError(ctx, deps.Logger, "list_collections", "list collections", err), nil, nil
	}
	views := make([]collectionView, len(items))
	for i, item := range items {
		views[i] = collectionToView(item)
	}
	r, jErr := jsonResult(struct {
		Items []collectionView `json:"items"`
		Count int              `json:"count"`
	}{Items: views, Count: len(views)})
	return r, nil, jErr
}

func handleCreateCollection(ctx context.Context, deps Dependencies, input createCollectionInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "collection:write"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	collection, err := deps.MCPService.CreateCollection(ctx, orgID, input.Name, input.Description, subjectFromContext(ctx))
	if err != nil {
		return toolError(ctx, deps.Logger, "create_collection", "create collection", err), nil, nil
	}
	r, jErr := jsonResult(collectionToView(*collection))
	return r, nil, jErr
}

func handleAddToCollection(ctx context.Context, deps Dependencies, input collectionItemInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScopes(ctx, []string{"collection:write", "transcription:read"}); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := deps.MCPService.AddCollectionItem(ctx, orgID, input.CollectionID, input.TranscriptionID); err != nil {
		return toolError(ctx, deps.Logger, "add_to_collection", "add to collection", err), nil, nil
	}
	return textResult("transcription added to collection"), nil, nil
}

func handleRemoveFromCollection(ctx context.Context, deps Dependencies, input collectionItemInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "collection:write"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := deps.MCPService.RemoveCollectionItem(ctx, orgID, input.CollectionID, input.TranscriptionID); err != nil {
		return toolError(ctx, deps.Logger, "remove_from_collection", "remove from collection", err), nil, nil
	}
	return textResult("transcription removed from collection"), nil, nil
}

func handleAskCollection(ctx context.Context, deps Dependencies, input askCollectionInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScopes(ctx, []string{"collection:read", "transcription:read", "analysis:write"}); denied != nil {
		return denied, nil, nil
	}
	if input.Question == "" {
		return validationError(ctx, fmt.Errorf("question cannot be empty")), nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := deps.MCPService.AskCollection(ctx, orgID, port.AskCollectionRequest{
		CollectionID: input.CollectionID,
		Question:     input.Question,
		MaxSegments:  input.MaxSegments,
	})
	if err != nil {
		return toolError(ctx, deps.Logger, "ask_collection", "ask collection", err), nil, nil
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

type collectionView struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ItemCount      int    `json:"item_count"`
}

func collectionToView(item port.MCPCollection) collectionView {
	return collectionView{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		Name:           item.Name,
		Description:    item.Description,
		CreatedBy:      item.CreatedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
		ItemCount:      item.ItemCount,
	}
}
