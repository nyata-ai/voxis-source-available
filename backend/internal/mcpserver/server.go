// Package mcpserver provides a remote MCP (Model Context Protocol) server
// that exposes Voxis API operations as MCP tools over Streamable HTTP.
package mcpserver

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// Dependencies holds all service dependencies needed by MCP tools.
type Dependencies struct {
	MediaService           *service.MediaService
	TranscriptionService   *service.TranscriptionService
	SummaryService         *service.SummaryService
	UserRepo               port.UserRepository
	UsageRepo              port.UsageRepository
	MCPService             *service.MCPService
	TranscriptAnswerer     port.TranscriptAnswerer
	ExportFormatter        port.DocumentFormatter
	OrgService             *service.OrganizationService
	JobInserter            port.TranscriptionJobInserter
	SummaryJobInserter     port.SummaryJobInserter
	SummaryProfilesEnabled bool
	// CustomVocabularyEnabled mirrors CUSTOM_VOCABULARY_ENABLED.
	CustomVocabularyEnabled bool
	EnhanceAudioAvailable   bool
	PublicBaseURL           string
	// AllowedOrigins mirrors the list handed to middleware.ValidateOrigin on
	// the /mcp chain. Since go-sdk v1.4.0 the Streamable HTTP handler runs its
	// own net/http CrossOriginProtection underneath us, so both layers have to
	// agree on the same allowlist or a permitted Origin gets a 403 from the
	// inner one. Empty means the deployment sets no Origin restriction at all
	// (main.go installs ValidateOrigin only for a non-empty list).
	AllowedOrigins []string
	Logger         *slog.Logger
	// TranscriptionCreateLimiter meters create_transcription per user. It is
	// the same limiter as REST POST /transcriptions, so the two share a quota.
	TranscriptionCreateLimiter *middleware.RateLimiter
}

// schemaCache is a package-level cache shared across all stateless server
// instances. This avoids repeated reflection-based schema generation.
var schemaCache = mcp.NewSchemaCache()

// New creates an HTTP handler for the MCP server using Streamable HTTP transport.
// The handler runs in stateless mode — each request creates a temporary session.
func New(deps Dependencies) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return newServer(deps)
		},
		&mcp.StreamableHTTPOptions{
			Stateless:             true,
			JSONResponse:          true,
			Logger:                logger,
			CrossOriginProtection: crossOriginProtection(deps.AllowedOrigins, logger),
		},
	)

	return handler
}

// crossOriginProtection builds the SDK's cross-origin guard so it enforces the
// SAME allowlist as middleware.ValidateOrigin on the /mcp chain.
//
// net/http's CrossOriginProtection allows any request that carries neither
// Origin nor Sec-Fetch-Site, so server-to-server MCP clients are unaffected
// either way; this only governs browser-originated calls.
//
// A malformed configured origin is logged and NOT trusted: that fails closed
// (requests from it get a 403) rather than widening the allowlist on a typo.
func crossOriginProtection(allowedOrigins []string, logger *slog.Logger) *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()

	if len(allowedOrigins) == 0 {
		// No Origin restriction is configured for this deployment, so the
		// chain has no ValidateOrigin either. Bypass here too rather than
		// silently tightening /mcp as a side effect of an SDK upgrade.
		protection.AddInsecureBypassPattern("/")
		return protection
	}

	for _, origin := range allowedOrigins {
		if err := protection.AddTrustedOrigin(origin); err != nil {
			logger.Error("mcp: ignoring malformed allowed origin",
				"origin", origin, "error", err)
		}
	}
	return protection
}

// newServer creates a new MCP server instance with all tools registered.
func newServer(deps Dependencies) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "voxis",
			Version: "1.0.0",
			Title:   "Voxis Transcription Platform",
		},
		&mcp.ServerOptions{
			Instructions: "Voxis MCP server provides tools for managing audio transcriptions, " +
				"AI summaries, citation-grounded transcript analysis, collections, and " +
				"document exports. All operations are scoped to the authenticated " +
				"organization. Read voxis://api/info or voxis://api/capabilities for " +
				"tool workflows, scopes, and safety guidance.",
			SchemaCache: schemaCache,
		},
	)

	tools := &toolRegistry{server: server}
	registerTools(tools, deps)
	registerResources(server, tools.names)

	return server
}
