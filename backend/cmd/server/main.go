package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	apispec "github.com/voxis/backend/api"
	"github.com/voxis/backend/internal/adapter/clamav"
	"github.com/voxis/backend/internal/adapter/deepfilter"
	exportadapter "github.com/voxis/backend/internal/adapter/export"
	"github.com/voxis/backend/internal/adapter/ffmpeg"
	"github.com/voxis/backend/internal/adapter/ffprobe"
	"github.com/voxis/backend/internal/adapter/gemma"
	"github.com/voxis/backend/internal/adapter/localfs"
	"github.com/voxis/backend/internal/adapter/postgres"
	"github.com/voxis/backend/internal/adapter/riverjob"
	"github.com/voxis/backend/internal/adapter/speechmatics"
	"github.com/voxis/backend/internal/adapter/streamtoken"
	"github.com/voxis/backend/internal/adapter/system"
	"github.com/voxis/backend/internal/adapter/vault"
	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
	"github.com/voxis/backend/internal/worker"
)

const apiJSONMaxBodyBytes int64 = 64 << 10

// Pre-authentication ceiling per client IP on /api/* and /mcp: 1200 requests
// per minute with room for page-load bursts. It keeps a flood of forged
// tokens from turning into JWKS refetches against Keycloak while leaving an
// office of users behind one NAT address well below the limit. Client IPs
// come from TRUSTED_PROXIES.
const (
	apiPerIPRequestsPerSecond = 20
	apiPerIPBurst             = 300
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == purgeUserCommand {
		os.Exit(runPurgeUser(os.Args[2:], os.Stdout, os.Stderr))
	}
	if err := run(); err != nil {
		slog.Error("application error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadOSS()
	if err != nil {
		return fmt.Errorf("load OSS configuration: %w", err)
	}
	logger := config.SetupLogger(cfg.Environment)
	slog.SetDefault(logger)
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	if !cfg.Media.SandboxRequired {
		logger.Warn("MEDIA_SANDBOX=disabled: uploaded media is parsed without the bubblewrap sandbox when bubblewrap is unavailable")
	}
	warnIfGemmaEndpointPublic(context.Background(), logger, cfg.Gemma.BaseURL, net.DefaultResolver.LookupIPAddr)
	if migrationErr := runAppMigrations(cfg, logger, newAppMigrator); migrationErr != nil {
		return fmt.Errorf("migrate OSS schema: %w", migrationErr)
	}

	deps, err := initDependencies(cfg, logger)
	if err != nil {
		return err
	}
	defer deps.close()
	cleanupStaleTempFiles(logger)

	router, err := setupRouter(cfg, deps, logger)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.BindAddress, cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	return serve(server, deps.river, logger)
}

func serve(server *http.Server, riverClient *river.Client[pgx.Tx], logger *slog.Logger) (returnErr error) {
	if riverClient != nil {
		if err := riverClient.Start(context.Background()); err != nil {
			return fmt.Errorf("start job queue: %w", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := riverClient.Stop(ctx); err != nil {
				if returnErr == nil {
					returnErr = fmt.Errorf("stop job queue: %w", err)
					return
				}
				logger.Error("stop job queue after server error", "error", err)
			}
		}()
	}
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		return fmt.Errorf("server: %w", err)
	case receivedSignal := <-quit:
		logger.Info("received shutdown signal", "signal", receivedSignal.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}

type dependencies struct {
	pool        *postgres.Pool
	orgRepo     port.OrganizationRepository
	userRepo    port.UserRepository
	mediaRepo   port.MediaRepository
	transRepo   port.TranscriptionRepository
	segmentRepo port.TranscriptionSegmentRepository
	smTrans     port.SpeechmaticsTranscriptionRepository
	smSegments  port.SpeechmaticsSegmentRepository
	summaryRepo port.SummaryRepository
	dashboard   port.DashboardRepository
	usage       port.UsageRepository
	quota       port.StorageQuotaRepository
	promptRepo  port.LLMPromptRepository
	apiKeys     port.APIKeyRepository
	mcpRepo     port.MCPRepository
	adminRepo   *postgres.AdminRepository
	recordings  port.RecordingRepository
	chunks      port.ChunkRepository
	storage     port.StorageClient
	localDir    string
	vault       port.VaultClient
	speech      port.TranscriptionProvider
	gemma       *gemma.Client
	river       *river.Client[pgx.Tx]
	close       func()
}

func initDependencies(cfg *config.OSSConfig, logger *slog.Logger) (*dependencies, error) {
	if strings.TrimSpace(cfg.Database.URL) == "" {
		return nil, errors.New("DATABASE_URL is required by Voxis Source-Available")
	}
	localDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_DIR"))
	if localDir == "" {
		return nil, errors.New("LOCAL_STORAGE_DIR is required by Voxis Source-Available")
	}
	poolCfg := postgres.DefaultPoolConfig(cfg.Database.URL)
	maxConns, maxConnsErr := boundedPoolConnectionCount(cfg.Database.MaxOpenConns, 1, 100)
	if maxConnsErr != nil {
		return nil, fmt.Errorf("DATABASE_MAX_OPEN_CONNS: %w", maxConnsErr)
	}
	poolCfg.MaxConns = maxConns
	minConns, minConnsErr := boundedPoolConnectionCount(cfg.Database.MaxIdleConns, 0, 50)
	if minConnsErr != nil {
		return nil, fmt.Errorf("DATABASE_MAX_IDLE_CONNS: %w", minConnsErr)
	}
	poolCfg.MinConns = minConns
	if poolCfg.MinConns > poolCfg.MaxConns {
		poolCfg.MinConns = poolCfg.MaxConns
	}
	poolCfg.MaxConnLifetime = time.Duration(cfg.Database.ConnMaxLifetime) * time.Second
	poolCfg.StatementTimeout = time.Duration(cfg.Database.StatementTimeoutMS) * time.Millisecond
	poolCfg.IdleInTxnTimeout = time.Duration(cfg.Database.IdleInTxnTimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}

	storage, err := localfs.New(localDir, logger)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create local storage: %w", err)
	}
	vaultClient, err := newVaultClient(cfg, logger)
	if err != nil {
		closeWithWarning(logger, "local storage", storage)
		pool.Close()
		return nil, err
	}
	smConfig, err := speechmatics.LoadConfig()
	if err != nil {
		closeWithWarning(logger, "transit client", vaultClient)
		closeWithWarning(logger, "local storage", storage)
		pool.Close()
		return nil, fmt.Errorf("load Speechmatics configuration: %w", err)
	}
	speechClient, err := speechmatics.NewClient(smConfig, speechmatics.WithLogger(logger))
	if err != nil {
		closeWithWarning(logger, "transit client", vaultClient)
		closeWithWarning(logger, "local storage", storage)
		pool.Close()
		return nil, fmt.Errorf("create Speechmatics client: %w", err)
	}
	promptRepo := postgres.NewLLMPromptRepository(pool.Pool)
	gemmaClient, err := gemma.NewClient(cfg.Gemma, gemma.WithPromptRepository(promptRepo))
	if err != nil {
		closeWithWarning(logger, "transit client", vaultClient)
		closeWithWarning(logger, "local storage", storage)
		pool.Close()
		return nil, fmt.Errorf("create Gemma client: %w", err)
	}
	transRepo := postgres.NewTranscriptionRepository(pool.Pool)
	adminRepo := postgres.NewAdminRepository(pool.Pool)
	return &dependencies{
		pool:        pool,
		orgRepo:     postgres.NewOrganizationRepository(pool.Pool),
		userRepo:    postgres.NewUserRepository(pool.Pool),
		mediaRepo:   postgres.NewMediaRepository(pool.Pool),
		transRepo:   transRepo,
		segmentRepo: transRepo,
		smTrans:     transRepo,
		smSegments:  transRepo,
		summaryRepo: postgres.NewSummaryRepository(pool.Pool),
		dashboard:   postgres.NewDashboardRepository(pool.Pool),
		usage:       postgres.NewUsageRepository(pool.Pool),
		quota:       postgres.NewStorageQuotaRepository(pool.Pool),
		promptRepo:  promptRepo,
		apiKeys:     postgres.NewAPIKeyRepository(pool.Pool),
		mcpRepo:     postgres.NewMCPRepository(pool.Pool),
		adminRepo:   adminRepo,
		recordings:  postgres.NewRecordingRepository(pool.Pool),
		chunks:      postgres.NewChunkRepository(pool.Pool),
		storage:     storage,
		localDir:    localDir,
		vault:       vaultClient,
		speech:      speechClient,
		gemma:       gemmaClient,
		close: func() {
			closeWithWarning(logger, "transit client", vaultClient)
			closeWithWarning(logger, "local storage", storage)
			pool.Close()
		},
	}, nil
}

func newVaultClient(cfg *config.OSSConfig, logger *slog.Logger) (*vault.Client, error) {
	client, err := vault.NewClient(vault.Config{
		Address: cfg.Vault.Address, Token: cfg.Vault.Token, RoleID: cfg.Vault.RoleID,
		SecretID: cfg.Vault.SecretID, MountPath: cfg.Vault.MountPath, CacheTTL: cfg.Encryption.KEKCacheTTL,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("create transit client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		closeWithWarning(logger, "transit client", client)
		return nil, fmt.Errorf("verify transit client: %w", err)
	}
	return client, nil
}

func boundedPoolConnectionCount(value int, minimum, maximum int32) (int32, error) {
	if value < int(minimum) {
		if minimum > 0 {
			return 0, errors.New("must be positive")
		}
		return 0, errors.New("must not be negative")
	}
	for candidate := minimum; candidate < maximum; candidate++ {
		if value == int(candidate) {
			return candidate, nil
		}
	}
	return maximum, nil
}

func closeWithWarning(logger *slog.Logger, name string, closer io.Closer) {
	if err := closer.Close(); err != nil {
		logger.Warn("close dependency", "dependency", name, "error", err)
	}
}

func setupRouter(cfg *config.OSSConfig, deps *dependencies, logger *slog.Logger) (*gin.Engine, error) {
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}
	router.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	router.Use(middleware.RequestID(), middleware.SecurityHeadersWithConfig(middleware.SecurityConfig{
		EnableHSTS: cfg.Environment == "production",
	}), middleware.Logging(logger), middleware.Recovery(logger))
	router.Use(middleware.NewRateLimiter(apiPerIPRequestsPerSecond, apiPerIPBurst).PathPrefixMiddleware("/api/", "/mcp"))
	health := handler.NewHealthHandler(deps.pool, deps.vault, logger)
	router.GET("/health", health.Liveness)
	router.GET("/ready", health.Readiness)
	router.GET("/api/v1/openapi.json", handler.ServeOpenAPI(apispec.OpenAPISpec, cfg.PublicBaseURL))

	if cfg.Auth.IssuerURL() == "" || cfg.Auth.ClientID == "" {
		return nil, errors.New("OSS Keycloak issuer, realm, and API client ID are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	verifier, err := buildOIDCVerifier(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create OIDC verifier: %w", err)
	}

	orgService := service.NewOrganizationService(deps.orgRepo, deps.userRepo, deps.vault, logger,
		service.WithMissingExistingKEKRecovery(false))
	envelope := crypto.NewEnvelopeService(deps.vault, logger)
	mediaService := service.NewMediaService(deps.mediaRepo, deps.transRepo, deps.summaryRepo,
		deps.storage, envelope, nil, logger, service.WithStorageQuota(deps.quota, "localfs"),
		service.WithMaxMediaDuration(cfg.Media.MaxDuration))
	transService := service.NewTranscriptionService(deps.transRepo, deps.mediaRepo, deps.summaryRepo, envelope, logger)
	segmentService := service.NewSegmentTranscriptionService(deps.transRepo, deps.segmentRepo, transService, logger)
	summaryService := service.NewSummaryService(deps.summaryRepo, deps.transRepo, deps.mediaRepo, envelope, logger,
		service.WithHighStakesEnabled(cfg.SummaryHighStakesEnabled))

	prober := ffprobe.New(logger, ffprobe.WithSandboxRequired(cfg.Media.SandboxRequired))
	stitcher := ffmpeg.New(logger, ffmpeg.WithTimeout(time.Duration(cfg.Recording.StitchFFmpegTimeoutMin)*time.Minute),
		ffmpeg.WithSandboxRequired(cfg.Media.SandboxRequired), ffmpeg.WithMaxOutputDuration(ffmpegOutputCap(cfg)))
	if !prober.Available() || !stitcher.Available() {
		return nil, errors.New("ffprobe and ffmpeg are required by Voxis Source-Available")
	}
	preprocessor, enhancedPreprocessor, err := newOSSPreprocessors(cfg, logger)
	if err != nil {
		return nil, err
	}
	policy := service.RecordingPolicy{
		MaxDuration:          cfg.Recording.MaxDuration,
		MaxChunksPerSession:  cfg.Recording.MaxChunksPerSession,
		OrphanThreshold:      time.Duration(cfg.Recording.OrphanThresholdMinutes) * time.Minute,
		StitchServiceTimeout: time.Duration(cfg.Recording.StitchServiceTimeoutMin) * time.Minute,
	}
	recordingService := service.NewRecordingService(deps.recordings, deps.chunks, mediaService, deps.storage,
		envelope, prober, stitcher, nil, logger, service.WithRecordingPolicy(policy),
		service.WithRecordingStorageQuota(deps.quota, "localfs"))

	var scanner port.MalwareScanner
	if cfg.ClamAVAddress != "" {
		scanner = clamav.New(logger, clamav.WithAddress(cfg.ClamAVAddress))
	} else if cfg.ScanEnforcement == config.ScanEnforcementRequired {
		return nil, errors.New("CLAMAV_ADDRESS is required when SCAN_ENFORCEMENT=required")
	}
	recordingService.SetMalwareScanner(scanner)
	systemCollector := system.NewCollector(system.Config{
		Pool:           deps.pool.Pool,
		StorageBackend: "localfs",
		DepChecks: []system.DependencyCheck{
			{Name: "postgres", Probe: deps.pool.Pool.Ping},
			{Name: "vault", Probe: deps.vault.Ping},
			{Name: "storage", Probe: deps.storage.Ping},
			{Name: "clamav", Probe: clamAVProbe(scanner)},
			{Name: "keycloak", Probe: system.HTTPProbe(cfg.Auth.FetchURL() + "/.well-known/openid-configuration")},
		},
		Now: time.Now, Logger: logger,
	})
	retentionService := service.NewRecordingRetentionPolicyService(deps.adminRepo, time.Now)
	if workersErr := startWorkers(cfg, deps, envelope, transService, segmentService, summaryService,
		recordingService, prober, stitcher, preprocessor, enhancedPreprocessor, scanner, logger); workersErr != nil {
		return nil, workersErr
	}
	inserter := riverjob.NewInserter(deps.river)
	recordingService.SetJobInserter(inserter)
	mediaService.SetScanInserter(inserter)
	transService.ApplyOptions(service.WithSummaryGeneration(summaryService, inserter),
		service.WithSummaryPreferences(deps.userRepo), service.WithSummaryProfilesEnabled(cfg.SummaryProfilesEnabled))
	if provider, ok := any(deps.gemma).(port.SpeakerSuggestionProvider); ok {
		transService.ApplyOptions(service.WithSpeakerSuggestionProvider(provider))
	}

	streamSigner, err := streamtoken.NewHMACSigner(cfg.MediaStreamTokenSecret)
	if err != nil {
		return nil, fmt.Errorf("create stream token signer: %w", err)
	}
	mediaHandler := handler.NewMediaHandler(mediaService, orgService, prober, streamSigner,
		time.Duration(cfg.MediaStreamTokenTTL)*time.Second, middleware.NewRateLimiter(10, 20),
		middleware.NewRateLimiter(5.0/60.0, 5), logger)
	mediaHandler.ConfigureLocalUploadSpool(deps.localDir, cfg.UploadMaxConcurrent)
	transHandler := handler.NewTranscriptionHandler(transService, orgService, inserter, true, logger)
	transHandler.SetCustomVocabularyEnabled(false)
	summaryHandler := handler.NewSummaryHandler(summaryService, transService, orgService, deps.userRepo, inserter, logger)
	summaryHandler.SetSummaryProfilesEnabled(cfg.SummaryProfilesEnabled)
	summaryHandler.SetBillableRateLimiter(middleware.NewRateLimiter(float64(len(domain.DefaultSummaryTypes))/60, len(domain.DefaultSummaryTypes)*2))
	documents := exportadapter.NewDocumentFormatterWithLogger(logger)
	var bapFormatter port.ExaminationRecordFormatter
	if cfg.BAPExportEnabled {
		bapFormatter = documents
	}
	exportHandler := handler.NewExportHandler(transService, summaryService, orgService, mediaService, documents, bapFormatter, logger)
	apiKeys := service.NewAPIKeyService(deps.apiKeys, logger)
	if ownerErr := configureAPIKeyOwnerCheck(cfg, apiKeys, logger); ownerErr != nil {
		return nil, ownerErr
	}
	mcpService := service.NewMCPService(deps.mcpRepo, logger,
		service.WithMCPTranscriptionService(transService), service.WithMCPTranscriptAnswerer(deps.gemma))

	registerProtectedRoutes(router, cfg, deps, verifier, orgService, mediaService, transService, summaryService,
		mediaHandler, transHandler, summaryHandler, exportHandler, recordingService, retentionService, systemCollector,
		apiKeys, mcpService, documents, enhancedPreprocessor != nil, logger)
	return router, nil
}

func newOSSPreprocessors(cfg *config.OSSConfig, logger *slog.Logger) (defaultPreprocessor, enhancedPreprocessor port.AudioPreprocessor, err error) {
	var enhanced port.AudioPreprocessor
	if cfg.DeepFilterEnabled {
		opts := []deepfilter.Option{deepfilter.WithSandboxRequired(cfg.Media.SandboxRequired)}
		if cfg.DeepFilterBinPath != "" {
			opts = append(opts, deepfilter.WithBinPath(cfg.DeepFilterBinPath))
		}
		candidate := deepfilter.NewPreprocessor(logger, opts...)
		if !candidate.Available() {
			return nil, nil, errors.New("DeepFilterNet is enabled but unavailable")
		}
		enhanced = candidate
	}
	preprocessor := ffmpeg.NewPreprocessor(logger,
		ffmpeg.WithPreprocessorSandboxRequired(cfg.Media.SandboxRequired),
		ffmpeg.WithPreprocessorMaxOutputDuration(ffmpegOutputCap(cfg)),
		ffmpeg.WithOutputSampleRate(ossPreprocessorSampleRate(enhanced != nil)))
	if !preprocessor.Available() {
		return nil, nil, errors.New("ffmpeg preprocessor is required by Voxis Source-Available")
	}
	return preprocessor, enhanced, nil
}

// ffmpegOutputCap is the -t cap for every ffmpeg output: the media limit plus
// the grace that keeps accepted media and finished recordings untruncated.
func ffmpegOutputCap(cfg *config.OSSConfig) time.Duration {
	return cfg.Media.MaxDuration + domain.MediaDurationGrace
}

func ossPreprocessorSampleRate(enhancementEnabled bool) int {
	if enhancementEnabled {
		return 48000
	}
	return 16000
}

func startWorkers(cfg *config.OSSConfig, deps *dependencies, envelope *crypto.EnvelopeService,
	transService *service.TranscriptionService, segmentService *service.SegmentTranscriptionService,
	summaryService *service.SummaryService, recordingService *service.RecordingService,
	prober port.AudioProber, stitcher *ffmpeg.Stitcher, preprocessor, enhancedPreprocessor port.AudioPreprocessor,
	scanner port.MalwareScanner, logger *slog.Logger) error {
	workers := river.NewWorkers()
	river.AddWorker(workers, worker.NewTranscribeWorker(worker.TranscribeWorkerConfig{
		SandboxRequired: cfg.Media.SandboxRequired, MaxMediaDuration: cfg.Media.MaxDuration,
		MediaRepo: deps.mediaRepo, TransRepo: deps.transRepo,
		SegRepo: deps.segmentRepo, Storage: deps.storage, Envelope: envelope, Provider: deps.speech, SpeechmaticsRepo: deps.smTrans,
		SpeechmaticsSegRepo: deps.smSegments, Chunker: stitcher, Prober: prober,
		DefaultPreprocessor: preprocessor, EnhancedPreprocessor: enhancedPreprocessor,
		CustomVocabularyEnabled: false, Logger: logger,
	}))
	river.AddWorker(workers, worker.NewPollSpeechmaticsWorker(deps.smTrans, deps.smSegments, transService, segmentService, deps.speech, logger))
	river.AddWorker(workers, worker.NewCleanSpeechmaticsWorker(deps.smTrans, deps.smSegments, deps.transRepo, deps.speech, logger))
	river.AddWorker(workers, worker.NewSpeechmaticsWaitWorker(deps.smTrans, deps.smSegments, transService, segmentService, deps.speech, logger))
	river.AddWorker(workers, worker.NewSummarizeWorker(deps.summaryRepo, deps.transRepo, envelope, deps.gemma, summaryService, logger,
		worker.WithStructuredOutput(true), worker.WithStructuredHighStakes(cfg.SummaryHighStakesEnabled),
		worker.WithStructuredLegacyFallback(false), worker.WithStructuredSourceChunkBytes(cfg.SummarySourceChunkBytes),
		worker.WithSummaryProfilesEnabled(cfg.SummaryProfilesEnabled), worker.WithSummaryJobTimeout(cfg.Gemma.SummaryJobTimeout)))
	river.AddWorker(workers, worker.NewStitchRecordingWorker(recordingService, logger,
		worker.WithStitchJobTimeout(time.Duration(cfg.Recording.StitchJobTimeoutMin)*time.Minute)))
	river.AddWorker(workers, worker.NewCleanupRecordingWorker(recordingService, logger))
	river.AddWorker(workers, worker.NewScanMediaWorker(deps.mediaRepo, deps.storage, envelope, scanner, logger))

	migrator, err := rivermigrate.New(riverpgxv5.New(deps.pool.Pool), nil)
	if err != nil {
		return fmt.Errorf("create job migrations: %w", err)
	}
	if _, migrateErr := migrator.Migrate(context.Background(), rivermigrate.DirectionUp, nil); migrateErr != nil {
		return fmt.Errorf("migrate job queue: %w", migrateErr)
	}
	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(20*time.Second), func() (river.JobArgs, *river.InsertOpts) { return worker.PollSpeechmaticsJobArgs{}, nil }, &river.PeriodicJobOpts{RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(5*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return worker.CleanSpeechmaticsJobArgs{}, nil }, &river.PeriodicJobOpts{RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(5*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return worker.CleanupRecordingJobArgs{}, nil }, &river.PeriodicJobOpts{}),
	}
	client, err := river.NewClient(riverpgxv5.New(deps.pool.Pool), newRiverConfig(workers, periodic))
	if err != nil {
		return fmt.Errorf("create job queue: %w", err)
	}
	deps.river = client
	return nil
}

func newRiverConfig(workers *river.Workers, periodic []*river.PeriodicJob) *river.Config {
	return &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault:          {MaxWorkers: 20},
			"transcription":             {MaxWorkers: 5},
			"summarization":             {MaxWorkers: 1},
			"stitching":                 {MaxWorkers: 2},
			"scanning":                  {MaxWorkers: 2},
			worker.MaintenanceQueueName: {MaxWorkers: 1},
		},
		Workers: workers, PeriodicJobs: periodic,
	}
}
