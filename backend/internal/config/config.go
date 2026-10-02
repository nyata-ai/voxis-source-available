// Package config loads and validates the bounded Voxis Source-Available runtime settings.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// OSSGemmaModel is the only model that the first OSS release supports.
	// Keeping this value here and in the adapter makes a wrong environment
	// setting fail before a transcript can reach another model.
	OSSGemmaModel = "google/gemma-4-12B-it"
	// OSSGemmaQATModel is Google's official QAT GGUF variant of the supported
	// instruction model. It is a separate allowed identifier, not a prefix
	// match, so an operator cannot redirect transcripts to an arbitrary model.
	OSSGemmaQATModel = "google/gemma-4-12B-it-qat-q4_0-gguf"

	defaultGemmaMaxRequestBytes   = 4 << 20
	defaultGemmaMaxResponseBytes  = 8 << 20
	defaultGemmaMaxContextTokens  = 32768
	defaultGemmaTimeout           = 360 * time.Second
	defaultGemmaSummaryJobTimeout = 60 * time.Minute
	defaultGemmaMaxAttempts       = 3
	maxGemmaTimeoutSeconds        = 600
	maxGemmaSummaryJobTimeoutMin  = 120
	minOSSGemmaContextTokens      = 16384
	maxOSSSummaryChunkBytes       = 16 << 10
	ossSummaryContextReserve      = 12 << 10
	// envExamplePlaceholder is the value infra/oss/.env.example ships for
	// secrets the operator must replace.
	envExamplePlaceholder = "CHANGE_ME"
)

// GemmaConfig describes the bounded, local OpenAI-compatible model endpoint
// used by Voxis-OSS. Revisions are explicit because changing a model image,
// runtime, or quantization can change summary output for the same transcript.
type GemmaConfig struct {
	BaseURL           string
	AllowHTTPInternal bool
	Model             string
	Runtime           string
	ModelRevision     string
	RuntimeRevision   string
	Quantization      string
	MaxRequestBytes   int64
	MaxResponseBytes  int64
	MaxContextTokens  int
	RequestTimeout    time.Duration
	SummaryJobTimeout time.Duration
	MaxAttempts       int
}

// OSSConfig is the edition-owned configuration surface. It embeds the shared
// settings required by retained workflows and never populates Gladia, Vertex,
// URL transcription, billing, or privileged-recording values. The exported
// config overlay replaces the shared type with the reduced OSS-only shape.
type OSSConfig struct {
	Config
	TranscriptionProvider   string
	Gemma                   GemmaConfig
	SummarySourceChunkBytes int
	Media                   MediaProcessingConfig
}

