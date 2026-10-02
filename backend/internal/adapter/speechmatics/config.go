// Package speechmatics implements port.TranscriptionProvider against the
// Speechmatics batch API (v2) using the Melia 1 model.
//
// The port's Upload -> audioURL + Submit(req) split does not match
// Speechmatics' single multipart POST, so the adapter bridges it internally:
// Upload spools the plaintext audio to a temp file and returns an opaque
// staging token, and Submit performs the one real HTTP call. The token is
// threaded through the worker only — it is never persisted anywhere.
package speechmatics

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/voxis/backend/internal/domain"
)

// Mode selects the deployment the adapter talks to. Container mode changes the
// base URL and drops the bearer credential (an on-prem container is reached
// over a private network); the request/response shapes are the same, so there
// is no separate implementation.
//
// NOT REACHABLE YET: cmd/server/main.go only constructs this client when
// SPEECHMATICS_API_KEY is set, and container mode is exactly the deployment
// that has no account key. Setting SPEECHMATICS_MODE=container today leaves the
// provider unwired and is silently ignored. The code path below is finished and
// tested; landing on-prem means changing that one wiring condition, not this
// adapter.
const (
	ModeSaaS      = "saas"
	ModeContainer = "container"
)

// DefaultAPIURL is the Speechmatics EU1 batch endpoint.
//
// NOT eu2: on 2026-08-30 live testing, eu2.asr.api.speechmatics.com answered a
// valid production key with an nginx HTTP 401 on both GET and POST, while eu1
// and the global endpoint served the same key normally. Melia is EU/US only, so
// eu1 is both reachable and in-region. A deployment that needs another region
// overrides SPEECHMATICS_API_URL.
const DefaultAPIURL = "https://eu1.asr.api.speechmatics.com"

// Config holds the adapter's environment-derived settings.
type Config struct {
	// APIURL is the base URL, without a trailing slash.
	APIURL string
	// APIKey authenticates SaaS requests. Required in saas mode.
	APIKey string
	// Mode is ModeSaaS or ModeContainer.
	Mode string
	// LanguageHints are deployment-wide Melia language hints applied when a
	// transcription requests no explicit languages. Already mapped to
	// provider codes.
	LanguageHints []string
	// SpeakerSensitivity tunes Speechmatics' speaker_diarization_config
	// (0..1, lower = fewer distinct speakers). Nil means unset: the field is
	// omitted from the job config entirely and the provider default (0.5)
	// applies.
	SpeakerSensitivity *float64
	// PreferCurrentSpeaker tunes Speechmatics' speaker_diarization_config:
	// true sticks with the current speaker when a match is close. Nil means
	// unset: the field is omitted and the provider default (false) applies.
	PreferCurrentSpeaker *bool
	// WebhookEnabled opts new submissions into Speechmatics' completion
	// notification. Off by default: the callback needs a PUBLIC_BASE_URL that
	// Speechmatics' egress can actually reach, and the periodic poller plus
	// the post-submit wait job complete every job without it.
	WebhookEnabled bool
}

