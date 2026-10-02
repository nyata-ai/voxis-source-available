package mcpserver

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registeredToolNames(t *testing.T) []string {
	t.Helper()
	tools := &toolRegistry{server: mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)}
	registerTools(tools, Dependencies{})
	require.NotEmpty(t, tools.names)
	return tools.names
}

func TestCatalogMatchesRegisteredTools(t *testing.T) {
	names := registeredToolNames(t)
	registered := make(map[string]bool, len(names))
	for _, name := range names {
		require.False(t, registered[name], "tool %q registered twice", name)
		registered[name] = true
	}

	catalog := buildMCPToolCatalog()
	for _, name := range names {
		assert.Contains(t, catalog, name, "registered tool %q has no catalog entry", name)
	}
	for name := range catalog {
		assert.True(t, registered[name], "catalog advertises unregistered tool %q", name)
	}
}

func TestCapabilityResourceListsOnlyRegisteredTools(t *testing.T) {
	names := registeredToolNames(t)
	registered := make(map[string]bool, len(names))
	for _, name := range names {
		registered[name] = true
	}

	text, err := buildCapabilityCatalogJSON(names)
	require.NoError(t, err)
	var payload struct {
		ToolCount    int                        `json:"tool_count"`
		Tools        map[string]json.RawMessage `json:"tools"`
		Capabilities []capabilityGroup          `json:"capabilities"`
		Workflows    []wrapperWorkflow          `json:"workflows"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &payload))

	assert.Equal(t, len(names), payload.ToolCount)
	assert.Len(t, payload.Tools, len(names))
	for name := range payload.Tools {
		assert.True(t, registered[name], "capability resource lists unregistered tool %q", name)
	}
	for _, workflow := range payload.Workflows {
		for _, step := range workflow.Steps {
			assert.True(t, strings.HasPrefix(step, "REST ") || registered[step],
				"workflow %q uses unregistered tool %q", workflow.Name, step)
		}
	}
	assert.NotEmpty(t, payload.Workflows)
	assert.NotEmpty(t, payload.Capabilities)
	for _, removed := range []string{"create_url_transcription", "get_usage_stats", "retry_transcription"} {
		assert.NotContains(t, text, removed)
	}
}

func TestAPIInfoResourceListsOnlyRegisteredTools(t *testing.T) {
	names := registeredToolNames(t)

	text := buildAPIInfoText(names)

	assert.Contains(t, text, "Available tools ("+strconv.Itoa(len(names))+")")
	for _, name := range names {
		assert.Contains(t, text, "- "+name+" (")
	}
	for _, removed := range []string{"create_url_transcription", "get_usage_stats", "list_failed_jobs"} {
		assert.NotContains(t, text, removed)
	}
}

func TestRegisteredWorkflowsDropsWorkflowsWithMissingTools(t *testing.T) {
	workflows := registeredWorkflows([]string{"get_media_upload_instructions", "create_transcription", "get_transcription_detail"})

	require.Len(t, workflows, 1)
	assert.Equal(t, "Upload audio", workflows[0].Name)
}
