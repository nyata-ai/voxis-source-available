package config

import (
	"strings"
	"testing"
)

func TestLoadOSSRequiresSpeechmaticsOnly(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("TRANSCRIPTION_PROVIDER", "gladia")

	_, err := LoadOSS()
	if err == nil || err.Error() != "TRANSCRIPTION_PROVIDER must be speechmatics in Voxis Source-Available" {
		t.Fatalf("LoadOSS() error = %v, want Speechmatics-only error", err)
	}
}

func TestLoadOSSRejectsCustomVocabularyForMelia(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("CUSTOM_VOCABULARY_ENABLED", "true")

	_, err := LoadOSS()
	if err == nil || err.Error() != "CUSTOM_VOCABULARY_ENABLED is not supported by the Speechmatics Melia model in Voxis Source-Available" {
		t.Fatalf("LoadOSS() error = %v, want Melia vocabulary capability error", err)
	}
}

func TestLoadOSSRejectsExcludedProviderSettings(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("GLADIA_API_KEY", "not-a-secret")

	_, err := LoadOSS()
	if err == nil || err.Error() != "GLADIA_API_KEY is not supported by Voxis Source-Available" {
		t.Fatalf("LoadOSS() error = %v, want excluded provider error", err)
	}
}

func TestLoadOSSAcceptsOnlyPinnedGemmaModels(t *testing.T) {
	for _, model := range []string{OSSGemmaModel, OSSGemmaQATModel} {
		t.Run(model, func(t *testing.T) {
			setValidOSSEnv(t)
			t.Setenv("GEMMA_MODEL", model)

			cfg, err := LoadOSS()
			if err != nil {
				t.Fatalf("LoadOSS() error = %v", err)
			}
			if cfg.Gemma.Model != model {
				t.Fatalf("Gemma.Model = %q, want %q", cfg.Gemma.Model, model)
			}
		})
	}
}

func TestLoadOSSRejectsUnpinnedGemmaModel(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("GEMMA_MODEL", "another-model")

	_, err := LoadOSS()
	if err == nil {
		t.Fatal("LoadOSS() error = nil, want pinned-model error")
	}
}

func TestLoadOSSEnablesStructuredHighStakesWhenConfigured(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("SUMMARY_HIGH_STAKES_ENABLED", "true")

	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
	if !cfg.SummaryHighStakesEnabled {
		t.Fatal("SummaryHighStakesEnabled = false, want true")
	}
}

func TestLoadOSSDerivesSourceChunksFromGemmaContext(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("GEMMA_MAX_CONTEXT_TOKENS", "32768")

	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
	if cfg.SummarySourceChunkBytes != 16<<10 {
		t.Fatalf("SummarySourceChunkBytes = %d, want 16384", cfg.SummarySourceChunkBytes)
	}
}

func TestLoadOSSUsesBoundedGemmaTimeouts(t *testing.T) {
	setValidOSSEnv(t)

	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
	if cfg.Gemma.RequestTimeout.Seconds() != 360 {
		t.Fatalf("Gemma.RequestTimeout = %s, want 6m", cfg.Gemma.RequestTimeout)
	}
	if cfg.Gemma.SummaryJobTimeout.Minutes() != 60 {
		t.Fatalf("Gemma.SummaryJobTimeout = %s, want 60m", cfg.Gemma.SummaryJobTimeout)
	}
}

func TestLoadOSSRejectsUnsafeGemmaTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "request cap", key: "GEMMA_REQUEST_TIMEOUT_SECONDS", value: "601"},
		{name: "job cap", key: "GEMMA_SUMMARY_JOB_TIMEOUT_MINUTES", value: "121"},
		{name: "job does not exceed request", key: "GEMMA_SUMMARY_JOB_TIMEOUT_MINUTES", value: "6"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidOSSEnv(t)
			t.Setenv(test.key, test.value)

			if _, err := LoadOSS(); err == nil {
				t.Fatal("LoadOSS() error = nil, want timeout validation error")
			}
		})
	}
}

func TestLoadOSSRetainsOptInBAPExport(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("BAP_EXPORT_ENABLED", "true")

	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
	if !cfg.BAPExportEnabled {
		t.Fatal("BAPExportEnabled = false, want true")
	}
}

