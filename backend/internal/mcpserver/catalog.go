package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const capabilityCatalogVersion = "2026-06-06"

type mcpToolSpec struct {
	Title          string               `json:"title"`
	Capability     string               `json:"capability"`
	Description    string               `json:"description"`
	WhenToUse      string               `json:"when_to_use"`
	RequiredScopes []string             `json:"required_scopes,omitempty"`
	ScopeNote      string               `json:"scope_note,omitempty"`
	FollowUps      []string             `json:"follow_up_tools,omitempty"`
	Limitations    []string             `json:"limitations,omitempty"`
	Annotations    *mcp.ToolAnnotations `json:"annotations"`
}

type capabilityGroup struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type wrapperWorkflow struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

var mcpCapabilityGroups = []capabilityGroup{
	{"Ingest audio", "Upload local audio through the REST multipart endpoint."},
	{"Manage media", "List, inspect, update, and remove organization-scoped media records."},
	{"Transcribe and review", "Create transcription jobs, inspect status, retrieve transcript text, and manage speaker labels."},
	{"Search and cite evidence", "Search across media and transcript segments while returning bounded cited snippets."},
	{"Summarize and brief", "Create summaries and build citation-grounded professional briefs."},
	{"Organize collections", "Group transcripts for repeated analysis and collection-level Q&A."},
	{"Export safely", "Export transcript and summary content, including deterministic redacted JSON exports."},
	{"Monitor activity", "Fetch recent transcription activity and missing-summary work queues."},
}

var mcpWrapperWorkflows = []wrapperWorkflow{
	{"Upload audio", []string{"get_media_upload_instructions", "REST POST /api/v1/media/upload", "create_transcription", "get_transcription_detail"}},
	{"Answer with citations", []string{"search_all", "search_transcript_segments", "ask_transcript"}},
	{"Prepare a professional brief", []string{"list_collections", "add_to_collection", "ask_collection", "build_story_brief"}},
	{"Export for sharing", []string{"get_transcription_detail", "create_redacted_export", "export_document"}},
}

