package ffmpeg

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// The truncation check must compare against the same cap ffmpeg receives.
func TestPreprocessorOutputCapMatchesFFmpegSizeLimit(t *testing.T) {
	p := NewPreprocessor(nil)
	args := hardenFFmpegArgs([]string{"-i", "/in.wav", "/work/out.wav"}, 0)
	at := slices.Index(args, "-fs")
	if at < 0 {
		t.Fatalf("hardened args lack -fs: %v", args)
	}
	if args[at+1] != strconv.FormatInt(p.maxOutputBytes, 10) {
		t.Fatalf("-fs %s differs from the preprocessor cap %d", args[at+1], p.maxOutputBytes)
	}
	// At 48 kHz mono s16 the cap is reached well inside the 8 h media limit.
	if hours := float64(p.maxOutputBytes) / (48000 * 2) / 3600; hours > 8 {
		t.Fatalf("test premise: cap lasts %.1f h at 48 kHz", hours)
	}
}

// ffmpeg exits 0 when -fs stops it; a WAV that reached the cap must be
// rejected so the worker falls back instead of transcribing a cut copy.
func TestCheckOutputNotTruncated(t *testing.T) {
	p := NewPreprocessor(nil, WithOutputSampleRate(48000))
	p.maxOutputBytes = 1024
	dir := t.TempDir()

	complete := filepath.Join(dir, "complete.wav")
	if err := os.WriteFile(complete, make([]byte, 1023), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.checkOutputNotTruncated(complete); err != nil {
		t.Fatalf("complete output rejected: %v", err)
	}

	cut := filepath.Join(dir, "cut.wav")
	if err := os.WriteFile(cut, make([]byte, 1030), 0o600); err != nil {
		t.Fatal(err)
	}
	err := p.checkOutputNotTruncated(cut)
	if !errors.Is(err, ErrPreprocessOutputTruncated) {
		t.Fatalf("truncated output error = %v, want ErrPreprocessOutputTruncated", err)
	}
	if _, statErr := os.Stat(cut); !os.IsNotExist(statErr) {
		t.Fatalf("truncated output was not removed: %v", statErr)
	}

	if err := p.checkOutputNotTruncated(filepath.Join(dir, "missing.wav")); err == nil {
		t.Fatal("missing output accepted")
	}
}
