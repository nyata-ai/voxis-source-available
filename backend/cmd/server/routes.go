package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	oidcadapter "github.com/voxis/backend/internal/adapter/oidc"
	"github.com/voxis/backend/internal/adapter/riverjob"
	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/handler"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/mcpserver"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// Per-user ceiling on starting transcriptions, REST and MCP combined. Every
// start is billed by Speechmatics and deleting a transcription refunds
// nothing, so create-delete-create must not loop without bound.
const (
	transcriptionCreatesPerHour = 30
	transcriptionCreateBurst    = 10
)

func registerProtectedRoutes(
	router *gin.Engine,
	cfg *config.OSSConfig,
	deps *dependencies,
	verifier port.TokenVerifier,
	orgService *service.OrganizationService,
	mediaService *service.MediaService,
	transService *service.TranscriptionService,
	summaryService *service.SummaryService,
	mediaHandler *handler.MediaHandler,
	transHandler *handler.TranscriptionHandler,
	summaryHandler *handler.SummaryHandler,
	exportHandler *handler.ExportHandler,
	recordingService *service.RecordingService,
	retentionService *service.RecordingRetentionPolicyService,
	systemCollector port.SystemStatsCollector,
	apiKeys *service.APIKeyService,
	mcpService *service.MCPService,
	documents port.DocumentFormatter,
	enhanceAudioAvailable bool,
	logger *slog.Logger,
) {
	apiBase := router.Group("/api/v1")
	if cfg.PublicBaseURL != "" {
		router.GET("/.well-known/oauth-protected-resource", handler.OAuthProtectedResourceMetadata(handler.OAuthMetadataConfig{
			ResourceURL: cfg.PublicBaseURL + "/mcp", AuthServerURLs: []string{cfg.Auth.IssuerURL()},
			ScopesSupported: mcpScopes, ResourceName: "Voxis Source-Available MCP Server",
		}))
	}

	promptService := service.NewLLMPromptServiceWithAllowedModels(deps.promptRepo,
		func() []port.LLMPromptConfig { return deps.gemma.DefaultSummaryPromptConfigs() }, []string{cfg.Gemma.Model})
	admin := apiBase.Group("/admin")
	admin.Use(adminAuthChain(verifier, cfg.Auth.ClientID, cfg.Auth.AdminAllowedClients)...)
	adminAuth := handler.NewAdminAuthHandler()
	adminPrompts := handler.NewAdminLLMHandler(promptService, logger)
	adminOps := handler.NewAdminOpsHandler(newOSSAdminRepository(deps.adminRepo, cfg.Gemma), time.Now, logger)
	adminRetention := handler.NewAdminRecordingRetentionHandler(retentionService, logger)
	adminSystem := handler.NewAdminSystemHandler(systemCollector, logger)
	adminQuota := handler.NewAdminStorageQuotaHandler(deps.quota, "localfs")
	admin.GET("/me", adminAuth.Me)
	admin.GET("/ops/stats", adminOps.GetStats)
	admin.GET("/system/stats", adminSystem.GetStats)
	admin.GET("/llm/prompts", adminPrompts.ListPrompts)
	admin.PUT("/llm/prompts/:key", adminPrompts.UpdatePrompt)
	admin.DELETE("/llm/prompts/:key", adminPrompts.ResetPrompt)
	admin.GET("/recording-retention", adminRetention.Get)
	admin.POST("/recording-retention/preview", adminRetention.Preview)
	admin.PUT("/recording-retention", adminRetention.Update)
	admin.GET("/storage-quota", adminQuota.Get)
	admin.PUT("/storage-quota", adminQuota.Update)

	auth := middleware.DualAuth(verifier, apiKeys)
	api := apiBase.Group("")
	api.Use(restAuthChain(auth, cfg.Auth.ClientID, cfg.Auth.AdminAllowedClients)...)
	api.Use(jsonAPIBodyCap(apiJSONMaxBodyBytes))
	transcriptionCreates := middleware.NewRateLimiter(transcriptionCreatesPerHour/3600.0, transcriptionCreateBurst)
	registerAPIRoutes(api, cfg, deps, orgService, mediaHandler, transHandler, summaryHandler,
		exportHandler, recordingService, apiKeys, mcpService, transcriptionCreates, enhanceAudioAvailable, logger)

	// Browsers cannot add Authorization to an audio element request. Stream uses
	// a separately signed, short-lived token that MediaHandler validates.
	router.GET("/api/v1/media/:id/stream", middleware.ExtendWriteDeadline(20*time.Minute), mediaHandler.Stream)
	mcpHandler := mcpserver.New(mcpserver.Dependencies{
		MediaService: mediaService, TranscriptionService: transService, SummaryService: summaryService,
		UserRepo: deps.userRepo, UsageRepo: deps.usage, MCPService: mcpService,
		TranscriptAnswerer: deps.gemma, ExportFormatter: documents, OrgService: orgService,
		JobInserter: riverjob.NewInserter(deps.river), SummaryJobInserter: riverjob.NewInserter(deps.river),
		SummaryProfilesEnabled: cfg.SummaryProfilesEnabled, CustomVocabularyEnabled: false,
		EnhanceAudioAvailable: enhanceAudioAvailable, PublicBaseURL: cfg.PublicBaseURL, AllowedOrigins: cfg.MCPAllowedOrigins, Logger: logger,
		TranscriptionCreateLimiter: transcriptionCreates,
	})
	router.Any("/mcp", buildMCPMiddlewareChain(mcpChainConfig{
		Logger: logger, AuthMiddleware: auth, OrgService: orgService,
		PRMURL: handler.MCPProtectedResourceMetadataURL(cfg.PublicBaseURL), ResourceAudience: cfg.MCPResourceAudience,
		AllowedOrigins: cfg.MCPAllowedOrigins, MaxBodyBytes: cfg.MCPMaxBodyBytes, MCPHandler: mcpHandler,
	})...)
}

