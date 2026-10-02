package ffprobe

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSandboxedProbeOfGeneratedWAV runs ffprobe through bubblewrap with the
// sandbox required. It needs ffmpeg, ffprobe, and a bubblewrap that can create
// user namespaces with the hardened flags; it is how the runtime image is
// checked under its seccomp and AppArmor profiles.
func TestSandboxedProbeOfGeneratedWAV(t *testing.T) {
	for _, binary := range []string{"ffmpeg", "ffprobe", "bwrap"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s not installed", binary)
		}
	}
	input := filepath.Join(t.TempDir(), "tone.wav")
	generate := exec.CommandContext(context.Background(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:duration=2", "-ac", "1", "-ar", "16000", input)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate input: %v: %s", err, out)
	}
	prober := New(nil, WithSandboxRequired(true))
	if !prober.Available() {
		t.Fatal("sandbox required but ffprobe is unavailable through bubblewrap")
	}
	result, err := prober.Probe(context.Background(), input)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result.Duration < 1.9 || result.Duration > 2.1 {
		t.Fatalf("Probe() duration = %f, want about 2s", result.Duration)
	}
}