func buildMCPToolCatalog() map[string]mcpToolSpec {
	return map[string]mcpToolSpec{
		"list_media":                            spec("List Media", "Manage media", "List audio media files with filters for date, duration, status, search, and sort.", "Use before choosing media to inspect, transcribe, update, or delete.", []string{"media:read"}, true, false, false, false, "", nil, []string{"get_media", "create_transcription"}),
		"get_media":                             spec("Get Media", "Manage media", "Get metadata for one media file, including scan status and storage-facing details.", "Use when a wrapper already has a media_id and needs safe metadata before acting.", []string{"media:read"}, true, false, false, false, "", nil, []string{"create_transcription"}),
		"delete_media":                          spec("Delete Media", "Manage media", "Permanently delete one media file, its stored audio, and its transcriptions and summaries.", "Use only after explicit user confirmation because this is destructive.", deleteMediaScopes, false, true, false, false, "", nil, nil),
		"update_media":                          spec("Update Media Metadata", "Manage media", "Update a media title or description without changing audio content.", "Use to rename or annotate a media record after user intent is clear.", []string{"media:write"}, false, false, true, false, "", nil, []string{"get_media"}),
		"get_media_upload_instructions":         spec("Get Media Upload Instructions", "Ingest audio", "Return the REST multipart upload contract for the secure Voxis upload pipeline.", "Use for local audio uploads; do not send binary audio bytes through MCP.", []string{"media:write"}, true, false, false, false, "", nil, []string{"create_transcription"}),
		"list_transcriptions":                   spec("List Transcriptions", "Transcribe and review", "List transcriptions with language, speaker, status, duration, word count, and search filters.", "Use to find transcripts before reading details, segments, summaries, or exports.", []string{"transcription:read"}, true, false, false, false, "", nil, []string{"get_transcription_detail", "get_transcript_segments"}),
		"get_transcription":                     spec("Get Transcription", "Transcribe and review", "Get one transcription, including transcript text when completed and optional word-bounded truncation.", "Use when full transcript text is needed and bounded segment evidence is not enough.", []string{"transcription:read"}, true, false, false, false, "", nil, []string{"ask_transcript", "export_document"}),
		"get_transcript_segments":               spec("Get Transcript Segments", "Search and cite evidence", "Get bounded timestamped transcript segments with source and speaker citations.", "Use for citation-first reading, time-window review, and transcript excerpts.", []string{"transcription:read"}, true, false, false, false, "", nil, []string{"ask_transcript"}),
		"list_transcription_speakers":           spec("List Transcription Speakers", "Transcribe and review", "List diarized speakers and segment counts for one completed transcription.", "Use before asking a user to relabel speakers or filter by speaker.", []string{"transcription:read"}, true, false, false, false, "", nil, []string{"update_speaker_labels"}),
		"update_speaker_labels":                 spec("Update Speaker Labels", "Transcribe and review", "Update diarized speaker labels inside encrypted transcript content.", "Use after the user confirms speaker names or corrections.", []string{"transcription:read", "transcription:write"}, false, false, true, false, "", nil, []string{"list_transcription_speakers", "get_transcript_segments"}),
		"ask_transcript":                        spec("Ask Transcript", "Search and cite evidence", "Answer a question using only bounded cited transcript evidence and refuse unsupported answers.", "Use for grounded Q&A on one transcript; show citations to the user.", []string{"transcription:read", "analysis:write"}, true, false, false, false, "", nil, []string{"get_transcript_segments"}),
		"create_transcription":                  spec("Create Transcription", "Transcribe and review", "Start an asynchronous transcription job for an existing media file.", "Use after uploading audio or selecting ready media; poll with get_transcription_detail.", []string{"transcription:write"}, false, false, false, false, "", nil, []string{"get_transcription_detail"}),
		"get_transcription_detail":              spec("Get Transcription Detail", "Transcribe and review", "Get transcription metadata plus parent media or URL metadata, preprocessing info, and summary statuses.", "Use as the status poll and detail view after any transcription job.", []string{"transcription:read"}, true, false, false, false, "", []string{"Summaries are omitted when summary:read is not granted."}, []string{"get_transcript_segments", "create_summary"}),
		"get_recent_activity":                   spec("Get Recent Activity", "Monitor activity", "Get recent transcriptions with source-aware metadata and summary status in one call.", "Use for dashboards and wrapper landing-state summaries.", []string{"transcription:read", "summary:read"}, true, false, false, false, "", nil, []string{"get_transcription_detail"}),
		"list_summaries":                        spec("List Summaries", "Summarize and brief", "List AI-generated summaries with type, status, word count, and search filters.", "Use to find existing summaries before creating a duplicate.", []string{"summary:read"}, true, false, false, false, "", nil, []string{"get_summary"}),
		"get_summary":                           spec("Get Summary", "Summarize and brief", "Get one summary, including content when completed.", "Use when the user asks for an existing summary or export source.", []string{"summary:read"}, true, false, false, false, "", nil, []string{"export_document", "create_redacted_export"}),
		"create_summary":                        spec("Create Summary", "Summarize and brief", "Start asynchronous summarization for a completed transcription.", "Use when no suitable completed summary exists for the requested summary type.", []string{"summary:write", "transcription:read"}, false, false, false, false, "", nil, []string{"get_summary"}),
		"export_document":                       spec("Export Document", "Export safely", "Export a transcription or summary as JSON through MCP.", "Use when a wrapper needs structured export data; use REST for PDF or DOCX.", []string{"export:read"}, true, false, false, false, "Also requires transcription:read for transcription exports or summary:read for summary exports.", []string{"MCP supports JSON only for this tool."}, nil),
		"create_redacted_export":                spec("Create Redacted Export", "Export safely", "Export transcription or summary JSON with deterministic redaction placeholders.", "Use before sharing sensitive transcript or summary content outside the trusted workspace.", []string{"export:read"}, true, false, false, false, "Also requires transcription:read for transcription exports or summary:read for summary exports.", []string{"Redaction is deterministic mitigation, not a legal guarantee."}, nil),
		"search_all":                            spec("Search All", "Search and cite evidence", "Search media files and URL transcription metadata across the organization.", "Use as the first step when the user describes a recording but does not provide an ID.", []string{"media:read", "transcription:read"}, true, false, false, false, "", []string{"Semantic mode returns semantic_search_unavailable until encrypted semantic indexing exists."}, []string{"get_transcription_detail", "search_transcript_segments"}),
		"search_transcript_segments":            spec("Search Transcript Segments", "Search and cite evidence", "Search bounded transcript segments lexically and return cited timestamped snippets.", "Use when the user asks where something was said or needs quote-level evidence.", []string{"transcription:read"}, true, false, false, false, "", []string{"Search is lexical and does not persist plaintext indexes."}, []string{"ask_transcript"}),
		"list_transcriptions_without_summaries": spec("List Transcriptions Without Summaries", "Monitor activity", "List completed transcriptions missing summaries of a requested type.", "Use to build summarization backlogs and batch work queues.", []string{"transcription:read", "summary:read"}, true, false, false, false, "", nil, []string{"create_summary"}),
		"list_collections":                      spec("List Collections", "Organize collections", "List transcript collections in the authenticated organization.", "Use before adding transcripts to a collection or asking collection-level questions.", []string{"collection:read"}, true, false, false, false, "", nil, []string{"ask_collection"}),
		"create_collection":                     spec("Create Collection", "Organize collections", "Create a transcript collection for repeated analysis.", "Use when the user wants to group related transcripts for a case, class, deal, or story.", []string{"collection:write"}, false, false, false, false, "", nil, []string{"add_to_collection"}),
		"add_to_collection":                     spec("Add To Collection", "Organize collections", "Add a transcription to a collection idempotently.", "Use after the user chooses a transcript and target collection.", []string{"collection:write", "transcription:read"}, false, false, true, false, "", nil, []string{"ask_collection"}),
		"remove_from_collection":                spec("Remove From Collection", "Organize collections", "Remove a transcription from a collection idempotently.", "Use after explicit user confirmation to change collection membership.", []string{"collection:write"}, false, false, true, false, "", nil, []string{"list_collections"}),
		"ask_collection":                        spec("Ask Collection", "Search and cite evidence", "Answer a question using bounded cited transcript evidence from a collection.", "Use for grounded Q&A across a known group of transcripts.", []string{"collection:read", "transcription:read", "analysis:write"}, true, false, false, false, "", nil, []string{"build_story_brief", "create_case_timeline", "build_diligence_memo", "create_study_guide"}),
		"build_story_brief":                     spec("Build Story Brief", "Summarize and brief", "Build a journalist story brief from cited transcript evidence.", "Use for editorial planning, source review, and story angle extraction.", []string{"analysis:write", "transcription:read"}, true, false, false, false, "If collection_id is provided, collection:read is also required.", nil, nil),
		"create_case_timeline":                  spec("Create Case Timeline", "Summarize and brief", "Create a law enforcement case timeline from cited transcript evidence.", "Use for chronological event extraction from interviews or recordings.", []string{"analysis:write", "transcription:read"}, true, false, false, false, "If collection_id is provided, collection:read is also required.", nil, nil),
		"build_diligence_memo":                  spec("Build Diligence Memo", "Summarize and brief", "Build an investment banking diligence memo from cited transcript evidence.", "Use for deal, management-call, and due-diligence transcript review.", []string{"analysis:write", "transcription:read"}, true, false, false, false, "If collection_id is provided, collection:read is also required.", nil, nil),
		"create_study_guide":                    spec("Create Study Guide", "Summarize and brief", "Create a student study guide from cited transcript evidence.", "Use for lectures, tutorials, and class discussion recordings.", []string{"analysis:write", "transcription:read"}, true, false, false, false, "If collection_id is provided, collection:read is also required.", nil, nil),
	}
}

