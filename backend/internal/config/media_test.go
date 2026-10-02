package config

import (
	"strings"
	"testing"
	"time"
)

func setMediaEnv(t *testing.T, sandbox, maxDuration, recordingHours string) {
	t.Helper()
	setValidOSSEnv(t)
	t.Setenv("MEDIA_SANDBOX", sandbox)
	t.Setenv("MEDIA_MAX_DURATION", maxDuration)
	t.Setenv("RECORDING_MAX_DURATION_HOURS", recordingHours)
}

func TestLoadOSSRequiresMediaSandboxInEveryEnvironmentByDefault(t *testing.T) {
	setMediaEnv(t, "", "", "")
	t.Setenv("ENVIRONMENT", "development")

	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v", err)
	}
	if !cfg.Media.SandboxRequired {
		t.Fatal("Media.SandboxRequired = false by default, want true")
	}
	if cfg.Media.MaxDuration != 8*time.Hour {
		t.Fatalf("Media.MaxDuration = %s, want the 8h default", cfg.Media.MaxDuration)
	}
}

func TestLoadOSSMediaSandboxOptOutMustBeExplicit(t *testing.T) {
	setMediaEnv(t, "disabled", "", "")
	cfg, err := LoadOSS()
	if err != nil || cfg.Media.SandboxRequired {
		t.Fatalf("LoadOSS() = %+v, %v; want the sandbox disabled", cfg, err)
	}

	setMediaEnv(t, "off", "", "")
	if _, err := LoadOSS(); err == nil || !strings.Contains(err.Error(), "MEDIA_SANDBOX") {
		t.Fatalf("LoadOSS() error = %v, want an invalid MEDIA_SANDBOX error", err)
	}
}

func TestLoadOSSMediaMaxDurationBounds(t *testing.T) {
	setMediaEnv(t, "", "90m", "1")
	cfg, err := LoadOSS()
	if err != nil || cfg.Media.MaxDuration != 90*time.Minute {
		t.Fatalf("LoadOSS() = %+v, %v; want 90m", cfg, err)
	}
	for _, value := range []string{"10s", "25h", "six hours"} {
		setMediaEnv(t, "", value, "1")
		if _, err := LoadOSS(); err == nil || !strings.Contains(err.Error(), "MEDIA_MAX_DURATION") {
			t.Fatalf("MEDIA_MAX_DURATION=%q error = %v, want a bounds error", value, err)
		}
	}
}

func TestLoadOSSClampsRecordingLimitToMediaLimit(t *testing.T) {
	setMediaEnv(t, "", "6h", "8")
	cfg, err := LoadOSS()
	if err != nil {
		t.Fatalf("LoadOSS() error = %v, want the recording limit clamped, not refused", err)
	}
	if cfg.Recording.MaxDuration != 6*time.Hour {
		t.Fatalf("Recording.MaxDuration = %s, want it clamped to the 6h media limit", cfg.Recording.MaxDuration)
	}

	setMediaEnv(t, "", "30m", "")
	cfg, err = LoadOSS()
	if err != nil || cfg.Recording.MaxDuration != 30*time.Minute {
		t.Fatalf("LoadOSS() = %+v, %v; want the default 8h recording limit clamped to 30m", cfg, err)
	}
}

func TestLoadOSSKeepsRecordingLimitWithinMediaLimit(t *testing.T) {
	setMediaEnv(t, "", "12h", "3")
	cfg, err := LoadOSS()
	if err != nil || cfg.Recording.MaxDuration != 3*time.Hour {
		t.Fatalf("LoadOSS() = %+v, %v; want the 3h recording limit unchanged", cfg, err)
	}
}