// LoadOSS reads the configuration accepted by the Voxis-OSS composition.
// It does not call Load: the commercial loader intentionally accepts cloud
// providers and product surfaces that an OSS process must not import or start.
func LoadOSS() (*OSSConfig, error) {
	environment := getEnv("ENVIRONMENT", "development")
	corsOrigins, trustedProxies, mcpOrigins, err := loadNetworkAllowlists(environment)
	if err != nil {
		return nil, err
	}

	cfg := &OSSConfig{
		Config: Config{
			Port:        getEnv("PORT", "8080"),
			BindAddress: getEnv("BIND_ADDRESS", "127.0.0.1"),
			Environment: environment,
			Debug:       getEnvBool("DEBUG", false),
			Auth: AuthConfig{
				KeycloakURL:            getEnv("OSS_KEYCLOAK_PUBLIC_URL", ""),
				KeycloakFetchURL:       getEnv("OSS_KEYCLOAK_FETCH_URL", ""),
				Realm:                  getEnv("OSS_KEYCLOAK_REALM", ""),
				ClientID:               getEnv("OSS_KEYCLOAK_API_CLIENT_ID", ""),
				AdditionalAudiences:    parseCSVList(getEnv("OSS_KEYCLOAK_ADDITIONAL_AUDIENCES", "")),
				AdminAllowedClients:    parseCSVList(getEnv("OSS_KEYCLOAK_WEB_CLIENT_ID", "")),
				UserLookupClientID:     strings.TrimSpace(os.Getenv("KEYCLOAK_USER_LOOKUP_CLIENT_ID")),
				UserLookupClientSecret: strings.TrimSpace(os.Getenv("KEYCLOAK_USER_LOOKUP_CLIENT_SECRET")),
			},
			CORSAllowedOrigins: corsOrigins,
			TrustedProxies:     trustedProxies,
			Database: DatabaseConfig{
				URL:                getEnv("DATABASE_URL", ""),
				MaxOpenConns:       getEnvInt("DATABASE_MAX_OPEN_CONNS", 25),
				MaxIdleConns:       getEnvInt("DATABASE_MAX_IDLE_CONNS", 5),
				ConnMaxLifetime:    getEnvInt("DATABASE_CONN_MAX_LIFETIME", 300),
				StatementTimeoutMS: getEnvInt("DATABASE_STATEMENT_TIMEOUT_MS", 60000),
				IdleInTxnTimeoutMS: getEnvInt("DATABASE_IDLE_IN_TXN_TIMEOUT_MS", 60000),
			},
			Vault: VaultConfig{
				Address:   getEnv("VAULT_ADDR", ""),
				Token:     getEnv("VAULT_TOKEN", ""),
				RoleID:    getEnv("VAULT_ROLE_ID", ""),
				SecretID:  getEnv("VAULT_SECRET_ID", ""),
				MountPath: getEnv("VAULT_MOUNT_PATH", "transit"),
			},
			Encryption:             loadEncryptionConfig(),
			MediaStreamTokenSecret: getEnv("MEDIA_STREAM_TOKEN_SECRET", ""),
			MediaStreamTokenTTL:    getEnvInt("MEDIA_STREAM_TOKEN_TTL", 900),
			UploadMaxConcurrent:    getEnvInt("UPLOAD_MAX_CONCURRENT", 4),
			// Gemma supplies only the structured summary contract. The legacy
			// renderer stores raw provider JSON and is not an OSS fallback.
			SummaryStructuredOutputEnabled: true,
			SummaryHighStakesEnabled:       getEnvBool("SUMMARY_HIGH_STAKES_ENABLED", false),
			SummaryProfilesEnabled:         getEnvBool("SUMMARY_PROFILES_ENABLED", false),
			SummarySharedAnalysisEnabled:   getEnvBool("SUMMARY_SHARED_ANALYSIS_ENABLED", false),
			CustomVocabularyEnabled:        getEnvBool("CUSTOM_VOCABULARY_ENABLED", false),
			BAPExportEnabled:               getEnvBool("BAP_EXPORT_ENABLED", false),
			PublicFrontendURL:              getEnv("PUBLIC_FRONTEND_URL", "http://localhost:5173"),
			Recording: RecordingConfig{
				MaxDurationHours:        getEnvPositiveInt("RECORDING_MAX_DURATION_HOURS", 8),
				MaxChunksPerSession:     getEnvPositiveInt("RECORDING_MAX_CHUNKS_PER_SESSION", 1200),
				OrphanThresholdMinutes:  getEnvPositiveInt("RECORDING_ORPHAN_THRESHOLD_MINUTES", 30),
				StitchFFmpegTimeoutMin:  getEnvPositiveInt("RECORDING_STITCH_FFMPEG_TIMEOUT_MINUTES", 45),
				StitchJobTimeoutMin:     getEnvPositiveInt("RECORDING_STITCH_JOB_TIMEOUT_MINUTES", 90),
				StitchServiceTimeoutMin: getEnvPositiveInt("RECORDING_STITCH_SERVICE_TIMEOUT_MINUTES", 50),
			},
			ClamAVAddress:        getEnv("CLAMAV_ADDRESS", ""),
			ScanEnforcement:      strings.ToLower(strings.TrimSpace(getEnv("SCAN_ENFORCEMENT", ScanEnforcementRequired))),
			MCPAllowedOrigins:    mcpOrigins,
			MCPMaxBodyBytes:      int64(getEnvPositiveInt("MCP_MAX_BODY_BYTES", 1<<20)),
			MCPResourceAudience:  strings.TrimSpace(getEnv("MCP_RESOURCE_AUDIENCE", "")),
			DeepFilterBinPath:    getEnv("DEEPFILTER_BIN_PATH", ""),
			DeepFilterEnabled:    getEnvBool("DEEPFILTER_ENABLED", false),
			RequireAudioBinaries: getEnvBool("REQUIRE_AUDIO_BINARIES", environment == "production"),
			PublicBaseURL:        strings.TrimRight(strings.TrimSpace(getEnv("PUBLIC_BASE_URL", "")), "/"),
		},
		TranscriptionProvider: "speechmatics",
	}

	if excludedErr := rejectOSSExcludedSettings(); excludedErr != nil {
		return nil, excludedErr
	}
	if providerErr := validateOSSSpeechmatics(); providerErr != nil {
		return nil, providerErr
	}
	if cfg.CustomVocabularyEnabled {
		return nil, fmt.Errorf("CUSTOM_VOCABULARY_ENABLED is not supported by the Speechmatics Melia model in Voxis Source-Available")
	}
	gemma, err := loadGemmaConfig()
	if err != nil {
		return nil, err
	}
	cfg.Gemma = gemma
	cfg.SummarySourceChunkBytes = ossSummarySourceChunkBytes(gemma.MaxContextTokens)

	if err := validateRecordingTimeouts(cfg.Recording); err != nil {
		return nil, err
	}
	media, mediaErr := loadMediaProcessing()
	if mediaErr != nil {
		return nil, mediaErr
	}
	cfg.Recording.MaxDuration = clampRecordingToMedia(cfg.Recording.MaxDurationHours, media)
	cfg.Media = media
	if err := validateEncryption(cfg.Encryption, cfg.Environment); err != nil {
		return nil, err
	}
	if err := validateProductionPersistence(cfg.Database, cfg.Environment); err != nil {
		return nil, err
	}
	if err := validateOSSAudience(&cfg.Config); err != nil {
		return nil, err
	}
	if err := validatePublicBaseURL(cfg.PublicBaseURL, cfg.Environment); err != nil {
		return nil, err
	}
	if err := validateScanEnforcement(cfg.ScanEnforcement, cfg.Environment); err != nil {
		return nil, err
	}
	if err := validateUserLookup(cfg.Auth, cfg.Environment); err != nil {
		return nil, err
	}
	return cfg, nil
}