func spec(
	title, capability, description, whenToUse string,
	scopes []string,
	readOnly bool,
	destructive bool,
	idempotent bool,
	openWorld bool,
	scopeNote string,
	limitations []string,
	followUps []string,
) mcpToolSpec {
	return mcpToolSpec{
		Title:          title,
		Capability:     capability,
		Description:    description,
		WhenToUse:      whenToUse,
		RequiredScopes: scopes,
		ScopeNote:      scopeNote,
		FollowUps:      followUps,
		Limitations:    limitations,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: boolPtr(destructive),
			IdempotentHint:  idempotent,
			OpenWorldHint:   boolPtr(openWorld),
		},
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func catalogTool(name string) *mcp.Tool {
	spec, ok := buildMCPToolCatalog()[name]
	if ok {
		return &mcp.Tool{
			Name:        name,
			Title:       spec.Title,
			Description: spec.Description + " " + spec.WhenToUse,
			Annotations: cloneToolAnnotations(spec.Annotations),
		}
	}

	return &mcp.Tool{
		Name:        name,
		Title:       titleFromToolName(name),
		Description: fmt.Sprintf("Voxis MCP tool %q.", name),
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: boolPtr(false)},
	}
}

func cloneToolAnnotations(a *mcp.ToolAnnotations) *mcp.ToolAnnotations {
	if a == nil {
		return nil
	}
	clone := *a
	if a.DestructiveHint != nil {
		clone.DestructiveHint = boolPtr(*a.DestructiveHint)
	}
	if a.OpenWorldHint != nil {
		clone.OpenWorldHint = boolPtr(*a.OpenWorldHint)
	}
	return &clone
}