func registerAPIRoutes(
	api *gin.RouterGroup,
	cfg *config.OSSConfig,
	deps *dependencies,
	orgService *service.OrganizationService,
	mediaHandler *handler.MediaHandler,
	transHandler *handler.TranscriptionHandler,
	summaryHandler *handler.SummaryHandler,
	exportHandler *handler.ExportHandler,
	recordingService *service.RecordingService,
	apiKeys *service.APIKeyService,
	mcpService *service.MCPService,
	transcriptionCreates *middleware.RateLimiter,
	enhanceAudioAvailable bool,
	logger *slog.Logger,
) {
	api.GET("/me", handler.NewUserHandler(orgService, logger).Me)
	api.GET("/dashboard/stats", middleware.RequireScope("media:read"), handler.NewDashboardHandler(deps.dashboard, orgService, logger).GetStats)
	api.GET("/usage/stats", middleware.RequireScope("media:read"), handler.NewUsageHandler(deps.usage, orgService, 0, 0, logger,
		handler.WithUserStorageQuota(deps.quota, "localfs")).GetStats)
	api.GET("/search", middleware.RequireScope("media:read"), middleware.RequireScope("transcription:read"), handler.NewOSSSearchHandler(newOSSSearchService(mcpService), orgService, logger).Search)

	api.POST("/media/upload", middleware.RequireScope("media:write"), middleware.ExtendWriteDeadline(20*time.Minute), mediaHandler.Upload)
	api.GET("/media", middleware.RequireScope("media:read"), mediaHandler.List)
	api.GET("/media/:id", middleware.RequireScope("media:read"), mediaHandler.GetByID)
	api.GET("/media/:id/download", middleware.RequireScope("media:read"), middleware.ExtendWriteDeadline(20*time.Minute), mediaHandler.Download)
	api.PATCH("/media/:id", middleware.RequireScope("media:write"), mediaHandler.UpdateMetadata)
	// Deleting media also deletes its transcriptions and summaries.
	api.DELETE("/media/:id", middleware.RequireScope("media:write"), middleware.RequireScope("transcription:write"),
		middleware.RequireScope("summary:write"), mediaHandler.Delete)
	api.GET("/media/:id/stream-url", middleware.RequireScope("media:read"), mediaHandler.StreamURL)

	api.POST("/transcriptions", middleware.RequireScope("transcription:write"),
		transcriptionCreates.UserMiddleware(middleware.TranscriptionCreateLimitKey), transHandler.Create)
	api.GET("/transcriptions", middleware.RequireScope("transcription:read"), transHandler.List)
	api.GET("/transcriptions/:id", middleware.RequireScope("transcription:read"), transHandler.GetByID)
	api.DELETE("/transcriptions/:id", append(requireScopes(deleteTranscriptionScopes), transHandler.Delete)...)
	api.PATCH("/transcriptions/:id/speakers", middleware.RequireScope("transcription:write"), transHandler.UpdateSpeakers)
	api.POST("/transcriptions/:id/speakers/suggestions", middleware.RequireScope("transcription:write"), transHandler.GenerateSpeakerSuggestions)
	api.DELETE("/transcriptions/:id/speakers/suggestions/:index", middleware.RequireScope("transcription:write"), transHandler.DismissSpeakerSuggestion)

	api.POST("/summaries", middleware.RequireScope("summary:write"), middleware.RequireScope("transcription:read"), summaryHandler.Create)
	api.GET("/summaries", middleware.RequireScope("summary:read"), summaryHandler.List)
	api.GET("/summaries/:id", middleware.RequireScope("summary:read"), summaryHandler.GetByID)
	api.DELETE("/summaries/:id", middleware.RequireScope("summary:write"), summaryHandler.Delete)
	api.POST("/summaries/:id/regenerate", middleware.RequireScope("summary:write"), middleware.RequireScope("transcription:read"), summaryHandler.Regenerate)
	api.GET("/transcriptions/:id/summaries", middleware.RequireScope("summary:read"), summaryHandler.ListByTranscription)
	api.POST("/transcriptions/:id/summaries", middleware.RequireScope("summary:write"), middleware.RequireScope("transcription:read"), summaryHandler.GenerateAll)

	api.GET("/transcriptions/:id/export", middleware.RequireScope("export:read"), middleware.RequireScope("transcription:read"), exportHandler.ExportTranscription)
	api.GET("/summaries/:id/export", middleware.RequireScope("export:read"), middleware.RequireScope("summary:read"), exportHandler.ExportSummary)
	settings := handler.NewSettingsHandler(orgService, deps.userRepo, logger)
	api.GET("/settings/preferences", settings.GetPreferences)
	api.PUT("/settings/preferences", settings.UpdatePreferences)
	api.GET("/features", middleware.RequireScope("transcription:read"), handler.NewFeaturesHandler(handler.FeatureFlags{
		EnhanceAudio: enhanceAudioAvailable, SummaryHighStakes: cfg.SummaryHighStakesEnabled, SummaryStructuredOutput: true,
		SummaryProfiles: cfg.SummaryProfilesEnabled, SummarySharedAnalysis: cfg.SummarySharedAnalysisEnabled,
		AccountSecurity: false, Billing: false, CustomVocabulary: false, BAPExport: cfg.BAPExportEnabled,
	}).GetFeatures)

	keyHandler := handler.NewAPIKeyHandler(apiKeys, orgService, cfg.PublicBaseURL, logger)
	api.POST("/api-keys", middleware.RequireJWT(), keyHandler.Create)
	api.GET("/api-keys", middleware.RequireJWT(), keyHandler.List)
	api.DELETE("/api-keys/:id", middleware.RequireJWT(), keyHandler.Revoke)

	recordings := handler.NewRecordingHandler(recordingService, orgService, logger, handler.WithPrivilegeRecording(false))
	activity := handler.NewActivityHandler(service.NewActivityService(deps.recordings, deps.transRepo, deps.summaryRepo), orgService, logger)
	api.GET("/activity", middleware.RequireJWT(), activity.GetActivity)
	api.POST("/recordings", middleware.RequireJWT(), recordings.CreateSession)
	api.GET("/recordings/:id", middleware.RequireJWT(), recordings.GetSession)
	api.POST("/recordings/:id/chunks", middleware.RequireJWT(), middleware.ExtendReadDeadline(5*time.Minute), middleware.ExtendWriteDeadline(5*time.Minute), recordings.UploadChunk)
	api.POST("/recordings/:id/complete", middleware.RequireJWT(), recordings.CompleteSession)
	api.PATCH("/recordings/:id/pause", middleware.RequireJWT(), recordings.PauseSession)
	api.PATCH("/recordings/:id/resume", middleware.RequireJWT(), recordings.ResumeSession)
	api.POST("/recordings/:id/heartbeat", middleware.RequireJWT(), recordings.HeartbeatSession)
	api.GET("/recordings/interrupted", middleware.RequireJWT(), recordings.ListInterrupted)
	api.GET("/recordings/active", middleware.RequireJWT(), recordings.GetActiveSession)
	api.POST("/recordings/:id/recover", middleware.RequireJWT(), recordings.RecoverSession)
	api.POST("/recordings/:id/release", middleware.RequireJWT(), recordings.ReleaseSession)
	api.DELETE("/recordings/:id", middleware.RequireJWT(), recordings.AbandonSession)
}