func rejectOSSExcludedSettings() error {
	for _, key := range []string{
		"GLADIA_API_KEY", "GLADIA_WEBHOOK_SECRET", "GEMINI_PROVIDER", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "VERTEX_AI_PROJECT", "VERTEX_AI_LOCATION", "VERTEX_AI_MODEL",
		"MIDTRANS_SERVER_KEY", "MIDTRANS_CLIENT_KEY", "MIDTRANS_MERCHANT_ID",
		"GCS_PRIVILEGE_BUCKET",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return fmt.Errorf("%s is not supported by Voxis Source-Available", key)
		}
	}
	if getEnvBool("BILLING_ENABLED", false) {
		return fmt.Errorf("BILLING_ENABLED=true is not supported by Voxis Source-Available")
	}
	if strings.TrimSpace(os.Getenv("SUMMARY_STRUCTURED_OUTPUT_ENABLED")) != "" {
		return fmt.Errorf("SUMMARY_STRUCTURED_OUTPUT_ENABLED is always enabled in Voxis Source-Available and must not be configured")
	}
	return nil
}

func validateOSSSpeechmatics() error {
	provider := strings.ToLower(strings.TrimSpace(getEnv("TRANSCRIPTION_PROVIDER", "speechmatics")))
	if provider != "speechmatics" {
		return fmt.Errorf("TRANSCRIPTION_PROVIDER must be speechmatics in Voxis Source-Available")
	}
	if strings.ToLower(strings.TrimSpace(getEnv("SPEECHMATICS_MODE", "saas"))) != "saas" {
		return fmt.Errorf("SPEECHMATICS_MODE must be saas in Voxis Source-Available; on-premises mode is not yet supported")
	}
	apiKey := strings.TrimSpace(os.Getenv("SPEECHMATICS_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("SPEECHMATICS_API_KEY is required by Voxis Source-Available")
	}
	if strings.EqualFold(apiKey, envExamplePlaceholder) {
		return fmt.Errorf("SPEECHMATICS_API_KEY still holds the %s placeholder from .env.example; set your Speechmatics API key", envExamplePlaceholder)
	}

	apiURL := strings.TrimSpace(getEnv("SPEECHMATICS_API_URL", "https://eu1.asr.api.speechmatics.com"))
	u, err := url.Parse(apiURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("SPEECHMATICS_API_URL must be an absolute https URL in Voxis Source-Available")
	}
	return nil
}

func loadGemmaConfig() (GemmaConfig, error) {
	limits, err := loadGemmaLimits()
	if err != nil {
		return GemmaConfig{}, err
	}

	cfg := GemmaConfig{
		BaseURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("GEMMA_BASE_URL")), "/"),
		AllowHTTPInternal: getEnvBool("GEMMA_ALLOW_HTTP_INTERNAL", false),
		Model:             strings.TrimSpace(getEnv("GEMMA_MODEL", OSSGemmaModel)),
		Runtime:           strings.TrimSpace(os.Getenv("GEMMA_RUNTIME")),
		ModelRevision:     strings.TrimSpace(os.Getenv("GEMMA_MODEL_REVISION")),
		RuntimeRevision:   strings.TrimSpace(os.Getenv("GEMMA_RUNTIME_REVISION")),
		Quantization:      strings.TrimSpace(os.Getenv("GEMMA_QUANTIZATION")),
		MaxRequestBytes:   limits.requestBytes,
		MaxResponseBytes:  limits.responseBytes,
		MaxContextTokens:  limits.contextTokens,
		RequestTimeout:    time.Duration(limits.timeoutSeconds) * time.Second,
		SummaryJobTimeout: time.Duration(limits.summaryJobTimeoutMinutes) * time.Minute,
		MaxAttempts:       limits.attempts,
	}
	if cfg.Model != OSSGemmaModel && cfg.Model != OSSGemmaQATModel {
		return GemmaConfig{}, fmt.Errorf(
			"GEMMA_MODEL must be %q or %q in Voxis Source-Available", OSSGemmaModel, OSSGemmaQATModel)
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{"GEMMA_BASE_URL", cfg.BaseURL},
		{"GEMMA_RUNTIME", cfg.Runtime},
		{"GEMMA_MODEL_REVISION", cfg.ModelRevision},
		{"GEMMA_RUNTIME_REVISION", cfg.RuntimeRevision},
		{"GEMMA_QUANTIZATION", cfg.Quantization},
	} {
		if required.value == "" {
			return GemmaConfig{}, fmt.Errorf("%s is required by Voxis Source-Available", required.name)
		}
	}
	if endpointErr := validateGemmaBaseURL(cfg.BaseURL, cfg.AllowHTTPInternal); endpointErr != nil {
		return GemmaConfig{}, endpointErr
	}
	if cfg.MaxRequestBytes > 32<<20 || cfg.MaxResponseBytes > 32<<20 || cfg.MaxContextTokens > 131072 || cfg.MaxAttempts > 5 {
		return GemmaConfig{}, fmt.Errorf("gemma limits exceed Voxis Source-Available safety bounds")
	}
	if cfg.MaxContextTokens < minOSSGemmaContextTokens {
		return GemmaConfig{}, fmt.Errorf("GEMMA_MAX_CONTEXT_TOKENS must be at least %d in Voxis Source-Available", minOSSGemmaContextTokens)
	}
	return cfg, nil
}

