package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config is the complete configuration accepted by the OSS server. It has no
// fields for commercial providers, URL ingestion, privileged recordings, or
// payments. The export replaces the commercial config package with this one.
type Config struct {
	Port        string
	BindAddress string
	Environment string
	Debug       bool
	Auth        AuthConfig

	CORSAllowedOrigins []string
	TrustedProxies     []string
	Database           DatabaseConfig
	Vault              VaultConfig
	Encryption         EncryptionConfig

	MediaStreamTokenSecret string
	MediaStreamTokenTTL    int
	UploadMaxConcurrent    int
	PublicBaseURL          string
	PublicFrontendURL      string

	SummaryHighStakesEnabled       bool
	SummaryStructuredOutputEnabled bool
	SummaryProfilesEnabled         bool
	SummarySharedAnalysisEnabled   bool
	CustomVocabularyEnabled        bool
	BAPExportEnabled               bool

	Recording            RecordingConfig
	ClamAVAddress        string
	ScanEnforcement      string
	MCPAllowedOrigins    []string
	MCPMaxBodyBytes      int64
	MCPResourceAudience  string
	DeepFilterBinPath    string
	DeepFilterEnabled    bool
	RequireAudioBinaries bool
}

// DatabaseConfig controls the retained PostgreSQL connection pool.
type DatabaseConfig struct {
	URL                string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetime    int
	StatementTimeoutMS int
	IdleInTxnTimeoutMS int
}

// VaultConfig identifies the persistent transit service used for org keys.
type VaultConfig struct {
	Address   string
	Token     string
	RoleID    string
	SecretID  string
	MountPath string
}

// Encryption provider names accepted by the OSS configuration loader.
const (
	EncryptionProviderVault  = "vault"
	EncryptionProviderMemory = "memory"
	defaultKEKCacheTTL       = 5 * time.Minute
	maxKEKCacheTTL           = time.Hour
)

// EncryptionConfig controls the required persistent transit key provider.
type EncryptionConfig struct {
	Provider    string
	KEKCacheTTL time.Duration
}

func loadEncryptionConfig() EncryptionConfig {
	return EncryptionConfig{
		Provider:    strings.ToLower(strings.TrimSpace(getEnv("ENCRYPTION_PROVIDER", EncryptionProviderVault))),
		KEKCacheTTL: getEnvDuration("KEK_CACHE_TTL", defaultKEKCacheTTL),
	}
}

// RecordingConfig bounds retained standard recording sessions and workers.
type RecordingConfig struct {
	MaxDurationHours int
	// MaxDuration is the effective recording limit: MaxDurationHours,
	// lowered to MEDIA_MAX_DURATION when that is shorter. Set by LoadOSS.
	MaxDuration             time.Duration
	MaxChunksPerSession     int
	OrphanThresholdMinutes  int
	StitchFFmpegTimeoutMin  int
	StitchJobTimeoutMin     int
	StitchServiceTimeoutMin int
}

const (
	recordingStitchScanTimeoutMin      = 10
	recordingStitchUnbudgetedAllowance = 30
)

// AuthConfig contains the Keycloak issuer and API audience configuration.
type AuthConfig struct {
	KeycloakURL         string
	KeycloakFetchURL    string
	Realm               string
	ClientID            string
	AdditionalAudiences []string
	AdminAllowedClients []string
	// UserLookupClientID and UserLookupClientSecret identify the confidential
	// service-account client that checks API key owners in Keycloak.
	UserLookupClientID     string
	UserLookupClientSecret string
}

// IssuerURL returns the public canonical issuer for JWT verification.
func (a AuthConfig) IssuerURL() string {
	if a.KeycloakURL == "" || a.Realm == "" {
		return ""
	}
	return a.KeycloakURL + "/realms/" + a.Realm
}

// FetchURL returns the discovery endpoint origin, which may be privately routed.
func (a AuthConfig) FetchURL() string {
	baseURL := a.FetchBaseURL()
	if baseURL == "" || a.Realm == "" {
		return ""
	}
	return baseURL + "/realms/" + a.Realm
}

// FetchBaseURL returns the privately routed Keycloak base URL (without
// "/realms/<realm>"), falling back to the public URL.
func (a AuthConfig) FetchBaseURL() string {
	if a.KeycloakFetchURL != "" {
		return a.KeycloakFetchURL
	}
	return a.KeycloakURL
}

func validateUserLookup(auth AuthConfig, environment string) error {
	hasID, hasSecret := auth.UserLookupClientID != "", auth.UserLookupClientSecret != ""
	if hasID != hasSecret {
		return fmt.Errorf("KEYCLOAK_USER_LOOKUP_CLIENT_ID and KEYCLOAK_USER_LOOKUP_CLIENT_SECRET must be set together")
	}
	if environment == "production" && !hasID {
		return fmt.Errorf("KEYCLOAK_USER_LOOKUP_CLIENT_ID and KEYCLOAK_USER_LOOKUP_CLIENT_SECRET are required in production so API keys stop working when their owner is disabled")
	}
	return nil
}