func buildCapabilityCatalogJSON(toolNames []string) (string, error) {
	tools := configuredMCPToolCatalog(toolNames)
	payload := struct {
		Version        string                 `json:"version"`
		ToolCount      int                    `json:"tool_count"`
		Capabilities   []capabilityGroup      `json:"capabilities"`
		Workflows      []wrapperWorkflow      `json:"workflows"`
		Tools          map[string]mcpToolSpec `json:"tools"`
		SafetyGuidance []string               `json:"safety_guidance"`
	}{
		Version:      capabilityCatalogVersion,
		ToolCount:    len(tools),
		Capabilities: registeredCapabilityGroups(toolNames),
		Workflows:    registeredWorkflows(toolNames),
		Tools:        tools,
		SafetyGuidance: []string{
			"All operations are scoped to the authenticated organization.",
			"Use citation-returning tools when answering from transcript content.",
			"Audio bytes are uploaded through REST multipart, not through MCP.",
			"Semantic search is unavailable until an opt-in encrypted semantic index is configured.",
			"Redaction is deterministic mitigation, not a legal guarantee.",
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal MCP capability catalog: %w", err)
	}
	return string(data), nil
}

// configuredMCPToolCatalog returns the catalog entries for the registered
// tools only. A registered tool without an entry still appears, described by
// the same fallback annotateTool gives it.
func configuredMCPToolCatalog(toolNames []string) map[string]mcpToolSpec {
	all := buildMCPToolCatalog()
	configured := make(map[string]mcpToolSpec, len(toolNames))
	for _, name := range toolNames {
		spec, ok := all[name]
		if !ok {
			fallback := catalogTool(name)
			spec = mcpToolSpec{Title: fallback.Title, Description: fallback.Description, Annotations: fallback.Annotations}
		}
		configured[name] = spec
	}
	return configured
}

// registeredCapabilityGroups keeps the groups that contain a registered tool.
func registeredCapabilityGroups(toolNames []string) []capabilityGroup {
	all := buildMCPToolCatalog()
	used := make(map[string]bool, len(mcpCapabilityGroups))
	for _, name := range toolNames {
		used[all[name].Capability] = true
	}
	groups := make([]capabilityGroup, 0, len(mcpCapabilityGroups))
	for _, group := range mcpCapabilityGroups {
		if used[group.Name] {
			groups = append(groups, group)
		}
	}
	return groups
}

// registeredWorkflows keeps the workflows whose MCP steps are all registered.
// Steps naming a REST call ("REST ...") are not MCP tools and always pass.
func registeredWorkflows(toolNames []string) []wrapperWorkflow {
	registered := make(map[string]bool, len(toolNames))
	for _, name := range toolNames {
		registered[name] = true
	}
	workflows := make([]wrapperWorkflow, 0, len(mcpWrapperWorkflows))
	for _, workflow := range mcpWrapperWorkflows {
		if workflowStepsRegistered(workflow, registered) {
			workflows = append(workflows, workflow)
		}
	}
	return workflows
}

func workflowStepsRegistered(workflow wrapperWorkflow, registered map[string]bool) bool {
	for _, step := range workflow.Steps {
		if !strings.HasPrefix(step, "REST ") && !registered[step] {
			return false
		}
	}
	return true
}

func formatScopes(scopes []string) string {
	if len(scopes) == 0 {
		return "none"
	}
	return strings.Join(scopes, ", ")
}

func titleFromToolName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