type gemmaLimits struct {
	requestBytes             int64
	responseBytes            int64
	contextTokens            int
	timeoutSeconds           int
	summaryJobTimeoutMinutes int
	attempts                 int
}

func loadGemmaLimits() (gemmaLimits, error) {
	requestBytes, err := positiveInt64Env("GEMMA_MAX_REQUEST_BYTES", defaultGemmaMaxRequestBytes)
	if err != nil {
		return gemmaLimits{}, err
	}
	responseBytes, err := positiveInt64Env("GEMMA_MAX_RESPONSE_BYTES", defaultGemmaMaxResponseBytes)
	if err != nil {
		return gemmaLimits{}, err
	}
	contextTokens, err := positiveIntEnv("GEMMA_MAX_CONTEXT_TOKENS", defaultGemmaMaxContextTokens)
	if err != nil {
		return gemmaLimits{}, err
	}
	timeoutSeconds, err := positiveIntEnv("GEMMA_REQUEST_TIMEOUT_SECONDS", int(defaultGemmaTimeout.Seconds()))
	if err != nil {
		return gemmaLimits{}, err
	}
	summaryJobTimeoutMinutes, err := positiveIntEnv("GEMMA_SUMMARY_JOB_TIMEOUT_MINUTES", int(defaultGemmaSummaryJobTimeout.Minutes()))
	if err != nil {
		return gemmaLimits{}, err
	}
	attempts, err := positiveIntEnv("GEMMA_MAX_ATTEMPTS", defaultGemmaMaxAttempts)
	if err != nil {
		return gemmaLimits{}, err
	}
	if timeoutSeconds > maxGemmaTimeoutSeconds {
		return gemmaLimits{}, fmt.Errorf("GEMMA_REQUEST_TIMEOUT_SECONDS must be no greater than %d", maxGemmaTimeoutSeconds)
	}
	if summaryJobTimeoutMinutes > maxGemmaSummaryJobTimeoutMin {
		return gemmaLimits{}, fmt.Errorf("GEMMA_SUMMARY_JOB_TIMEOUT_MINUTES must be no greater than %d", maxGemmaSummaryJobTimeoutMin)
	}
	if timeoutSeconds >= summaryJobTimeoutMinutes*60 {
		return gemmaLimits{}, fmt.Errorf("GEMMA_SUMMARY_JOB_TIMEOUT_MINUTES must exceed GEMMA_REQUEST_TIMEOUT_SECONDS")
	}
	return gemmaLimits{
		requestBytes:             requestBytes,
		responseBytes:            responseBytes,
		contextTokens:            contextTokens,
		timeoutSeconds:           timeoutSeconds,
		summaryJobTimeoutMinutes: summaryJobTimeoutMinutes,
		attempts:                 attempts,
	}, nil
}

