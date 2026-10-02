package ffmpeg

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHardenFFmpegArgsCapsOutputDuration(t *testing.T) {
	args := hardenFFmpegArgs([]string{"-i", "/in.wav", "-c:a", "libopus", "/work/out.webm"}, 8*time.Hour+15*time.Minute)
	if args[len(args)-1] != "/work/out.webm" {
		t.Fatalf("output is not last: %v", args)
	}
	at := slices.Index(args, "-t")
	if at < 0 || args[at+1] != "29700" {
		t.Fatalf("args %v lack -t 29700", args)
	}
	if fs := slices.Index(args, "-fs"); fs < at {
		t.Fatalf("-t must be an output option before -fs and the output: %v", args)
	}
	if uncapped := hardenFFmpegArgs([]string{"-i", "/in.wav", "/work/out.webm"}, 0); slices.Contains(uncapped, "-t") {
		t.Fatalf("zero cap must not add -t: %v", uncapped)
	}
}

func TestSandboxCommandUsesUserAndCgroupNamespaces(t *testing.T) {
	stitcher := New(nil, WithSandboxPath("/usr/bin/bwrap"))
	cmd := stitcher.sandboxCommand(context.Background(), []string{"-version"}, nil, t.TempDir())
	for _, flag := range []string{"--unshare-user", "--unshare-cgroup", "--unshare-net", "--unshare-pid", "--cap-drop"} {
		if !slices.Contains(cmd.Args, flag) {
			t.Fatalf("bwrap args %v lack %s", cmd.Args, flag)
		}
	}
	// --disable-userns needs a writable /proc/sys, which the API container
	// does not have; adding it would stop every media job there.
	if slices.Contains(cmd.Args, "--disable-userns") {
		t.Fatalf("bwrap args %v include --disable-userns", cmd.Args)
	}
}

func TestNewDefaultsOutputCapToMediaLimitPlusGrace(t *testing.T) {
	if got := New(nil).maxOutputDuration; got != 8*time.Hour+15*time.Minute {
		t.Fatalf("default output cap = %s", got)
	}
	if got := New(nil, WithMaxOutputDuration(time.Hour)).maxOutputDuration; got != time.Hour {
		t.Fatalf("configured output cap = %s", got)
	}
}

// TestSandboxedConversionHonorsDurationCap runs the real binaries: it needs
// ffmpeg, ffprobe, and a bubblewrap that can create user namespaces here.
func TestSandboxedConversionHonorsDurationCap(t *testing.T) {
	for _, binary := range []string{"ffmpeg", "ffprobe", "bwrap"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s not installed", binary)
		}
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "tone.wav")
	generate := exec.CommandContext(context.Background(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:duration=5", "-ac", "1", "-ar", "16000", input)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate input: %v: %s", err, out)
	}
	stitcher := New(nil, WithSandboxRequired(true), WithMaxOutputDuration(2*time.Second))
	if !stitcher.Available() {
		t.Fatal("sandbox required but the bwrap probe failed with the hardened flags")
	}
	output := filepath.Join(dir, "capped.webm")
	if err := stitcher.ConvertToWebMOpus(context.Background(), input, output); err != nil {
		t.Fatalf("ConvertToWebMOpus() error = %v", err)
	}
	probe := exec.CommandContext(context.Background(), "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", output)
	raw, err := probe.Output()
	if err != nil {
		t.Fatalf("probe output: %v", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil || seconds > 2.5 {
		t.Fatalf("output duration = %q (%v), want at most the 2s cap", raw, err)
	}
}