// Malware scan enforcement modes accepted by SCAN_ENFORCEMENT.
const (
	// ScanEnforcementRequired refuses to start without ClamAV.
	ScanEnforcementRequired = "required"
	// ScanEnforcementOptional scans when CLAMAV_ADDRESS is set and otherwise
	// marks uploads scan_skipped. Development only.
	ScanEnforcementOptional = "optional"
)

func validateScanEnforcement(mode, environment string) error {
	switch mode {
	case ScanEnforcementRequired:
		return nil
	case ScanEnforcementOptional:
		if environment == "production" {
			return fmt.Errorf("SCAN_ENFORCEMENT must be %q in production", ScanEnforcementRequired)
		}
		return nil
	default:
		return fmt.Errorf("SCAN_ENFORCEMENT must be %q or %q", ScanEnforcementRequired, ScanEnforcementOptional)
	}
}

func validateEncryption(enc EncryptionConfig, _ string) error {
	if enc.Provider != EncryptionProviderVault {
		return fmt.Errorf("ENCRYPTION_PROVIDER must be %q in Voxis Source-Available", EncryptionProviderVault)
	}
	if enc.KEKCacheTTL <= 0 || enc.KEKCacheTTL > maxKEKCacheTTL {
		return fmt.Errorf("KEK_CACHE_TTL must be a positive duration no greater than %s", maxKEKCacheTTL)
	}
	return nil
}

func validateProductionPersistence(db DatabaseConfig, environment string) error {
	if environment == "production" && strings.TrimSpace(db.URL) == "" {
		return fmt.Errorf("DATABASE_URL is required in production")
	}
	return nil
}

func validateRecordingTimeouts(recording RecordingConfig) error {
	pipelineTimeout := max(recording.StitchServiceTimeoutMin, recording.StitchFFmpegTimeoutMin)
	minimumJobTimeout := pipelineTimeout + recordingStitchScanTimeoutMin + recordingStitchUnbudgetedAllowance
	if recording.StitchJobTimeoutMin < minimumJobTimeout {
		return fmt.Errorf("RECORDING_STITCH_JOB_TIMEOUT_MINUTES (%d) must be at least %d to cover scan, stitch, and I/O finalization", recording.StitchJobTimeoutMin, minimumJobTimeout)
	}
	return nil
}

func validatePublicBaseURL(rawURL, environment string) error {
	if environment != "production" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("PUBLIC_BASE_URL must be an HTTPS origin in production")
	}
	return nil
}

func loadNetworkAllowlists(environment string) (corsOrigins, trustedProxies, mcpOrigins []string, err error) {
	corsOrigins, err = parseCORSOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid CORS_ALLOWED_ORIGINS: %w", err)
	}
	trustedProxies, err = parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}
	if environment == "production" && len(trustedProxies) == 0 {
		return nil, nil, nil, fmt.Errorf("TRUSTED_PROXIES is required in production")
	}
	if environment == "production" && trustsEveryAddress(trustedProxies) {
		return nil, nil, nil, fmt.Errorf("TRUSTED_PROXIES must not trust every address (0.0.0.0/0 or ::/0) in production: any client could then spoof its IP for rate limits")
	}
	mcpOrigins, err = parseMCPOrigins(getEnv("MCP_ALLOWED_ORIGINS", ""))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid MCP_ALLOWED_ORIGINS: %w", err)
	}
	if environment == "production" && len(mcpOrigins) == 0 {
		return nil, nil, nil, fmt.Errorf("MCP_ALLOWED_ORIGINS is required in production")
	}
	return corsOrigins, trustedProxies, mcpOrigins, nil
}

func parseCORSOrigins(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if origin == "*" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("invalid origin %q", origin)
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

func parseTrustedProxies(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	proxies := make([]string, 0, len(parts))
	for _, part := range parts {
		proxy := strings.TrimSpace(part)
		if proxy == "" {
			continue
		}
		if strings.Contains(proxy, "/") {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				return nil, fmt.Errorf("invalid CIDR %q", proxy)
			}
		} else if net.ParseIP(proxy) == nil {
			return nil, fmt.Errorf("invalid IP %q", proxy)
		}
		proxies = append(proxies, proxy)
	}
	return proxies, nil
}

// trustsEveryAddress reports whether any entry is a zero-length prefix such as
// 0.0.0.0/0 or ::/0. Entries were already validated by parseTrustedProxies.
func trustsEveryAddress(proxies []string) bool {
	for _, proxy := range proxies {
		_, network, err := net.ParseCIDR(proxy)
		if err != nil {
			continue
		}
		if ones, _ := network.Mask.Size(); ones == 0 {
			return true
		}
	}
	return false
}

func parseMCPOrigins(raw string) ([]string, error) { return parseCORSOrigins(raw) }

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func overlaps(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value, err := positiveIntEnv(key, fallback)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvPositiveInt(key string, fallback int) int { return getEnvInt(key, fallback) }

func getEnvBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return value
}
