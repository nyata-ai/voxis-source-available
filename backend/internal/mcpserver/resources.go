package mcpserver

import (
	"context"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerResources adds static MCP resources to the server.
// These provide documentation and schema information to MCP clients.
// toolNames is the list of tools actually registered on this server, so the
// resources can never advertise a tool that does not exist.
func registerResources(server *mcp.Server, toolNames []string) {
	server.AddResource(
		&mcp.Resource{
			URI:         "voxis://api/info",
			Name:        "Voxis API Information",
			Description: "Overview of the Voxis API capabilities and available tools.",
			MIMEType:    "text/plain",
		},
		func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						URI:      "voxis://api/info",
						MIMEType: "text/plain",
						Text:     buildAPIInfoText(toolNames),
					},
				},
			}, nil
		},
	)
	server.AddResource(
		&mcp.Resource{
			URI:         "voxis://api/capabilities",
			Name:        "Voxis MCP Capability Catalog",
			Description: "Machine-readable MCP tool capability groups, scopes, and workflow guidance.",
			MIMEType:    "application/json",
		},
		func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			text, err := buildCapabilityCatalogJSON(toolNames)
			if err != nil {
				return nil, err
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						URI:      "voxis://api/capabilities",
						MIMEType: "application/json",
						Text:     text,
					},
				},
			}, nil
		},
	)
}

// buildAPIInfoText composes the /info resource body from the registered tool
// names so the list of tools can never drift from the registered set.
func buildAPIInfoText(toolNames []string) string {
	catalog := buildMCPToolCatalog()
	var b strings.Builder
	b.WriteString("Voxis Transcription Platform - MCP API\n\n")
	b.WriteString("Voxis exposes organization-scoped tools for secure audio ingestion, transcription, transcript search, citation-grounded answers, summaries, professional briefs, collections, and exports.\n\n")
	b.WriteString("Capability groups:\n")
	for _, group := range registeredCapabilityGroups(toolNames) {
		b.WriteString("- ")
		b.WriteString(group.Name)
		b.WriteString(": ")
		b.WriteString(group.Description)
		b.WriteString("\n")
	}
	b.WriteString("Common wrapper workflows:\n")
	for _, workflow := range registeredWorkflows(toolNames) {
		b.WriteString("- ")
		b.WriteString(workflow.Name)
		b.WriteString(": ")
		b.WriteString(strings.Join(workflow.Steps, " -> "))
		b.WriteString("\n")
	}
	b.WriteString("Scope model:\n")
	b.WriteString("Request the smallest scope set required by the intended workflow. Scope checks fail closed: a token that carries no Voxis scopes is denied for every scope-gated tool. Tool-specific scopes are listed below and in voxis://api/capabilities.\n\n")
	b.WriteString("Available tools (")
	b.WriteString(strconv.Itoa(len(toolNames)))
	b.WriteString("):\n")
	for _, name := range toolNames {
		spec := catalog[name]
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(" (")
		b.WriteString(spec.Title)
		b.WriteString("): ")
		b.WriteString(spec.WhenToUse)
		b.WriteString(" Required scopes: ")
		b.WriteString(formatScopes(spec.RequiredScopes))
		b.WriteString(".")
		if spec.ScopeNote != "" {
			b.WriteString(" ")
			b.WriteString(spec.ScopeNote)
		}
		for _, limitation := range spec.Limitations {
			b.WriteString(" ")
			b.WriteString(limitation)
		}
		b.WriteString("\n")
	}

	b.WriteString("\nSafety and limits:\n")
	b.WriteString("- All operations are scoped to the authenticated organization.\n")
	b.WriteString("- Use cited segment, transcript Q&A, collection Q&A, and role brief tools for user-facing answers from transcript content.\n")
	b.WriteString("- Audio bytes are uploaded through REST multipart after get_media_upload_instructions; MCP does not carry audio bytes.\n")
	b.WriteString("- Semantic search is unavailable until an opt-in encrypted semantic index is configured.\n")
	b.WriteString("- Redaction is deterministic mitigation, not a legal guarantee.\n")
	b.WriteString("- Authentication is via API key (Bearer vxs_...) or JWT token.")
	return b.String()
}