func TestValidateGemmaBaseURLRequiresExplicitInternalHTTPOptIn(t *testing.T) {
	t.Setenv("GEMMA_ALLOW_HTTP_INTERNAL", "false")
	if err := validateGemmaBaseURL("http://gemma:8080/v1", false); err == nil {
		t.Fatal("validateGemmaBaseURL() error = nil, want HTTPS requirement")
	}
	t.Setenv("GEMMA_ALLOW_HTTP_INTERNAL", "true")
	if err := validateGemmaBaseURL("http://gemma:8080/v1", true); err != nil {
		t.Fatalf("validateGemmaBaseURL(internal) error = %v", err)
	}
	if err := validateGemmaBaseURL("https://gemma.example/v1?token=secret", false); err == nil {
		t.Fatal("validateGemmaBaseURL() error = nil, want unsafe URL rejection")
	}
}

func TestLoadOSSRejectsEnvExamplePlaceholderSpeechmaticsKey(t *testing.T) {
	for _, value := range []string{"CHANGE_ME", " change_me "} {
		setValidOSSEnv(t)
		t.Setenv("SPEECHMATICS_API_KEY", value)

		_, err := LoadOSS()
		if err == nil || !strings.Contains(err.Error(), "SPEECHMATICS_API_KEY still holds the CHANGE_ME placeholder") {
			t.Fatalf("LoadOSS(%q) error = %v, want placeholder rejection", value, err)
		}
	}
}

func TestLoadOSSAcceptsValidProductionEnv(t *testing.T) {
	setValidProductionOSSEnv(t)

	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
	if cfg.ScanEnforcement != ScanEnforcementRequired {
		t.Fatalf("ScanEnforcement = %q, want required", cfg.ScanEnforcement)
	}
	if cfg.Auth.UserLookupClientID != "voxis-oss-api-lookup" || cfg.Auth.UserLookupClientSecret != "lookup-secret" {
		t.Fatalf("user lookup client = %q/%q", cfg.Auth.UserLookupClientID, cfg.Auth.UserLookupClientSecret)
	}
	if cfg.Auth.FetchBaseURL() != "http://keycloak:8080/auth" {
		t.Fatalf("FetchBaseURL() = %q", cfg.Auth.FetchBaseURL())
	}
}

func TestLoadOSSRejectsWildcardTrustedProxiesInProduction(t *testing.T) {
	for _, value := range []string{"0.0.0.0/0", "172.30.0.0/24, ::/0", "::0/0"} {
		setValidProductionOSSEnv(t)
		t.Setenv("TRUSTED_PROXIES", value)

		_, err := LoadOSS()
		if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES must not trust every address") {
			t.Fatalf("LoadOSS(TRUSTED_PROXIES=%q) error = %v, want wildcard rejection", value, err)
		}
	}
}

func TestLoadOSSAllowsWildcardTrustedProxiesInDevelopment(t *testing.T) {
	setValidOSSEnv(t)
	t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")

	if _, err := LoadOSS(); err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
}

