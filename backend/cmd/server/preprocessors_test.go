package main

import (
	"path/filepath"
	"testing"

	"github.com/voxis/backend/internal/config"
)

func TestOSSPreprocessorSampleRate(t *testing.T) {
	tests := []struct {
		name               string
		enhancementEnabled bool
		want               int
	}{
		{name: "ffmpeg only", want: 16000},
		{name: "deepfilter", enhancementEnabled: true, want: 48000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ossPreprocessorSampleRate(test.enhancementEnabled); got != test.want {
				t.Fatalf("ossPreprocessorSampleRate(%t) = %d, want %d", test.enhancementEnabled, got, test.want)
			}
		})
	}
}

func TestNewOSSPreprocessorsRejectsUnavailableDeepFilter(t *testing.T) {
	cfg := &config.OSSConfig{Config: config.Config{
		DeepFilterEnabled: true,
		DeepFilterBinPath: filepath.Join(t.TempDir(), "missing-deep-filter"),
	}}
	_, _, err := newOSSPreprocessors(cfg, nil)
	if err == nil || err.Error() != "DeepFilterNet is enabled but unavailable" {
		t.Fatalf("newOSSPreprocessors() error = %v, want unavailable DeepFilterNet error", err)
	}
}
