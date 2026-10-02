package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// --- Input types for tools ---
//
// JSON schema behavior:
// - Fields without `omitempty` in their `json` tag are automatically required.
// - The `jsonschema` tag value is used as the field description (plain text, no key=value).

// ListMediaInput is the input for the list_media tool.
type ListMediaInput struct {
	Limit       int      `json:"limit,omitempty" jsonschema:"Maximum number of items to return (default 20, max 100)"`
	Offset      int      `json:"offset,omitempty" jsonschema:"Number of items to skip for pagination"`
	DateFrom    string   `json:"date_from,omitempty" jsonschema:"Filter by creation date (RFC 3339, e.g. 2026-01-01T00:00:00Z)"`
	DateTo      string   `json:"date_to,omitempty" jsonschema:"Filter by creation date upper bound (RFC 3339)"`
	MinDuration *float64 `json:"min_duration,omitempty" jsonschema:"Minimum audio duration in seconds (0-86400)"`
	MaxDuration *float64 `json:"max_duration,omitempty" jsonschema:"Maximum audio duration in seconds (0-86400)"`
	Status      string   `json:"status,omitempty" jsonschema:"Filter by status: pending, encrypting, ready, or failed"`
	Search      string   `json:"search,omitempty" jsonschema:"Filter by filename, title, or description (case-insensitive, max 200 chars)"`
	SortBy      string   `json:"sort_by,omitempty" jsonschema:"Sort field: created_at, duration, or size (default: created_at)"`
	SortOrder   string   `json:"sort_order,omitempty" jsonschema:"Sort direction: asc or desc (default: desc)"`
}

// GetMediaInput is the input for the get_media tool.
type GetMediaInput struct {
	MediaID string `json:"media_id" jsonschema:"The UUID of the media file"`
}

// DeleteMediaInput is the input for the delete_media tool.
type DeleteMediaInput struct {
	MediaID string `json:"media_id" jsonschema:"The UUID of the media file to delete"`
}

// UpdateMediaInput is the input for the update_media tool.
type UpdateMediaInput struct {
	MediaID     string  `json:"media_id" jsonschema:"The UUID of the media file to update"`
	Title       *string `json:"title,omitempty" jsonschema:"New title for the media (empty string clears it)"`
	Description *string `json:"description,omitempty" jsonschema:"New description for the media (empty string clears it)"`
}

// GetMediaUploadInstructionsInput is the input for the get_media_upload_instructions tool.
type GetMediaUploadInstructionsInput struct {
	Filename    string `json:"filename" jsonschema:"Original audio filename"`
	ContentType string `json:"content_type" jsonschema:"Audio MIME type"`
	SizeBytes   int64  `json:"size_bytes,omitempty" jsonschema:"Expected file size in bytes, if known"`
}

// ListTranscriptionsInput is the input for the list_transcriptions tool.
type ListTranscriptionsInput struct {
	Limit       int      `json:"limit,omitempty" jsonschema:"Maximum number of items to return (default 20, max 100)"`
	Offset      int      `json:"offset,omitempty" jsonschema:"Number of items to skip for pagination"`
	DateFrom    string   `json:"date_from,omitempty" jsonschema:"Filter by creation date (RFC 3339, e.g. 2026-01-01T00:00:00Z)"`
	DateTo      string   `json:"date_to,omitempty" jsonschema:"Filter by creation date upper bound (RFC 3339)"`
	MinDuration *float64 `json:"min_duration,omitempty" jsonschema:"Minimum audio duration in seconds (0-86400)"`
	MaxDuration *float64 `json:"max_duration,omitempty" jsonschema:"Maximum audio duration in seconds (0-86400)"`
	Languages   []string `json:"languages,omitempty" jsonschema:"Filter by language codes (e.g. [en, id, ms]). Returns transcriptions containing any of the specified languages."`
	MinSpeakers *int     `json:"min_speakers,omitempty" jsonschema:"Minimum speaker count (0-100)"`
	MaxSpeakers *int     `json:"max_speakers,omitempty" jsonschema:"Maximum speaker count (0-100)"`
	Status      string   `json:"status,omitempty" jsonschema:"Filter by status: pending, submitted, completed, or failed"`
	Search      string   `json:"search,omitempty" jsonschema:"Filter by media title, description, filename, or source URL (case-insensitive, max 200 chars)"`
	SortBy      string   `json:"sort_by,omitempty" jsonschema:"Sort field: created_at, duration_seconds, word_count, or speaker_count (default: created_at)"`
	SortOrder   string   `json:"sort_order,omitempty" jsonschema:"Sort direction: asc or desc (default: desc)"`
}

// GetTranscriptionInput is the input for the get_transcription tool.
type GetTranscriptionInput struct {
	TranscriptionID string `json:"transcription_id" jsonschema:"The UUID of the transcription"`
	MaxWords        int    `json:"max_words,omitempty" jsonschema:"Maximum number of words to return (1-50000). When set, truncates the transcript on a word boundary and includes truncation metadata."`
}

// GetTranscriptionDetailInput is the input for the get_transcription_detail tool.
type GetTranscriptionDetailInput struct {
	TranscriptionID string `json:"transcription_id" jsonschema:"The UUID of the transcription"`
}

// GetRecentActivityInput is the input for the get_recent_activity tool.
type GetRecentActivityInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Number of recent items to return (default 10, max 25, min 1)"`
}

// CreateTranscriptionInput is the input for the create_transcription tool.
type CreateTranscriptionInput struct {
	MediaID      string   `json:"media_id" jsonschema:"The UUID of the media file to transcribe"`
	Languages    []string `json:"languages,omitempty" jsonschema:"Language codes (e.g. [en, id, ms]). Empty for auto-detect."`
	Diarization  bool     `json:"diarization,omitempty" jsonschema:"Enable speaker diarization"`
	EnhanceAudio bool     `json:"enhance_audio,omitempty" jsonschema:"Enable audio preprocessing to improve transcription quality"`
}

// ListSummariesInput is the input for the list_summaries tool.
type ListSummariesInput struct {
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum number of items to return (default 20, max 100)"`
	Offset      int    `json:"offset,omitempty" jsonschema:"Number of items to skip for pagination"`
	DateFrom    string `json:"date_from,omitempty" jsonschema:"Filter by creation date (RFC 3339, e.g. 2026-01-01T00:00:00Z)"`
	DateTo      string `json:"date_to,omitempty" jsonschema:"Filter by creation date upper bound (RFC 3339)"`
	Status      string `json:"status,omitempty" jsonschema:"Filter by status: pending, completed, or failed"`
	SummaryType string `json:"summary_type,omitempty" jsonschema:"Filter by type: general, key_points, action_items, or q_and_a (Questions & Answers)"`
	Search      string `json:"search,omitempty" jsonschema:"Filter by audio name or description (case-insensitive, max 200 chars)"`
	SortBy      string `json:"sort_by,omitempty" jsonschema:"Sort field: created_at or word_count (default: created_at)"`
	SortOrder   string `json:"sort_order,omitempty" jsonschema:"Sort direction: asc or desc (default: desc)"`
}

// GetSummaryInput is the input for the get_summary tool.
type GetSummaryInput struct {
	SummaryID string `json:"summary_id" jsonschema:"The UUID of the summary"`
}

// CreateSummaryInput is the input for the create_summary tool.
type CreateSummaryInput struct {
	TranscriptionID string `json:"transcription_id" jsonschema:"The UUID of the completed transcription"`
	SummaryType     string `json:"summary_type" jsonschema:"Type of summary: general, key_points, action_items, or q_and_a (Questions & Answers)"`
	HighStakes      *bool  `json:"high_stakes,omitempty" jsonschema:"Run the high-fidelity two-pass summary path. Defaults to the caller preference."`
	SummaryProfile  string `json:"summary_profile,omitempty" jsonschema:"Professional emphasis for this summary: general_professional, legal, investment_analysis, journalism, negotiation, decision_committee, or investigation. Defaults to the caller preference."`
}

// ExportDocumentInput is the input for the export_document tool.
type ExportDocumentInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Type of resource to export: transcription or summary"`
	ResourceID   string `json:"resource_id" jsonschema:"The UUID of the transcription or summary to export"`
	Format       string `json:"format" jsonschema:"Export format: json, pdf, or docx"`
}