func TestLoadOSSScanEnforcementIsAStrictEnum(t *testing.T) {
	tests := []struct {
		env, value, wantErr string
	}{
		{"development", "optional", ""},
		{"development", " Required ", ""},
		{"development", "off", "SCAN_ENFORCEMENT must be"},
		{"development", "disabled", "SCAN_ENFORCEMENT must be"},
		{"production", "optional", `SCAN_ENFORCEMENT must be "required" in production`},
		{"production", "skip", "SCAN_ENFORCEMENT must be"},
		{"production", "required", ""},
	}
	for _, tt := range tests {
		t.Run(tt.env+"/"+tt.value, func(t *testing.T) {
			if tt.env == "production" {
				setValidProductionOSSEnv(t)
			} else {
				setValidOSSEnv(t)
			}
			t.Setenv("SCAN_ENFORCEMENT", tt.value)

			_, err := LoadOSS()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("LoadOSS() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("LoadOSS() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadOSSUserLookupClient(t *testing.T) {
	t.Run("required in production", func(t *testing.T) {
		setValidProductionOSSEnv(t)
		t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_ID", "")
		t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_SECRET", "")

		_, err := LoadOSS()
		if err == nil || !strings.Contains(err.Error(), "required in production") {
			t.Fatalf("LoadOSS() error = %v, want lookup client requirement", err)
		}
	})
	t.Run("optional in development", func(t *testing.T) {
		setValidOSSEnv(t)

		cfg, err := LoadOSS()
		if err != nil {
			t.Fatalf("LoadOSS() error = %v", err)
		}
		if cfg.Auth.UserLookupClientID != "" {
			t.Fatalf("UserLookupClientID = %q, want empty", cfg.Auth.UserLookupClientID)
		}
	})
	t.Run("id and secret travel together", func(t *testing.T) {
		setValidOSSEnv(t)
		t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_ID", "voxis-oss-api-lookup")

		_, err := LoadOSS()
		if err == nil || !strings.Contains(err.Error(), "must be set together") {
			t.Fatalf("LoadOSS() error = %v, want pairing error", err)
		}
	})
}

func setValidProductionOSSEnv(t *testing.T) {
	t.Helper()
	setValidOSSEnv(t)
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("DATABASE_URL", "postgres://voxis@postgres:5432/voxis")
	t.Setenv("TRUSTED_PROXIES", "172.30.0.0/24")
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://voxis.example.invalid")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://voxis.example.invalid")
	t.Setenv("PUBLIC_BASE_URL", "https://voxis.example.invalid")
	t.Setenv("OSS_KEYCLOAK_PUBLIC_URL", "https://voxis.example.invalid/auth")
	t.Setenv("OSS_KEYCLOAK_FETCH_URL", "http://keycloak:8080/auth")
	t.Setenv("OSS_KEYCLOAK_REALM", "voxis-oss")
	t.Setenv("OSS_KEYCLOAK_ADDITIONAL_AUDIENCES", "")
	t.Setenv("MCP_RESOURCE_AUDIENCE", "")
	t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_ID", "voxis-oss-api-lookup")
	t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_SECRET", "lookup-secret")
}

func setValidOSSEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GLADIA_API_KEY", "GLADIA_WEBHOOK_SECRET", "GEMINI_PROVIDER", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "VERTEX_AI_PROJECT", "VERTEX_AI_LOCATION", "VERTEX_AI_MODEL",
		"MIDTRANS_SERVER_KEY", "MIDTRANS_CLIENT_KEY", "MIDTRANS_MERCHANT_ID",
		"GCS_PRIVILEGE_BUCKET",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("SCAN_ENFORCEMENT", "")
	t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_ID", "")
	t.Setenv("KEYCLOAK_USER_LOOKUP_CLIENT_SECRET", "")
	t.Setenv("KEK_CACHE_TTL", "")
	t.Setenv("BILLING_ENABLED", "false")
	t.Setenv("BAP_EXPORT_ENABLED", "false")
	t.Setenv("SUMMARY_STRUCTURED_OUTPUT_ENABLED", "")
	t.Setenv("SUMMARY_HIGH_STAKES_ENABLED", "false")
	t.Setenv("TRANSCRIPTION_PROVIDER", "speechmatics")
	t.Setenv("SPEECHMATICS_MODE", "saas")
	t.Setenv("SPEECHMATICS_API_KEY", "not-a-secret")
	t.Setenv("SPEECHMATICS_API_URL", "https://eu1.asr.api.speechmatics.com")
	t.Setenv("GEMMA_BASE_URL", "http://127.0.0.1:8080/v1")
	t.Setenv("GEMMA_ALLOW_HTTP_INTERNAL", "true")
	t.Setenv("GEMMA_RUNTIME", "test-runtime")
	t.Setenv("GEMMA_MODEL_REVISION", "test-revision")
	t.Setenv("GEMMA_RUNTIME_REVISION", "test-runtime")
	t.Setenv("GEMMA_QUANTIZATION", "q4")
	for _, key := range []string{
		"GEMMA_MAX_REQUEST_BYTES", "GEMMA_MAX_RESPONSE_BYTES", "GEMMA_MAX_CONTEXT_TOKENS",
		"GEMMA_REQUEST_TIMEOUT_SECONDS", "GEMMA_SUMMARY_JOB_TIMEOUT_MINUTES", "GEMMA_MAX_ATTEMPTS",
	} {
		t.Setenv(key, "")
	}
}
