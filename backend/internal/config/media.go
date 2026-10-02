package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	// defaultMediaMaxDuration mirrors domain.DefaultMaxMediaDuration.
	defaultMediaMaxDuration = 8 * time.Hour
	minMediaMaxDuration     = time.Minute
	maxMediaMaxDuration     = 24 * time.Hour
)

// MediaProcessingConfig controls how the API parses untrusted media.
type MediaProcessingConfig struct {
	// SandboxRequired makes ffprobe, ffmpeg and DeepFilterNet refuse to run
	// without bubblewrap. Only MEDIA_SANDBOX=disabled turns it off.
	SandboxRequired bool
	// MaxDuration is the longest audio accepted (MEDIA_MAX_DURATION).
	MaxDuration time.Duration
}

// loadMediaProcessing reads MEDIA_SANDBOX and MEDIA_MAX_DURATION. The sandbox
// is required in every environment unless explicitly disabled.
func loadMediaProcessing() (MediaProcessingConfig, error) {
	cfg := MediaProcessingConfig{SandboxRequired: true, MaxDuration: defaultMediaMaxDuration}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MEDIA_SANDBOX"))) {
	case "", "required":
	case "disabled":
		cfg.SandboxRequired = false
	default:
		return MediaProcessingConfig{}, fmt.Errorf("MEDIA_SANDBOX must be required or disabled")
	}
	raw := strings.TrimSpace(os.Getenv("MEDIA_MAX_DURATION"))
	if raw == "" {
		return cfg, nil
	}
	limit, err := time.ParseDuration(raw)
	if err != nil || limit < minMediaMaxDuration || limit > maxMediaMaxDuration {
		return MediaProcessingConfig{}, fmt.Errorf("MEDIA_MAX_DURATION must be a duration between %s and %s, such as 8h",
			minMediaMaxDuration, maxMediaMaxDuration)
	}
	cfg.MaxDuration = limit
	return cfg, nil
}

// clampRecordingToMedia keeps a finished recording inside the media limit.
// A RECORDING_MAX_DURATION_HOURS longer than MEDIA_MAX_DURATION would let a
// session record audio the media pipeline then refuses, so the recording
// limit is lowered to the media limit, with a warning, instead of failing
// startup.
func clampRecordingToMedia(recordingMaxHours int, media MediaProcessingConfig) time.Duration {
	recording := time.Duration(recordingMaxHours) * time.Hour
	if recording <= media.MaxDuration {
		return recording
	}
	slog.Warn("RECORDING_MAX_DURATION_HOURS exceeds MEDIA_MAX_DURATION; recordings are limited to MEDIA_MAX_DURATION",
		"recording_max_duration_hours", recordingMaxHours, "media_max_duration", media.MaxDuration.String())
	return media.MaxDuration
}