// CreateRedactedExportInput is the input for the create_redacted_export tool.
type CreateRedactedExportInput struct {
	ResourceType      string   `json:"resource_type" jsonschema:"transcription or summary"`
	ResourceID        string   `json:"resource_id" jsonschema:"Resource UUID"`
	Format            string   `json:"format" jsonschema:"json only for MCP v1"`
	RedactionPolicies []string `json:"redaction_policies,omitempty" jsonschema:"pii, financial, law_enforcement, education"`
}

// checkScope verifies the authenticated request carries the required scope.
// Authorization fails closed: a request whose context has no scope list (or
// an empty one) is denied regardless of auth method. API-key requests always
// carry their granted scopes. JWT/OAuth requests carry scopes when the token
// has Voxis authorization scopes in its OAuth `scope` claim (e.g. via
// Keycloak client-scope mappings) — OIDC identity scopes such as "openid"
// are filtered out upstream by middleware.DualAuth and never satisfy this
// check.
// Returns nil if authorized, or an error CallToolResult if not.
func checkScope(ctx context.Context, scope string) *mcp.CallToolResult {
	scopes, ok := ctx.Value(middleware.CtxKeyScopeList).([]string)
	if !ok || len(scopes) == 0 {
		return noScopesResult(ctx, scope)
	}
	if !hasScope(scopes, scope) {
		return missingScopeResult(ctx, scope)
	}
	return nil
}

// checkScopes verifies the authenticated request has ALL required scopes (AND
// semantics). It follows checkScope semantics and fails closed: any request
// without Voxis scopes in context is denied. Returns nil if authorized, or an
// error CallToolResult identifying the first missing scope.
func checkScopes(ctx context.Context, scopes []string) *mcp.CallToolResult {
	for _, required := range scopes {
		if denied := checkScope(ctx, required); denied != nil {
			return denied
		}
	}
	return nil
}

// contextHasScope reports whether the request carries scope without
// recording a denial, for tools that degrade instead of refusing.
func contextHasScope(ctx context.Context, scope string) bool {
	scopes, ok := ctx.Value(middleware.CtxKeyScopeList).([]string)
	return ok && hasScope(scopes, scope)
}

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

func missingScopeResult(ctx context.Context, scope string) *mcp.CallToolResult {
	recordOutcome(ctx, "forbidden")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("forbidden: missing required scope %q", scope)},
		},
		IsError: true,
	}
}

// noScopesResult denies a request whose context carries no Voxis scopes at
// all (e.g. an OAuth token that requested only OIDC identity scopes such as
// "openid"), with guidance on how to obtain access.
func noScopesResult(ctx context.Context, scope string) *mcp.CallToolResult {
	recordOutcome(ctx, "forbidden")
	text := fmt.Sprintf(
		"forbidden: token carries no Voxis scopes; request scopes such as %q when authorizing, or use an API key granted the required scopes",
		scope,
	)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

// clampLimit ensures limit is between 1 and 100, defaulting to 20.
func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// clampOffset ensures offset is non-negative.
func clampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// orgIDFromContext extracts the organization ID from request context.
// Returns an error if the org_id is not present (auth bridge not configured).
func orgIDFromContext(ctx context.Context) (string, error) {
	val := ctx.Value(middleware.CtxKeyOrgID)
	if val == nil {
		return "", fmt.Errorf("organization ID not found in context (authentication required)")
	}
	orgID, ok := val.(string)
	if !ok || orgID == "" {
		return "", fmt.Errorf("invalid organization ID in context")
	}
	return orgID, nil
}

func subjectFromContext(ctx context.Context) string {
	subject, ok := ctx.Value(middleware.CtxKeySubject).(string)
	if !ok {
		return ""
	}
	return subject
}

// textResult creates a CallToolResult with a single text content.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// jsonResult marshals v to JSON and wraps it in a CallToolResult.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return textResult(string(data)), nil
}

// errorResult creates a CallToolResult indicating an error.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: err.Error()},
		},
		IsError: true,
	}
}

// registerTools adds all MCP tools to the server.
func registerTools(server *toolRegistry, deps Dependencies) {
	registerMediaTools(server, deps)
	registerTranscriptionTools(server, deps)
	registerSummaryTools(server, deps)
	registerExportTools(server, deps)
	registerSearchTools(server, deps)
}