func buildOIDCVerifier(ctx context.Context, cfg *config.OSSConfig) (port.TokenVerifier, error) {
	if len(cfg.Auth.AdditionalAudiences) == 0 {
		return oidcadapter.NewVerifier(ctx, cfg.Auth.FetchURL(), cfg.Auth.IssuerURL(), cfg.Auth.ClientID)
	}
	return oidcadapter.NewVerifierWithAudiences(ctx, cfg.Auth.FetchURL(), cfg.Auth.IssuerURL(), append([]string{cfg.Auth.ClientID}, cfg.Auth.AdditionalAudiences...))
}

func adminAuthChain(verifier port.TokenVerifier, audience string, clients []string) []gin.HandlerFunc {
	return []gin.HandlerFunc{middleware.Auth(verifier), middleware.RequireJWT(), middleware.RequireEmailVerified(), middleware.RequireAudience(audience), middleware.RequireAuthorizedParty(clients), middleware.RequireAdmin()}
}

func restAuthChain(auth gin.HandlerFunc, audience string, clients []string) []gin.HandlerFunc {
	return []gin.HandlerFunc{auth, middleware.RequireRESTAudienceAndClient(audience, clients), middleware.RequireEmailVerified()}
}

var streamingRoutes = map[string]bool{
	"/api/v1/media/upload": true, "/api/v1/recordings/:id/chunks": true,
}