// LoadConfig reads the adapter configuration from the environment:
//
//	SPEECHMATICS_API_URL                    base URL (default DefaultAPIURL)
//	SPEECHMATICS_API_KEY                    required in saas mode
//	SPEECHMATICS_MODE                       saas (default) | container
//	SPEECHMATICS_LANGUAGE_HINTS             optional comma-separated Voxis language codes
//	SPEECHMATICS_SPEAKER_SENSITIVITY        optional float in [0,1]; unset = provider default (0.5)
//	SPEECHMATICS_PREFER_CURRENT_SPEAKER     optional bool; unset = provider default (false)
//	SPEECHMATICS_WEBHOOK_ENABLED            optional bool; default false (poller-only completion)
//
// Every failure is domain.ErrInvalidInput so startup wiring can report it
// uniformly. Language hints and diarization tuning are validated here rather
// than at submit time so a typo fails the process instead of every job.
func LoadConfig() (Config, error) {
	cfg := Config{
		APIURL: strings.TrimRight(getEnvOrDefault("SPEECHMATICS_API_URL", DefaultAPIURL), "/"),
		APIKey: strings.TrimSpace(os.Getenv("SPEECHMATICS_API_KEY")),
		Mode:   strings.ToLower(strings.TrimSpace(getEnvOrDefault("SPEECHMATICS_MODE", ModeSaaS))),
	}

	if cfg.Mode != ModeSaaS && cfg.Mode != ModeContainer {
		return Config{}, fmt.Errorf("speechmatics: SPEECHMATICS_MODE must be %q or %q, got %q: %w",
			ModeSaaS, ModeContainer, cfg.Mode, domain.ErrInvalidInput)
	}

	if err := validateBaseURL(cfg.APIURL, cfg.Mode); err != nil {
		return Config{}, err
	}

	if cfg.Mode == ModeSaaS && cfg.APIKey == "" {
		return Config{}, fmt.Errorf("speechmatics: SPEECHMATICS_API_KEY is required in saas mode: %w", domain.ErrInvalidInput)
	}

	hints, err := MapLanguageHints(splitCSV(os.Getenv("SPEECHMATICS_LANGUAGE_HINTS")))
	if err != nil {
		return Config{}, fmt.Errorf("speechmatics: SPEECHMATICS_LANGUAGE_HINTS: %w", err)
	}
	cfg.LanguageHints = hints

	sensitivity, err := parseOptionalSensitivity(os.Getenv("SPEECHMATICS_SPEAKER_SENSITIVITY"))
	if err != nil {
		return Config{}, err
	}
	cfg.SpeakerSensitivity = sensitivity

	preferCurrent, err := parseOptionalBool(os.Getenv("SPEECHMATICS_PREFER_CURRENT_SPEAKER"))
	if err != nil {
		return Config{}, err
	}
	cfg.PreferCurrentSpeaker = preferCurrent

	webhookEnabled, err := parseOptionalBool(os.Getenv("SPEECHMATICS_WEBHOOK_ENABLED"))
	if err != nil {
		return Config{}, fmt.Errorf("speechmatics: SPEECHMATICS_WEBHOOK_ENABLED is not a bool: %w", domain.ErrInvalidInput)
	}
	cfg.WebhookEnabled = webhookEnabled != nil && *webhookEnabled

	return cfg, nil
}

// parseOptionalSensitivity parses SPEECHMATICS_SPEAKER_SENSITIVITY. An
// empty/whitespace-only value means unset (nil); anything else must parse as
// a float64 in [0,1].
func parseOptionalSensitivity(raw string) (*float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil, fmt.Errorf("speechmatics: SPEECHMATICS_SPEAKER_SENSITIVITY %q is not a number: %w", raw, domain.ErrInvalidInput)
	}
	if v < 0 || v > 1 {
		return nil, fmt.Errorf("speechmatics: SPEECHMATICS_SPEAKER_SENSITIVITY %q must be in [0,1]: %w", raw, domain.ErrInvalidInput)
	}
	return &v, nil
}

// parseOptionalBool parses SPEECHMATICS_PREFER_CURRENT_SPEAKER. An
// empty/whitespace-only value means unset (nil); anything else must parse via
// strconv.ParseBool.
func parseOptionalBool(raw string) (*bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	v, err := strconv.ParseBool(trimmed)
	if err != nil {
		return nil, fmt.Errorf("speechmatics: SPEECHMATICS_PREFER_CURRENT_SPEAKER %q is not a bool: %w", raw, domain.ErrInvalidInput)
	}
	return &v, nil
}

// validateBaseURL rejects a malformed or plaintext SaaS endpoint. A container
// endpoint may be plain http because it is reached over a private network.
func validateBaseURL(raw, mode string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("speechmatics: SPEECHMATICS_API_URL %q is not a valid URL: %w", raw, domain.ErrInvalidInput)
	}
	if mode == ModeSaaS && u.Scheme != "https" {
		return fmt.Errorf("speechmatics: SPEECHMATICS_API_URL must be https in saas mode: %w", domain.ErrInvalidInput)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("speechmatics: SPEECHMATICS_API_URL scheme %q is not supported: %w", u.Scheme, domain.ErrInvalidInput)
	}
	return nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// splitCSV splits a comma-separated list, dropping empty entries.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