// registerMediaTools adds media-related MCP tools.
func registerMediaTools(server *toolRegistry, deps Dependencies) {
	registerTool(server, &mcp.Tool{
		Name:        "list_media",
		Description: "List audio media files in the organization with optional filters. Returns metadata including filename, content type, size, duration, and status.",
	}, wrapTool[ListMediaInput, any]("list_media", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input ListMediaInput) (*mcp.CallToolResult, any, error) {
			return handleListMedia(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_media",
		Description: "Get details of a specific media file by ID. Returns metadata including filename, content type, size, duration, and status.",
	}, wrapTool[GetMediaInput, any]("get_media", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetMediaInput) (*mcp.CallToolResult, any, error) {
			if denied := checkScope(ctx, "media:read"); denied != nil {
				return denied, nil, nil
			}
			orgID, err := orgIDFromContext(ctx)
			if err != nil {
				return errorResult(err), nil, nil
			}

			media, err := deps.MediaService.GetByID(ctx, orgID, input.MediaID)
			if err != nil {
				return toolError(ctx, deps.Logger, "get_media", "get media", err), nil, nil
			}
			// Privilege media is hidden from MCP get_media (self-review iter 1, Finding #2):
			// return ErrNotFound to avoid leaking metadata.
			if media.IsPrivilege() {
				return toolError(ctx, deps.Logger, "get_media", "get media", domain.ErrNotFound), nil, nil
			}

			result := struct {
				ID          string  `json:"id"`
				Filename    string  `json:"filename"`
				ContentType string  `json:"content_type"`
				Size        int64   `json:"size"`
				Duration    float64 `json:"duration"`
				Status      string  `json:"status"`
				ScanStatus  string  `json:"scan_status"`
				CreatedAt   string  `json:"created_at"`
			}{
				ID:          media.ID,
				Filename:    media.Filename,
				ContentType: media.ContentType,
				Size:        media.Size,
				Duration:    media.Duration,
				Status:      media.Status,
				ScanStatus:  media.ScanStatus,
				CreatedAt:   media.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}

			r, jErr := jsonResult(result)
			return r, nil, jErr
		}))

	registerTool(server, &mcp.Tool{
		Name:        "delete_media",
		Description: "Delete a media file by ID. This permanently removes the media and associated storage.",
	}, wrapTool[DeleteMediaInput, any]("delete_media", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input DeleteMediaInput) (*mcp.CallToolResult, any, error) {
			return handleDeleteMedia(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "update_media",
		Description: "Update the title and/or description of a media file. Provide at least one field to update. Empty string clears the field.",
	}, wrapTool[UpdateMediaInput, any]("update_media", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateMediaInput) (*mcp.CallToolResult, any, error) {
			return handleUpdateMedia(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_media_upload_instructions",
		Description: "Return the REST multipart upload contract for uploading audio through Voxis secure upload pipeline. MCP does not carry audio bytes.",
	}, wrapTool[GetMediaUploadInstructionsInput, any]("get_media_upload_instructions", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetMediaUploadInstructionsInput) (*mcp.CallToolResult, any, error) {
			return handleGetMediaUploadInstructions(ctx, deps, input)
		}))
}

// deleteMediaScopes are required by delete_media, which removes the media's
// transcriptions and summaries along with the audio.
var deleteMediaScopes = []string{"media:write", "transcription:write", "summary:write"}

func handleDeleteMedia(ctx context.Context, deps Dependencies, input DeleteMediaInput) (*mcp.CallToolResult, any, error) {
	// Deleting media also deletes its transcriptions and summaries.
	if denied := checkScopes(ctx, deleteMediaScopes); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if err := deps.MediaService.Delete(ctx, orgID, input.MediaID); err != nil {
		return toolError(ctx, deps.Logger, "delete_media", "delete media", err), nil, nil
	}

	return textResult("Media deleted successfully"), nil, nil
}

// mediaSortAllowlist is the valid sort fields for list_media.
var mediaSortAllowlist = []string{"created_at", "duration", "size"}

// mediaStatusAllowlist is the valid status values for list_media.
var mediaStatusAllowlist = []string{"pending", "encrypting", "ready", "failed"}

func handleListMedia(ctx context.Context, deps Dependencies, input ListMediaInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "media:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if validErr := validateMediaFilters(input); validErr != nil {
		return validationError(ctx, validErr), nil, nil
	}

	filter, err := buildMediaFilter(input)
	if err != nil {
		return errorResult(err), nil, nil
	}

	items, err := deps.MediaService.ListFiltered(ctx, orgID, filter)
	if err != nil {
		return toolError(ctx, deps.Logger, "list_media", "list media", err), nil, nil
	}

	type mediaItem struct {
		ID            string  `json:"id"`
		Filename      string  `json:"filename"`
		ContentType   string  `json:"content_type"`
		Size          int64   `json:"size"`
		Duration      float64 `json:"duration"`
		Status        string  `json:"status"`
		ScanStatus    string  `json:"scan_status"`
		RecordingMode string  `json:"recording_mode,omitempty"`
		CreatedAt     string  `json:"created_at"`
	}

	// Filter privilege media from MCP results entirely (self-review iter 1, Finding #2):
	// MCP must not expose privilege filenames, sizes, or durations.
	visible := items[:0]
	for _, m := range items {
		if m.IsPrivilege() {
			continue
		}
		visible = append(visible, m)
	}

	result := struct {
		Items []mediaItem `json:"items"`
		Count int         `json:"count"`
	}{
		Items: make([]mediaItem, len(visible)),
		Count: len(visible),
	}

	for i, m := range visible {
		result.Items[i] = mediaItem{
			ID:            m.ID,
			Filename:      m.Filename,
			ContentType:   m.ContentType,
			Size:          m.Size,
			Duration:      m.Duration,
			Status:        m.Status,
			ScanStatus:    m.ScanStatus,
			RecordingMode: string(m.RecordingMode),
			CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// validateMediaFilters validates all filter parameters for list_media.
func validateMediaFilters(input ListMediaInput) error {
	if err := validateOffsetCap(input.Offset); err != nil {
		return err
	}
	if err := validateDateRange(input.DateFrom, input.DateTo); err != nil {
		return err
	}
	if err := validateDurationPair(input.MinDuration, input.MaxDuration); err != nil {
		return err
	}
	if err := validateStatus(input.Status, mediaStatusAllowlist); err != nil {
		return err
	}
	if err := validateSearch(input.Search, 200); err != nil {
		return err
	}
	if err := validateSortBy(input.SortBy, mediaSortAllowlist); err != nil {
		return err
	}
	return validateSortOrder(input.SortOrder)
}

// buildMediaFilter constructs a port.MediaListFilter from validated input.
func buildMediaFilter(input ListMediaInput) (port.MediaListFilter, error) {
	search, err := service.NormalizeSearchQuery(input.Search)
	if err != nil {
		return port.MediaListFilter{}, err
	}
	f := port.MediaListFilter{
		Status:    input.Status,
		Search:    search,
		SortBy:    input.SortBy,
		SortOrder: input.SortOrder,
		Limit:     clampLimit(input.Limit),
		Offset:    clampOffset(input.Offset),
	}

	if input.DateFrom != "" {
		t, err := time.Parse(time.RFC3339, input.DateFrom)
		if err != nil {
			return f, fmt.Errorf("invalid date_from: %w", err)
		}
		f.DateFrom = &t
	}
	if input.DateTo != "" {
		t, err := time.Parse(time.RFC3339, input.DateTo)
		if err != nil {
			return f, fmt.Errorf("invalid date_to: %w", err)
		}
		f.DateTo = &t
	}

	f.MinDuration = input.MinDuration
	f.MaxDuration = input.MaxDuration

	return f, nil
}

func handleUpdateMedia(ctx context.Context, deps Dependencies, input UpdateMediaInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "media:write"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if input.Title == nil && input.Description == nil {
		return validationError(ctx, fmt.Errorf("at least one of title or description must be provided")), nil, nil
	}

	existing, getErr := deps.MediaService.GetByID(ctx, orgID, input.MediaID)
	if getErr != nil {
		return toolError(ctx, deps.Logger, "update_media", "update media", getErr), nil, nil
	}
	if existing.IsPrivilege() {
		return toolError(ctx, deps.Logger, "update_media", "update media", domain.ErrNotFound), nil, nil
	}

	media, err := deps.MediaService.UpdateMetadata(ctx, orgID, input.MediaID, input.Title, input.Description)
	if err != nil {
		return toolError(ctx, deps.Logger, "update_media", "update media", err), nil, nil
	}

	result := struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Filename    string `json:"filename"`
		Status      string `json:"status"`
		Message     string `json:"message"`
	}{
		ID:          media.ID,
		Title:       media.Title,
		Description: media.Description,
		Filename:    media.Filename,
		Status:      media.Status,
		Message:     "Media metadata updated successfully",
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func handleGetMediaUploadInstructions(
	ctx context.Context,
	deps Dependencies,
	input GetMediaUploadInstructionsInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "media:write"); denied != nil {
		return denied, nil, nil
	}
	if _, err := orgIDFromContext(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if err := validateMediaUploadInstructionInput(input); err != nil {
		return validationError(ctx, err), nil, nil
	}

	result, err := mediaUploadInstructionsResult(deps.PublicBaseURL, input)
	if err != nil {
		return errorResult(err), nil, nil
	}
	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func validateMediaUploadInstructionInput(input GetMediaUploadInstructionsInput) error {
	filename := strings.TrimSpace(input.Filename)
	contentType := strings.TrimSpace(input.ContentType)
	if filename == "" {
		return fmt.Errorf("filename is required")
	}
	if !domain.IsAllowedAudioType(contentType) {
		return fmt.Errorf("invalid media upload metadata: unsupported content type: %w", domain.ErrInvalidInput)
	}
	if input.SizeBytes < 0 || input.SizeBytes > domain.MaxMediaSize {
		return fmt.Errorf("size_bytes must be between 0 and %d", domain.MaxMediaSize)
	}
	if input.SizeBytes > 0 && input.SizeBytes < domain.MinMediaSize {
		return fmt.Errorf("size_bytes must be 0 when unknown or at least %d", domain.MinMediaSize)
	}
	if err := domain.ValidateExtensionMIME(filename, contentType); err != nil {
		return fmt.Errorf("invalid media upload metadata: %w", err)
	}
	return nil
}

func mediaUploadInstructionsResult(publicBaseURL string, input GetMediaUploadInstructionsInput) (any, error) {
	path := "/api/v1/media/upload"
	uploadURL, err := absoluteUploadURL(publicBaseURL, path)
	if err != nil {
		return nil, err
	}
	return struct {
		Method         string   `json:"method"`
		Path           string   `json:"path"`
		URL            string   `json:"url"`
		ContentType    string   `json:"content_type"`
		FormField      string   `json:"form_field"`
		AuthHeader     string   `json:"auth_header"`
		RequiredScopes []string `json:"required_scopes"`
		NextScopes     []string `json:"next_required_scopes"`
		MinSizeBytes   int64    `json:"min_size_bytes"`
		MaxSizeBytes   int64    `json:"max_size_bytes"`
		ResponseID     string   `json:"response_id_field"`
		NextTool       string   `json:"next_tool"`
		RequestedName  string   `json:"requested_filename"`
		ExpectedSize   int64    `json:"expected_size_bytes"`
		Notes          []string `json:"notes"`
	}{
		Method:         "POST",
		Path:           path,
		URL:            uploadURL,
		ContentType:    "multipart/form-data",
		FormField:      "file",
		AuthHeader:     "Authorization: Bearer <Voxis API key or JWT>",
		RequiredScopes: []string{"media:write"},
		NextScopes:     []string{"transcription:write"},
		MinSizeBytes:   domain.MinMediaSize,
		MaxSizeBytes:   domain.MaxMediaSize,
		ResponseID:     "id",
		NextTool:       "create_transcription",
		RequestedName:  strings.TrimSpace(input.Filename),
		ExpectedSize:   input.SizeBytes,
		Notes: []string{
			"Upload the audio file directly to the REST endpoint; do not base64-encode audio through MCP.",
			"Server-to-server uploads do not use browser CORS; browser-hosted wrappers must also be allowed by CORS_ALLOWED_ORIGINS.",
			"The upload response is a media JSON object; use its id as media_id for create_transcription.",
			"Uploaded media may still be security-scanned before transcription processing proceeds.",
		},
	}, nil
}

func absoluteUploadURL(publicBaseURL, path string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL is required for media upload instructions")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL must be an absolute HTTPS URL")
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL must be an HTTPS origin without path, query, or fragment")
	}
	return base + path, nil
}

// registerTranscriptionTools adds transcription-related MCP tools.
func registerTranscriptionTools(server *toolRegistry, deps Dependencies) {
	registerTool(server, &mcp.Tool{
		Name:        "list_transcriptions",
		Description: "List transcriptions in the organization. Returns metadata including status, language, speaker count, word count, and timestamps.",
	}, wrapTool[ListTranscriptionsInput, any]("list_transcriptions", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input ListTranscriptionsInput) (*mcp.CallToolResult, any, error) {
			return handleListTranscriptions(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_transcription",
		Description: "Get details of a specific transcription by ID. For completed transcriptions, includes the full transcript text and speaker map.",
	}, wrapTool[GetTranscriptionInput, any]("get_transcription", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetTranscriptionInput) (*mcp.CallToolResult, any, error) {
			return handleGetTranscription(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_transcript_segments",
		Description: "Get bounded, timestamped transcript segments with citations. Supports optional time-range and speaker filters.",
	}, wrapTool[GetTranscriptSegmentsInput, any]("get_transcript_segments", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetTranscriptSegmentsInput) (*mcp.CallToolResult, any, error) {
			return handleGetTranscriptSegments(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "list_transcription_speakers",
		Description: "List diarized speakers and segment counts for a completed transcription.",
	}, wrapTool[ListTranscriptionSpeakersInput, any]("list_transcription_speakers", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input ListTranscriptionSpeakersInput) (*mcp.CallToolResult, any, error) {
			return handleListTranscriptionSpeakers(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "update_speaker_labels",
		Description: "Update diarized speaker labels for a completed transcription. Requires read and write transcription scopes.",
	}, wrapTool[UpdateSpeakerLabelsInput, any]("update_speaker_labels", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateSpeakerLabelsInput) (*mcp.CallToolResult, any, error) {
			return handleUpdateSpeakerLabels(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "ask_transcript",
		Description: "Answer a question using only bounded, cited transcript evidence. Refuses unsupported answers.",
	}, wrapTool[AskTranscriptInput, any]("ask_transcript", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input AskTranscriptInput) (*mcp.CallToolResult, any, error) {
			return handleAskTranscript(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "create_transcription",
		Description: "Start a new transcription job for a media file. The transcription runs asynchronously -- use get_transcription to check status.",
	}, wrapTool[CreateTranscriptionInput, any]("create_transcription", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input CreateTranscriptionInput) (*mcp.CallToolResult, any, error) {
			return handleCreateTranscription(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_transcription_detail",
		Description: "Get a transcription with all related data in one call: transcription metadata, parent media info, URL metadata (for URL sources), preprocessing info, and summary statuses. Requires transcription:read scope; summaries list requires summary:read (omitted if missing).",
	}, wrapTool[GetTranscriptionDetailInput, any]("get_transcription_detail", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetTranscriptionDetailInput) (*mcp.CallToolResult, any, error) {
			return handleGetTranscriptionDetail(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_recent_activity",
		Description: "Get the most recent transcriptions with summary statuses. Returns transcription metadata, source-type-aware info (media or URL), and which summary types exist per transcription. Single call, no pagination. Requires both transcription:read and summary:read scopes.",
	}, wrapTool[GetRecentActivityInput, any]("get_recent_activity", heavyToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetRecentActivityInput) (*mcp.CallToolResult, any, error) {
			return handleGetRecentActivity(ctx, deps, input)
		}))
}

// transcriptionSortAllowlist is the valid sort fields for list_transcriptions.
var transcriptionSortAllowlist = []string{"created_at", "duration_seconds", "word_count", "speaker_count"}

// transcriptionStatusAllowlist is the valid status values for list_transcriptions.
var transcriptionStatusAllowlist = []string{"pending", "submitted", "completed", "failed"}

func handleListTranscriptions(ctx context.Context, deps Dependencies, input ListTranscriptionsInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "transcription:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if validErr := validateTranscriptionFilters(input); validErr != nil {
		return validationError(ctx, validErr), nil, nil
	}

	filter, err := buildTranscriptionFilter(input)
	if err != nil {
		return errorResult(err), nil, nil
	}

	items, err := deps.TranscriptionService.ListFiltered(ctx, orgID, filter)
	if err != nil {
		return toolError(ctx, deps.Logger, "list_transcriptions", "list transcriptions", err), nil, nil
	}

	type transItem struct {
		ID               string   `json:"id"`
		MediaID          string   `json:"media_id"`
		MediaFilename    string   `json:"media_filename"`
		MediaTitle       string   `json:"media_title,omitempty"`
		MediaDescription string   `json:"media_description,omitempty"`
		Status           string   `json:"status"`
		Languages        []string `json:"languages"`
		SpeakerCount     int      `json:"speaker_count"`
		WordCount        int      `json:"word_count"`
		Duration         float64  `json:"duration_seconds"`
		CreatedAt        string   `json:"created_at"`
	}

	result := struct {
		Items []transItem `json:"items"`
		Count int         `json:"count"`
	}{
		Items: make([]transItem, len(items)),
		Count: len(items),
	}

	for i, t := range items {
		result.Items[i] = transItem{
			ID:               t.ID,
			MediaID:          t.MediaID,
			MediaFilename:    t.MediaFilename,
			MediaTitle:       t.MediaTitle,
			MediaDescription: t.MediaDescription,
			Status:           t.Status,
			Languages:        t.Languages,
			SpeakerCount:     t.SpeakerCount,
			WordCount:        t.WordCount,
			Duration:         t.DurationSeconds,
			CreatedAt:        t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// validateTranscriptionFilters validates all filter parameters for list_transcriptions.
func validateTranscriptionFilters(input ListTranscriptionsInput) error {
	if err := validateOffsetCap(input.Offset); err != nil {
		return err
	}
	if err := validateDateRange(input.DateFrom, input.DateTo); err != nil {
		return err
	}
	if err := validateDurationPair(input.MinDuration, input.MaxDuration); err != nil {
		return err
	}
	if err := validateLanguages(input.Languages); err != nil {
		return err
	}
	if err := validateSpeakerPair(input.MinSpeakers, input.MaxSpeakers); err != nil {
		return err
	}
	if err := validateStatus(input.Status, transcriptionStatusAllowlist); err != nil {
		return err
	}
	if err := validateSearch(input.Search, 200); err != nil {
		return err
	}
	if err := validateSortBy(input.SortBy, transcriptionSortAllowlist); err != nil {
		return err
	}
	return validateSortOrder(input.SortOrder)
}

// validateDurationPair validates optional min/max duration floats.
func validateDurationPair(minDur, maxDur *float64) error {
	if minDur != nil {
		if err := validateFloatRange(*minDur, 0, 86400, "min_duration"); err != nil {
			return err
		}
	}
	if maxDur != nil {
		if err := validateFloatRange(*maxDur, 0, 86400, "max_duration"); err != nil {
			return err
		}
	}
	if minDur != nil && maxDur != nil {
		return validateMinMaxFloat(*minDur, *maxDur, "duration")
	}
	return nil
}

// validateSpeakerPair validates optional min/max speaker ints.
func validateSpeakerPair(minSp, maxSp *int) error {
	if minSp != nil {
		if err := validateIntRange(*minSp, 0, 100, "min_speakers"); err != nil {
			return err
		}
	}
	if maxSp != nil {
		if err := validateIntRange(*maxSp, 0, 100, "max_speakers"); err != nil {
			return err
		}
	}
	if minSp != nil && maxSp != nil {
		return validateMinMaxInt(*minSp, *maxSp, "speakers")
	}
	return nil
}

// buildTranscriptionFilter constructs a port.TranscriptionListFilter from validated input.
func buildTranscriptionFilter(input ListTranscriptionsInput) (port.TranscriptionListFilter, error) {
	search, err := service.NormalizeSearchQuery(input.Search)
	if err != nil {
		return port.TranscriptionListFilter{}, err
	}
	f := port.TranscriptionListFilter{
		Languages: input.Languages,
		Status:    input.Status,
		Search:    search,
		SortBy:    input.SortBy,
		SortOrder: input.SortOrder,
		Limit:     clampLimit(input.Limit),
		Offset:    clampOffset(input.Offset),
	}

	if input.DateFrom != "" {
		t, err := time.Parse(time.RFC3339, input.DateFrom)
		if err != nil {
			return f, fmt.Errorf("invalid date_from: %w", err)
		}
		f.DateFrom = &t
	}
	if input.DateTo != "" {
		t, err := time.Parse(time.RFC3339, input.DateTo)
		if err != nil {
			return f, fmt.Errorf("invalid date_to: %w", err)
		}
		f.DateTo = &t
	}

	f.MinDuration = input.MinDuration
	f.MaxDuration = input.MaxDuration
	f.MinSpeakers = input.MinSpeakers
	f.MaxSpeakers = input.MaxSpeakers

	return f, nil
}

func handleGetTranscription(ctx context.Context, deps Dependencies, input GetTranscriptionInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "transcription:read"); denied != nil {
		return denied, nil, nil
	}

	// Validate max_words if provided (non-zero means user set it).
	if input.MaxWords != 0 {
		if err := validateIntRange(input.MaxWords, 1, 50000, "max_words"); err != nil {
			return errorResult(err), nil, nil
		}
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	trans, err := deps.TranscriptionService.GetByID(ctx, orgID, input.TranscriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "get_transcription", "get transcription", err), nil, nil
	}

	// Apply truncation if max_words is set and transcript has content.
	transcript := trans.FullTranscript
	truncated := false
	totalWordCount := trans.WordCount

	if input.MaxWords > 0 && transcript != "" {
		tr := service.TruncateTranscript(transcript, input.MaxWords)
		transcript = tr.Text
		truncated = tr.Truncated
		totalWordCount = tr.TotalWordCount
	}

	result := struct {
		ID             string            `json:"id"`
		MediaID        string            `json:"media_id"`
		MediaFilename  string            `json:"media_filename"`
		Status         string            `json:"status"`
		Languages      []string          `json:"languages"`
		Diarization    bool              `json:"diarization"`
		SpeakerCount   int               `json:"speaker_count"`
		WordCount      int               `json:"word_count"`
		Duration       float64           `json:"duration_seconds"`
		FullTranscript string            `json:"full_transcript,omitempty"`
		SpeakerMap     map[string]string `json:"speaker_map,omitempty"`
		Truncated      bool              `json:"truncated"`
		TotalWordCount int               `json:"total_word_count"`
		ErrorMessage   string            `json:"error_message,omitempty"`
		CreatedAt      string            `json:"created_at"`
		CompletedAt    *string           `json:"completed_at,omitempty"`
	}{
		ID:             trans.ID,
		MediaID:        trans.MediaID,
		MediaFilename:  trans.MediaFilename,
		Status:         trans.Status,
		Languages:      trans.Languages,
		Diarization:    trans.Diarization,
		SpeakerCount:   trans.SpeakerCount,
		WordCount:      trans.WordCount,
		Duration:       trans.DurationSeconds,
		FullTranscript: transcript,
		SpeakerMap:     trans.SpeakerMap,
		Truncated:      truncated,
		TotalWordCount: totalWordCount,
		ErrorMessage:   trans.ErrorMessage,
		CreatedAt:      trans.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if trans.CompletedAt != nil {
		s := trans.CompletedAt.Format("2006-01-02T15:04:05Z")
		result.CompletedAt = &s
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func handleCreateTranscription(ctx context.Context, deps Dependencies, input CreateTranscriptionInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "transcription:write"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	languages := input.Languages
	if len(languages) == 0 {
		languages = []string{"auto"}
	}
	if input.EnhanceAudio && !deps.EnhanceAudioAvailable {
		return errorResult(fmt.Errorf("audio enhancement is not available on this server")), nil, nil
	}
	if deps.JobInserter == nil {
		return errorResult(fmt.Errorf("transcription processing is not configured")), nil, nil
	}
	if limited := transcriptionCreateLimited(ctx, deps); limited != nil {
		return limited, nil, nil
	}

	trans, err := deps.TranscriptionService.Create(ctx, orgID, input.MediaID, languages, input.Diarization, input.EnhanceAudio)
	if err != nil {
		return toolError(ctx, deps.Logger, "create_transcription", "create transcription", err), nil, nil
	}

	if jobErr := deps.JobInserter.InsertTranscribeJob(ctx, trans.ID, trans.MediaID, orgID); jobErr != nil {
		cleanupFailedTranscriptionEnqueue(ctx, deps, orgID, trans.ID, jobErr)
		return errorResult(fmt.Errorf("failed to start transcription processing")), nil, nil
	}

	r, jErr := jsonResult(transcriptionCreatedResult(trans))
	return r, nil, jErr
}

// transcriptionCreateLimited applies the per-user transcription start quota
// shared with REST. A request without a subject is refused rather than
// metered under a shared anonymous bucket.
func transcriptionCreateLimited(ctx context.Context, deps Dependencies) *mcp.CallToolResult {
	if deps.TranscriptionCreateLimiter == nil {
		return nil
	}
	subject := subjectFromContext(ctx)
	if subject != "" && deps.TranscriptionCreateLimiter.AllowUser(middleware.TranscriptionCreateLimitKey, subject) {
		return nil
	}
	recordOutcome(ctx, "rate_limited")
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "rate_limited: too many transcriptions started recently; try again later"}},
		IsError: true,
	}
}

func cleanupFailedTranscriptionEnqueue(ctx context.Context, deps Dependencies, orgID, transcriptionID string, jobErr error) {
	if deps.Logger != nil {
		deps.Logger.Error("failed to enqueue transcription job", "error", jobErr, "transcription_id", transcriptionID)
	}
	if deleteErr := deps.TranscriptionService.Delete(ctx, orgID, transcriptionID); deleteErr != nil && deps.Logger != nil {
		deps.Logger.Error("failed to clean up transcription after enqueue failure", "error", deleteErr, "transcription_id", transcriptionID)
	}
}

func transcriptionCreatedResult(trans *domain.Transcription) any {
	return struct {
		ID      string `json:"id"`
		MediaID string `json:"media_id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}{
		ID:      trans.ID,
		MediaID: trans.MediaID,
		Status:  trans.Status,
		Message: "Transcription job created. Use get_transcription to check status.",
	}
}

func handleGetTranscriptionDetail(ctx context.Context, deps Dependencies, input GetTranscriptionDetailInput) (*mcp.CallToolResult, any, error) {
	// transcription:read is always required.
	if denied := checkScope(ctx, "transcription:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	detail, err := deps.TranscriptionService.GetTranscriptionDetail(ctx, orgID, input.TranscriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "get_transcription_detail", "get transcription detail", err), nil, nil
	}

	trans := detail.Transcription

	// Summaries are omitted, not refused, when summary:read is missing. The
	// check applies to every auth method: OAuth tokens carry scopes too.
	includeSummaries := contextHasScope(ctx, "summary:read")

	type summaryItem struct {
		ID          string  `json:"id"`
		SummaryType string  `json:"summary_type"`
		Status      string  `json:"status"`
		WordCount   int     `json:"word_count"`
		CompletedAt *string `json:"completed_at,omitempty"`
	}

	type mediaInfo struct {
		ID          string  `json:"id"`
		Filename    string  `json:"filename"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Duration    float64 `json:"duration"`
	}

	result := struct {
		ID               string        `json:"id"`
		Status           string        `json:"status"`
		SourceType       string        `json:"source_type"`
		Languages        []string      `json:"languages"`
		Diarization      bool          `json:"diarization"`
		EnhanceAudio     bool          `json:"enhance_audio"`
		PreprocessorUsed string        `json:"preprocessor_used"`
		SpeakerCount     int           `json:"speaker_count"`
		WordCount        int           `json:"word_count"`
		Duration         float64       `json:"duration_seconds"`
		ErrorMessage     string        `json:"error_message,omitempty"`
		CreatedAt        string        `json:"created_at"`
		CompletedAt      *string       `json:"completed_at,omitempty"`
		MediaInfo        *mediaInfo    `json:"media_info,omitempty"`
		Summaries        []summaryItem `json:"summaries,omitempty"`
	}{
		ID:               trans.ID,
		Status:           trans.Status,
		SourceType:       "media",
		Languages:        trans.Languages,
		Diarization:      trans.Diarization,
		EnhanceAudio:     trans.EnhanceAudio,
		PreprocessorUsed: trans.PreprocessorUsed,
		SpeakerCount:     trans.SpeakerCount,
		WordCount:        trans.WordCount,
		Duration:         trans.DurationSeconds,
		ErrorMessage:     trans.ErrorMessage,
		CreatedAt:        trans.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if trans.CompletedAt != nil {
		s := trans.CompletedAt.Format("2006-01-02T15:04:05Z")
		result.CompletedAt = &s
	}

	// Populate media info if present.
	if detail.Media != nil {
		result.MediaInfo = &mediaInfo{
			ID:          detail.Media.ID,
			Filename:    detail.Media.Filename,
			Title:       detail.Media.Title,
			Description: detail.Media.Description,
			Duration:    detail.Media.Duration,
		}
	}

	// Populate summaries if authorized.
	if includeSummaries {
		result.Summaries = make([]summaryItem, len(detail.Summaries))
		for i, s := range detail.Summaries {
			item := summaryItem{
				ID:          s.ID,
				SummaryType: s.SummaryType,
				Status:      s.Status,
				WordCount:   s.WordCount,
			}
			if s.CompletedAt != nil {
				ts := s.CompletedAt.Format("2006-01-02T15:04:05Z")
				item.CompletedAt = &ts
			}
			result.Summaries[i] = item
		}
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// registerSummaryTools adds summary-related MCP tools.
func registerSummaryTools(server *toolRegistry, deps Dependencies) {
	registerTool(server, &mcp.Tool{
		Name:        "list_summaries",
		Description: "List AI-generated summaries in the organization. Returns metadata including type, status, word count, and timestamps.",
	}, wrapTool[ListSummariesInput, any]("list_summaries", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input ListSummariesInput) (*mcp.CallToolResult, any, error) {
			return handleListSummaries(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "get_summary",
		Description: "Get details of a specific summary by ID. For completed summaries, includes the full summary content.",
	}, wrapTool[GetSummaryInput, any]("get_summary", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input GetSummaryInput) (*mcp.CallToolResult, any, error) {
			return handleGetSummary(ctx, deps, input)
		}))

	registerTool(server, &mcp.Tool{
		Name:        "create_summary",
		Description: "Start AI summarization of a completed transcription. Types: general (overview), key_points (bullet points), action_items (tasks), q_and_a (Questions & Answers). Runs asynchronously.",
	}, wrapTool[CreateSummaryInput, any]("create_summary", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input CreateSummaryInput) (*mcp.CallToolResult, any, error) {
			return handleCreateSummary(ctx, deps, input)
		}))
}

// summarySortAllowlist is the valid sort fields for list_summaries.
var summarySortAllowlist = []string{"created_at", "word_count"}

// summaryStatusAllowlist is the valid status values for list_summaries.
var summaryStatusAllowlist = []string{"pending", "completed", "failed"}

type mcpSummaryCitation struct {
	ID           string   `json:"id"`
	Speaker      *string  `json:"speaker,omitempty"`
	StartSeconds *float64 `json:"start_seconds,omitempty"`
	EndSeconds   *float64 `json:"end_seconds,omitempty"`
}

type mcpSummaryGenerationMetadata struct {
	PromptVersion           string   `json:"prompt_version"`
	Model                   string   `json:"model"`
	EndpointLocation        string   `json:"endpoint_location"`
	SourceVersion           string   `json:"source_version"`
	SourceHash              string   `json:"source_hash"`
	StructuredSchemaVersion string   `json:"structured_schema_version"`
	DegradationCodes        []string `json:"degradation_codes"`
}

type mcpSummaryListItem struct {
	ID                 string                        `json:"id"`
	TranscriptionID    string                        `json:"transcription_id"`
	SummaryType        string                        `json:"summary_type"`
	SummaryProfile     string                        `json:"summary_profile"`
	Status             string                        `json:"status"`
	HighStakes         bool                          `json:"high_stakes"`
	ReviewStatus       string                        `json:"review_status"`
	WordCount          int                           `json:"word_count"`
	CreatedAt          string                        `json:"created_at"`
	GenerationMetadata *mcpSummaryGenerationMetadata `json:"generation_metadata"`
}

type mcpSummaryDetailResult struct {
	ID                 string                        `json:"id"`
	TranscriptionID    string                        `json:"transcription_id"`
	SummaryType        string                        `json:"summary_type"`
	SummaryProfile     string                        `json:"summary_profile"`
	Status             string                        `json:"status"`
	Content            string                        `json:"content,omitempty"`
	HighStakes         bool                          `json:"high_stakes"`
	ReviewStatus       string                        `json:"review_status"`
	WordCount          int                           `json:"word_count"`
	ErrorMessage       string                        `json:"error_message,omitempty"`
	CreatedAt          string                        `json:"created_at"`
	CompletedAt        *string                       `json:"completed_at,omitempty"`
	StructuredContent  json.RawMessage               `json:"structured_content"`
	Citations          []mcpSummaryCitation          `json:"citations,omitempty"`
	GenerationMetadata *mcpSummaryGenerationMetadata `json:"generation_metadata"`
}

func handleListSummaries(ctx context.Context, deps Dependencies, input ListSummariesInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "summary:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if validErr := validateSummaryFilters(input); validErr != nil {
		return validationError(ctx, validErr), nil, nil
	}

	filter, err := buildSummaryFilter(input)
	if err != nil {
		return errorResult(err), nil, nil
	}

	items, err := deps.SummaryService.ListFiltered(ctx, orgID, filter)
	if err != nil {
		return toolError(ctx, deps.Logger, "list_summaries", "list summaries", err), nil, nil
	}

	result := struct {
		Items []mcpSummaryListItem `json:"items"`
		Count int                  `json:"count"`
	}{
		Items: make([]mcpSummaryListItem, len(items)),
		Count: len(items),
	}

	for i, s := range items {
		metadata := mcpSummaryMetadata(service.BuildSummaryStructuredPresentation(s, nil).GenerationMetadata)
		result.Items[i] = mcpSummaryListItem{
			ID:                 s.ID,
			TranscriptionID:    s.TranscriptionID,
			SummaryType:        s.SummaryType,
			SummaryProfile:     domain.NormalizeSummaryProfile(s.SummaryProfile),
			Status:             s.Status,
			HighStakes:         s.HighStakes,
			ReviewStatus:       s.ReviewStatus,
			WordCount:          s.WordCount,
			CreatedAt:          s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			GenerationMetadata: metadata,
		}
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// validateSummaryFilters validates all filter parameters for list_summaries.
func validateSummaryFilters(input ListSummariesInput) error {
	if err := validateOffsetCap(input.Offset); err != nil {
		return err
	}
	if err := validateDateRange(input.DateFrom, input.DateTo); err != nil {
		return err
	}
	if err := validateStatus(input.Status, summaryStatusAllowlist); err != nil {
		return err
	}
	if err := validateSummaryType(input.SummaryType); err != nil {
		return err
	}
	if err := validateSearch(input.Search, 200); err != nil {
		return err
	}
	if err := validateSortBy(input.SortBy, summarySortAllowlist); err != nil {
		return err
	}
	return validateSortOrder(input.SortOrder)
}

// buildSummaryFilter constructs a port.SummaryListFilter from validated input.
func buildSummaryFilter(input ListSummariesInput) (port.SummaryListFilter, error) {
	search, err := service.NormalizeSearchQuery(input.Search)
	if err != nil {
		return port.SummaryListFilter{}, err
	}
	f := port.SummaryListFilter{
		Status:      input.Status,
		SummaryType: input.SummaryType,
		Search:      search,
		SortBy:      input.SortBy,
		SortOrder:   input.SortOrder,
		Limit:       clampLimit(input.Limit),
		Offset:      clampOffset(input.Offset),
	}

	if input.DateFrom != "" {
		t, err := time.Parse(time.RFC3339, input.DateFrom)
		if err != nil {
			return f, fmt.Errorf("invalid date_from: %w", err)
		}
		f.DateFrom = &t
	}
	if input.DateTo != "" {
		t, err := time.Parse(time.RFC3339, input.DateTo)
		if err != nil {
			return f, fmt.Errorf("invalid date_to: %w", err)
		}
		f.DateTo = &t
	}

	return f, nil
}

func handleGetSummary(ctx context.Context, deps Dependencies, input GetSummaryInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "summary:read"); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	summary, err := deps.SummaryService.GetByID(ctx, orgID, input.SummaryID)
	if err != nil {
		return toolError(ctx, deps.Logger, "get_summary", "get summary", err), nil, nil
	}

	structuredContent, citations, metadata := mcpSummaryStructuredFields(ctx, deps, orgID, summary, nil)
	result := mcpSummaryDetailResult{
		ID:                 summary.ID,
		TranscriptionID:    summary.TranscriptionID,
		SummaryType:        summary.SummaryType,
		SummaryProfile:     domain.NormalizeSummaryProfile(summary.SummaryProfile),
		Status:             summary.Status,
		Content:            summary.Content,
		HighStakes:         summary.HighStakes,
		ReviewStatus:       summary.ReviewStatus,
		WordCount:          summary.WordCount,
		ErrorMessage:       summary.ErrorMessage,
		CreatedAt:          summary.CreatedAt.Format("2006-01-02T15:04:05Z"),
		StructuredContent:  structuredContent,
		Citations:          citations,
		GenerationMetadata: metadata,
	}

	if summary.CompletedAt != nil {
		s := summary.CompletedAt.Format("2006-01-02T15:04:05Z")
		result.CompletedAt = &s
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

func mcpSummaryStructuredFields(
	ctx context.Context,
	deps Dependencies,
	orgID string,
	summary *domain.Summary,
	transcriptions map[string]*domain.Transcription,
) (json.RawMessage, []mcpSummaryCitation, *mcpSummaryGenerationMetadata) {
	presentation := service.BuildSummaryStructuredPresentation(summary, nil)
	if len(summary.StructuredContent) != 0 {
		transcription, found := transcriptions[summary.TranscriptionID]
		if !found {
			var err error
			transcription, err = deps.TranscriptionService.GetByID(ctx, orgID, summary.TranscriptionID)
			if err == nil && transcriptions != nil {
				transcriptions[summary.TranscriptionID] = transcription
			}
		}
		if transcription != nil {
			presentation = service.BuildSummaryStructuredPresentation(summary, transcription)
		}
	}
	return presentation.StructuredContent, mcpSummaryCitations(presentation.Citations), mcpSummaryMetadata(presentation.GenerationMetadata)
}

func mcpSummaryCitations(citations []service.SummaryCitation) []mcpSummaryCitation {
	if len(citations) == 0 {
		return nil
	}
	result := make([]mcpSummaryCitation, len(citations))
	for i, citation := range citations {
		result[i] = mcpSummaryCitation{
			ID:           citation.ID,
			Speaker:      citation.Speaker,
			StartSeconds: citation.StartSeconds,
			EndSeconds:   citation.EndSeconds,
		}
	}
	return result
}

func mcpSummaryMetadata(metadata *service.SummaryGenerationMetadata) *mcpSummaryGenerationMetadata {
	if metadata == nil {
		return nil
	}
	return &mcpSummaryGenerationMetadata{
		PromptVersion:           metadata.PromptVersion,
		Model:                   metadata.Model,
		EndpointLocation:        metadata.EndpointLocation,
		SourceVersion:           metadata.SourceVersion,
		SourceHash:              metadata.SourceHash,
		StructuredSchemaVersion: metadata.StructuredSchemaVersion,
		DegradationCodes:        append([]string{}, metadata.DegradationCodes...),
	}
}

// resolveSummaryProfileParam resolves the professional profile for a summary
// the caller is creating. An explicit summary_profile applies only while the
// deployment has summary profiles enabled; otherwise it is ignored and the
// caller's saved preference (or the default profile) applies, so a client that
// always sends its preferred profile keeps working where the feature is off.
// This mirrors POST /api/v1/summaries. Returns a non-nil result when the call
// must fail.
func resolveSummaryProfileParam(
	ctx context.Context,
	deps Dependencies,
	tool, requested string,
) (string, *mcp.CallToolResult) {
	if requested != "" && deps.SummaryProfilesEnabled {
		if err := validateSummaryProfile(requested); err != nil {
			return "", validationError(ctx, err)
		}
		return requested, nil
	}
	profile, err := service.ResolveSummaryProfile(ctx, deps.UserRepo, subjectFromContext(ctx), deps.SummaryProfilesEnabled)
	if err != nil {
		return "", toolError(ctx, deps.Logger, tool, "resolve summary profile", err)
	}
	return profile, nil
}

func handleCreateSummary(ctx context.Context, deps Dependencies, input CreateSummaryInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScopes(ctx, []string{"summary:write", "transcription:read"}); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if deps.SummaryJobInserter == nil {
		return errorResult(fmt.Errorf("summary job queue is not configured")), nil, nil
	}

	highStakes := false
	if input.HighStakes != nil {
		highStakes = *input.HighStakes
	} else if deps.SummaryService.HighStakesEnabled() {
		preferred, prefErr := service.ResolveHighStakesPreference(ctx, deps.UserRepo, subjectFromContext(ctx))
		if prefErr != nil {
			return toolError(ctx, deps.Logger, "create_summary", "resolve high_stakes preference", prefErr), nil, nil
		}
		highStakes = preferred
	}
	summaryProfile, denied := resolveSummaryProfileParam(ctx, deps, "create_summary", input.SummaryProfile)
	if denied != nil {
		return denied, nil, nil
	}

	summary, err := deps.SummaryService.CreateWithProfile(ctx, orgID, input.TranscriptionID, input.SummaryType, highStakes, summaryProfile)
	if err != nil {
		return toolError(ctx, deps.Logger, "create_summary", "create summary", err), nil, nil
	}

	if jobErr := deps.SummaryJobInserter.InsertSummarizeJob(ctx, summary.ID, summary.TranscriptionID, orgID); jobErr != nil {
		deps.Logger.Error("failed to enqueue summary job", "error", jobErr, "summary_id", summary.ID)
		if deleteErr := deps.SummaryService.Delete(ctx, orgID, summary.ID); deleteErr != nil {
			deps.Logger.Error("failed to clean up summary after enqueue failure",
				"error", deleteErr,
				"summary_id", summary.ID,
			)
		}
		return errorResult(fmt.Errorf("failed to start summary job")), nil, nil
	}

	result := struct {
		ID              string `json:"id"`
		TranscriptionID string `json:"transcription_id"`
		SummaryType     string `json:"summary_type"`
		SummaryProfile  string `json:"summary_profile"`
		Status          string `json:"status"`
		HighStakes      bool   `json:"high_stakes"`
		Message         string `json:"message"`
	}{
		ID:              summary.ID,
		TranscriptionID: summary.TranscriptionID,
		SummaryType:     summary.SummaryType,
		SummaryProfile:  domain.NormalizeSummaryProfile(summary.SummaryProfile),
		Status:          summary.Status,
		HighStakes:      summary.HighStakes,
		Message:         "Summary job created. Use get_summary to check status.",
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// registerExportTools adds export-related MCP tools.
func registerExportTools(server *toolRegistry, deps Dependencies) {
	registerTool(server, &mcp.Tool{
		Name:        "export_document",
		Description: "Export a transcription or summary as JSON. Returns the full export data. For PDF/DOCX formats, use the REST API instead.",
	}, wrapTool[ExportDocumentInput, any]("export_document", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input ExportDocumentInput) (*mcp.CallToolResult, any, error) {
			if denied := checkScope(ctx, "export:read"); denied != nil {
				return denied, nil, nil
			}
			orgID, err := orgIDFromContext(ctx)
			if err != nil {
				return errorResult(err), nil, nil
			}

			if input.Format != "json" {
				return errorResult(fmt.Errorf("only json format is supported via MCP; use REST API for pdf/docx")), nil, nil
			}

			switch input.ResourceType {
			case "transcription":
				if denied := checkScope(ctx, "transcription:read"); denied != nil {
					return denied, nil, nil
				}
				return exportTranscription(ctx, deps, orgID, input.ResourceID)
			case "summary":
				if denied := checkScope(ctx, "summary:read"); denied != nil {
					return denied, nil, nil
				}
				return exportSummary(ctx, deps, orgID, input.ResourceID)
			default:
				return errorResult(fmt.Errorf("resource_type must be 'transcription' or 'summary'")), nil, nil
			}
		}))

	registerTool(server, &mcp.Tool{
		Name:        "create_redacted_export",
		Description: "Export transcription or summary content as JSON with deterministic v1 redaction placeholders.",
	}, wrapTool[CreateRedactedExportInput, any]("create_redacted_export", defaultToolTimeout, deps,
		func(ctx context.Context, _ *mcp.CallToolRequest, input CreateRedactedExportInput) (*mcp.CallToolResult, any, error) {
			return handleCreateRedactedExport(ctx, deps, input)
		}))
}

func handleCreateRedactedExport(
	ctx context.Context,
	deps Dependencies,
	input CreateRedactedExportInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "export:read"); denied != nil {
		return denied, nil, nil
	}
	if input.Format != "json" {
		return validationError(ctx, fmt.Errorf("only json format is supported for redacted MCP exports")), nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	switch input.ResourceType {
	case "transcription":
		if denied := checkScope(ctx, "transcription:read"); denied != nil {
			return denied, nil, nil
		}
		return redactedTranscriptionExport(ctx, deps, orgID, input)
	case "summary":
		if denied := checkScope(ctx, "summary:read"); denied != nil {
			return denied, nil, nil
		}
		return redactedSummaryExport(ctx, deps, orgID, input)
	default:
		return validationError(ctx, fmt.Errorf("resource_type must be 'transcription' or 'summary'")), nil, nil
	}
}

func redactedTranscriptionExport(
	ctx context.Context,
	deps Dependencies,
	orgID string,
	input CreateRedactedExportInput,
) (*mcp.CallToolResult, any, error) {
	trans, err := deps.TranscriptionService.GetByID(ctx, orgID, input.ResourceID)
	if err != nil {
		return toolError(ctx, deps.Logger, "create_redacted_export", "get transcription", err), nil, nil
	}
	redacted, err := service.RedactText(trans.FullTranscript, input.RedactionPolicies)
	if err != nil {
		return toolError(ctx, deps.Logger, "create_redacted_export", "redact transcription", err), nil, nil
	}
	r, jErr := jsonResult(struct {
		ResourceType       string   `json:"resource_type"`
		ResourceID         string   `json:"resource_id"`
		Content            string   `json:"content"`
		RedactedCategories []string `json:"redacted_categories"`
		RedactionWarnings  []string `json:"redaction_warnings,omitempty"`
	}{
		ResourceType:       "transcription",
		ResourceID:         trans.ID,
		Content:            redacted.Text,
		RedactedCategories: redacted.Categories,
		RedactionWarnings:  redacted.Warnings,
	})
	return r, nil, jErr
}

func redactedSummaryExport(
	ctx context.Context,
	deps Dependencies,
	orgID string,
	input CreateRedactedExportInput,
) (*mcp.CallToolResult, any, error) {
	summary, err := deps.SummaryService.GetByID(ctx, orgID, input.ResourceID)
	if err != nil {
		return toolError(ctx, deps.Logger, "create_redacted_export", "get summary", err), nil, nil
	}
	redacted, err := service.RedactText(summary.Content, input.RedactionPolicies)
	if err != nil {
		return toolError(ctx, deps.Logger, "create_redacted_export", "redact summary", err), nil, nil
	}
	r, jErr := jsonResult(struct {
		ResourceType       string   `json:"resource_type"`
		ResourceID         string   `json:"resource_id"`
		Content            string   `json:"content"`
		RedactedCategories []string `json:"redacted_categories"`
		RedactionWarnings  []string `json:"redaction_warnings,omitempty"`
	}{
		ResourceType:       "summary",
		ResourceID:         summary.ID,
		Content:            redacted.Text,
		RedactedCategories: redacted.Categories,
		RedactionWarnings:  redacted.Warnings,
	})
	return r, nil, jErr
}

// exportTranscription builds JSON export data for a transcription.
func exportTranscription(ctx context.Context, deps Dependencies, orgID, transcriptionID string) (*mcp.CallToolResult, any, error) {
	trans, err := deps.TranscriptionService.GetByID(ctx, orgID, transcriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "export_document", "get transcription", err), nil, nil
	}

	result := struct {
		ID             string            `json:"id"`
		MediaID        string            `json:"media_id"`
		Status         string            `json:"status"`
		Languages      []string          `json:"languages"`
		SpeakerCount   int               `json:"speaker_count"`
		WordCount      int               `json:"word_count"`
		Duration       float64           `json:"duration_seconds"`
		FullTranscript string            `json:"full_transcript,omitempty"`
		SpeakerMap     map[string]string `json:"speaker_map,omitempty"`
		CreatedAt      string            `json:"created_at"`
	}{
		ID:             trans.ID,
		MediaID:        trans.MediaID,
		Status:         trans.Status,
		Languages:      trans.Languages,
		SpeakerCount:   trans.SpeakerCount,
		WordCount:      trans.WordCount,
		Duration:       trans.DurationSeconds,
		FullTranscript: trans.FullTranscript,
		SpeakerMap:     trans.SpeakerMap,
		CreatedAt:      trans.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// exportSummary builds JSON export data for a summary.
func exportSummary(ctx context.Context, deps Dependencies, orgID, summaryID string) (*mcp.CallToolResult, any, error) {
	summary, err := deps.SummaryService.GetByID(ctx, orgID, summaryID)
	if err != nil {
		return toolError(ctx, deps.Logger, "export_document", "get summary", err), nil, nil
	}

	result := struct {
		ID              string `json:"id"`
		TranscriptionID string `json:"transcription_id"`
		SummaryType     string `json:"summary_type"`
		Status          string `json:"status"`
		Content         string `json:"content,omitempty"`
		WordCount       int    `json:"word_count"`
		CreatedAt       string `json:"created_at"`
	}{
		ID:              summary.ID,
		TranscriptionID: summary.TranscriptionID,
		SummaryType:     summary.SummaryType,
		Status:          summary.Status,
		Content:         summary.Content,
		WordCount:       summary.WordCount,
		CreatedAt:       summary.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}

// clampRecentActivityLimit ensures the limit is between 1 and 25, defaulting to 10.
func clampRecentActivityLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 25 {
		return 25
	}
	return limit
}

func handleGetRecentActivity(ctx context.Context, deps Dependencies, input GetRecentActivityInput) (*mcp.CallToolResult, any, error) {
	if denied := checkScopes(ctx, []string{"transcription:read", "summary:read"}); denied != nil {
		return denied, nil, nil
	}
	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	limit := clampRecentActivityLimit(input.Limit)

	items, err := deps.TranscriptionService.GetRecentActivity(ctx, orgID, limit)
	if err != nil {
		return toolError(ctx, deps.Logger, "get_recent_activity", "get recent activity", err), nil, nil
	}

	type summaryStatusItem struct {
		SummaryType string `json:"summary_type"`
		Status      string `json:"status"`
	}

	type activityItem struct {
		ID               string              `json:"id"`
		Status           string              `json:"status"`
		SourceType       string              `json:"source_type"`
		MediaFilename    string              `json:"media_filename,omitempty"`
		MediaTitle       string              `json:"media_title,omitempty"`
		MediaDescription string              `json:"media_description,omitempty"`
		Languages        []string            `json:"languages"`
		SpeakerCount     int                 `json:"speaker_count"`
		WordCount        int                 `json:"word_count"`
		Duration         float64             `json:"duration_seconds"`
		EnhanceAudio     bool                `json:"enhance_audio"`
		CreatedAt        string              `json:"created_at"`
		CompletedAt      *string             `json:"completed_at,omitempty"`
		SummaryStatuses  []summaryStatusItem `json:"summary_statuses"`
	}

	resultItems := make([]activityItem, len(items))
	for i, item := range items {
		ai := activityItem{
			ID:           item.Transcription.ID,
			Status:       item.Transcription.Status,
			SourceType:   "media",
			Languages:    item.Transcription.Languages,
			SpeakerCount: item.Transcription.SpeakerCount,
			WordCount:    item.Transcription.WordCount,
			Duration:     item.Transcription.DurationSeconds,
			EnhanceAudio: item.Transcription.EnhanceAudio,
			CreatedAt:    item.Transcription.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}

		if item.Transcription.CompletedAt != nil {
			s := item.Transcription.CompletedAt.Format("2006-01-02T15:04:05Z")
			ai.CompletedAt = &s
		}
		ai.MediaFilename = item.Transcription.MediaFilename
		ai.MediaTitle = item.Transcription.MediaTitle
		ai.MediaDescription = item.Transcription.MediaDescription

		// Summary statuses.
		ai.SummaryStatuses = make([]summaryStatusItem, len(item.SummaryStatuses))
		for j, ss := range item.SummaryStatuses {
			ai.SummaryStatuses[j] = summaryStatusItem{
				SummaryType: ss.SummaryType,
				Status:      ss.Status,
			}
		}

		resultItems[i] = ai
	}

	result := struct {
		Items []activityItem `json:"items"`
		Count int            `json:"count"`
	}{
		Items: resultItems,
		Count: len(resultItems),
	}

	r, jErr := jsonResult(result)
	return r, nil, jErr
}