func jsonAPIBodyCap(limit int64) gin.HandlerFunc {
	capBody := middleware.MaxBodyBytes(limit)
	return func(c *gin.Context) {
		if streamingRoutes[c.FullPath()] {
			c.Next()
			return
		}
		capBody(c)
	}
}

// deleteTranscriptionScopes are required of API keys and scoped tokens that
// delete a transcription: the delete also removes its summaries.
var deleteTranscriptionScopes = []string{"transcription:write", "summary:write"}

// requireScopes returns one RequireScope middleware for each scope.
func requireScopes(scopes []string) []gin.HandlerFunc {
	chain := make([]gin.HandlerFunc, 0, len(scopes)+1)
	for _, scope := range scopes {
		chain = append(chain, middleware.RequireScope(scope))
	}
	return chain
}

var mcpScopes = []string{"media:read", "media:write", "transcription:read", "transcription:write", "summary:read", "summary:write", "export:read", "collection:read", "collection:write", "analysis:read", "analysis:write", "email", "profile"}

type mcpChainConfig struct {
	Logger           *slog.Logger
	AuthMiddleware   gin.HandlerFunc
	OrgService       *service.OrganizationService
	PRMURL           string
	ResourceAudience string
	AllowedOrigins   []string
	MaxBodyBytes     int64
	MCPHandler       http.Handler
}

func buildMCPMiddlewareChain(cfg mcpChainConfig) []gin.HandlerFunc {
	preAuth := middleware.NewRateLimiter(2, 20)
	perKey := middleware.NewRateLimiter(0.5, 10)
	perOrg := middleware.NewRateLimiter(2, 20)
	chain := []gin.HandlerFunc{
		middleware.MCPAudit(cfg.Logger), middleware.MCPJSONRPCErrorEnvelope(), middleware.MCPWWWAuthenticate(cfg.PRMURL),
		middleware.MCPProtocolVersion([]string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}),
		preAuth.Middleware(), middleware.MaxBodyBytes(cfg.MaxBodyBytes), cfg.AuthMiddleware, middleware.RequireEmailVerified(),
		middleware.MCPRequireResourceAudience(cfg.ResourceAudience), middleware.ResolveMCPOrg(cfg.OrgService, cfg.Logger),
	}
	if len(cfg.AllowedOrigins) > 0 {
		chain = append(chain, middleware.ValidateOrigin(cfg.AllowedOrigins))
	}
	return append(chain, perKey.MCPMiddleware(), perOrg.MCPOrgMiddleware(), middleware.AuthBridge(), gin.WrapH(cfg.MCPHandler))
}