func ossSummarySourceChunkBytes(contextTokens int) int {
	chunkBytes := contextTokens - ossSummaryContextReserve
	if chunkBytes > maxOSSSummaryChunkBytes {
		return maxOSSSummaryChunkBytes
	}
	return chunkBytes
}

func validateGemmaBaseURL(raw string, allowHTTPInternal bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("GEMMA_BASE_URL must be an absolute http(s) URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("GEMMA_BASE_URL must not include credentials, query, or fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	if allowHTTPInternal {
		return nil
	}
	return fmt.Errorf("GEMMA_BASE_URL must use https unless GEMMA_ALLOW_HTTP_INTERNAL=true for a bundled private endpoint")
}

func positiveIntEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return v, nil
}

func positiveInt64Env(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return v, nil
}

func validateOSSAudience(cfg *Config) error {
	if cfg.Environment == "production" &&
		len(cfg.Auth.AdditionalAudiences) > 0 &&
		cfg.MCPResourceAudience == "" {
		return fmt.Errorf("MCP_RESOURCE_AUDIENCE is required in production when KEYCLOAK_ADDITIONAL_AUDIENCES is set")
	}
	if cfg.Environment == "production" && overlaps(cfg.Auth.AdminAllowedClients, cfg.Auth.AdditionalAudiences) {
		return fmt.Errorf("KEYCLOAK_ADMIN_ALLOWED_CLIENTS must not overlap KEYCLOAK_ADDITIONAL_AUDIENCES")
	}
	if cfg.MCPResourceAudience != "" && cfg.PublicBaseURL != "" && cfg.MCPResourceAudience != cfg.PublicBaseURL+"/mcp" {
		return fmt.Errorf("MCP_RESOURCE_AUDIENCE must equal PUBLIC_BASE_URL + \"/mcp\"")
	}
	return nil
}
