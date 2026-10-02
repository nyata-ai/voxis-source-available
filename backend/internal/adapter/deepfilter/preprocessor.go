package deepfilter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/voxis/backend/internal/adapter/ffmpeg"
	"github.com/voxis/backend/internal/port"
)

const (
	defaultBinPath          = "deep-filter"
	defaultTimeout          = 10 * time.Minute
	expectedInputSampleRate = 48000
)

// Preprocessor applies DeepFilterNet neural speech enhancement.
// Expects WAV input (typically produced by the FFmpeg preprocessor first).
type Preprocessor struct {
	binPath         string
	sandboxPath     string
	sandboxRequired bool
	timeout         time.Duration
	logger          *slog.Logger
}

// Option configures the DeepFilterNet Preprocessor.
type Option func(*Preprocessor)

// WithBinPath sets a custom path to the deep-filter binary.
func WithBinPath(path string) Option {
	return func(p *Preprocessor) { p.binPath = path }
}

// WithTimeout sets the maximum processing time.
func WithTimeout(d time.Duration) Option {
	return func(p *Preprocessor) { p.timeout = d }
}

// WithSandboxRequired makes Available and Preprocess fail unless bubblewrap can
// isolate deep-filter. Production enables this so a bug in the model runner
// cannot reach the API service's network or writable filesystem.
func WithSandboxRequired(required bool) Option {
	return func(p *Preprocessor) { p.sandboxRequired = required }
}

// WithSandboxPath overrides the bubblewrap binary path. Intended for tests and
// installations that do not expose bwrap on PATH.
func WithSandboxPath(path string) Option {
	return func(p *Preprocessor) { p.sandboxPath = path }
}

// NewPreprocessor creates a new DeepFilterNet preprocessor.
func NewPreprocessor(logger *slog.Logger, opts ...Option) *Preprocessor {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Preprocessor{
		binPath: defaultBinPath,
		timeout: defaultTimeout,
		logger:  logger,
	}
	if sandboxPath, err := exec.LookPath("bwrap"); err == nil {
		p.sandboxPath = sandboxPath
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// runner builds the sandboxed process runner for binPath. deep-filter reuses
// the media sandbox the ffmpeg adapter owns so there is one isolation policy.
func (p *Preprocessor) runner(binPath string) *ffmpeg.Stitcher {
	return ffmpeg.New(p.logger,
		ffmpeg.WithBinPath(binPath),
		ffmpeg.WithSandboxRequired(p.sandboxRequired),
		ffmpeg.WithSandboxPath(p.sandboxPath),
	)
}

// Available reports whether the deep-filter binary is accessible, and — when a
// sandbox is required — that bubblewrap can actually run it.
func (p *Preprocessor) Available() bool {
	if _, err := exec.LookPath(p.binPath); err != nil {
		return false
	}
	if !p.sandboxRequired {
		return true
	}
	return p.sandboxProbe() == nil
}

// sandboxProbe runs `deep-filter --version` inside the sandbox so a broken
// bubblewrap setup surfaces at startup instead of on the first job.
func (p *Preprocessor) sandboxProbe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	work, err := os.MkdirTemp("", "voxis-deepfilter-probe-*")
	if err != nil {
		return fmt.Errorf("create deep-filter probe dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }() //nolint:errcheck // best-effort cleanup of our private directory
	return p.runner(p.binPath).RunIsolated(ctx, []string{"--version"}, nil, work)
}

// Name returns the preprocessor identifier.
func (p *Preprocessor) Name() string { return "deepfilter" }

// Preprocess applies DeepFilterNet speech enhancement to the input WAV file.
// The input MUST be WAV format (run FFmpeg preprocessor first for format conversion).
func (p *Preprocessor) Preprocess(ctx context.Context, inputPath, outputPath string) (*port.PreprocessResult, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return nil, fmt.Errorf("input file: %w", err)
	}
	inputPath, absErr := filepath.Abs(inputPath)
	if absErr != nil {
		return nil, fmt.Errorf("resolve deep-filter input: %w", absErr)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	if err := p.validateInput(ctx, inputPath); err != nil {
		return nil, err
	}

	// DeepFilterNet CLI writes output to a directory with the same base filename
	// as the input. To prevent overwriting the input when both paths share the
	// same directory, we use a dedicated temporary subdirectory for output. That
	// directory is also the sandbox's only writable mount.
	outParent, parentErr := filepath.Abs(filepath.Dir(outputPath))
	if parentErr != nil {
		return nil, fmt.Errorf("resolve deepfilter output dir: %w", parentErr)
	}
	dfOutDir, mkErr := os.MkdirTemp(outParent, "deepfilter-out-*")
	if mkErr != nil {
		return nil, fmt.Errorf("create deepfilter output dir: %w", mkErr)
	}
	defer os.RemoveAll(dfOutDir) //nolint:errcheck // best-effort cleanup

	p.logger.Info("starting DeepFilterNet enhancement",
		"input", inputPath,
		"output_dir", dfOutDir,
	)

	args := []string{inputPath, "-o", dfOutDir}
	if err := p.runner(p.binPath).RunIsolated(ctx, args, []string{inputPath}, dfOutDir); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("deep-filter timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("deep-filter failed: %w", err)
	}

	// Move DeepFilterNet output from temp dir to the requested outputPath. Under
	// the sandbox the input is bound as /inputs/0, so the file deep-filter wrote
	// is named "0" rather than the host basename; resolveOutputFile discovers
	// either name because the directory holds nothing else.
	dfOutput, resolveErr := resolveOutputFile(dfOutDir, inputPath)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if err := os.Rename(dfOutput, outputPath); err != nil {
		return nil, fmt.Errorf("move deep-filter output: %w", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil || info.Size() == 0 {
		return nil, fmt.Errorf("deep-filter produced empty or missing output")
	}

	p.logger.Info("DeepFilterNet enhancement complete",
		"output_size", info.Size(),
	)

	return &port.PreprocessResult{
		OutputPath:       outputPath,
		Applied:          true,
		PreprocessorName: "deepfilter",
	}, nil
}

// validateInput asserts the preconditions DeepFilter expects: WAV container and
// a 48 kHz sample rate.
func (p *Preprocessor) validateInput(ctx context.Context, inputPath string) error {
	formatName, sampleRate, err := p.probeInputProperties(ctx, inputPath)
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(formatName), "wav") {
		return fmt.Errorf("deep-filter input must be WAV, got format %q", formatName)
	}
	if sampleRate != expectedInputSampleRate {
		return fmt.Errorf("deep-filter input must be %d Hz WAV, got %d Hz", expectedInputSampleRate, sampleRate)
	}
	return nil
}

// probeInputProperties reads the container format and sample rate with ffprobe,
// run inside the same sandbox as deep-filter itself.
func (p *Preprocessor) probeInputProperties(ctx context.Context, inputPath string) (formatName string, sampleRate int, err error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", 0, fmt.Errorf("ffprobe not available for deep-filter input validation: %w", err)
	}

	out, err := p.runner(ffprobePath).RunProbe(ctx, []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	}, inputPath)
	if err != nil {
		return "", 0, fmt.Errorf("probe deep-filter input: %w", err)
	}

	var parsed struct {
		Format struct {
			FormatName string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			SampleRate string `json:"sample_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	for _, stream := range parsed.Streams {
		if stream.CodecType != "audio" {
			continue
		}
		sr, convErr := strconv.Atoi(stream.SampleRate)
		if convErr != nil {
			return "", 0, fmt.Errorf("invalid audio sample rate %q", stream.SampleRate)
		}
		return parsed.Format.FormatName, sr, nil
	}

	return "", 0, fmt.Errorf("no audio stream found in deep-filter input")
}

// resolveOutputFile finds the output file produced by deep-filter.
// Some versions preserve the input basename, others may emit a different name.
func resolveOutputFile(outputDir, inputPath string) (string, error) {
	expected := filepath.Join(outputDir, filepath.Base(inputPath))
	if info, err := os.Stat(expected); err == nil && !info.IsDir() {
		return expected, nil
	}

	wavFiles, err := filepath.Glob(filepath.Join(outputDir, "*.wav"))
	if err == nil {
		sort.Strings(wavFiles)
		if len(wavFiles) == 1 {
			return wavFiles[0], nil
		}
		if len(wavFiles) > 1 {
			return "", fmt.Errorf("ambiguous deep-filter output: found %d wav files", len(wavFiles))
		}
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return "", fmt.Errorf("read deep-filter output directory: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, filepath.Join(outputDir, entry.Name()))
	}
	sort.Strings(files)

	switch len(files) {
	case 0:
		return "", fmt.Errorf("deep-filter produced no output files")
	case 1:
		return files[0], nil
	default:
		return "", fmt.Errorf("ambiguous deep-filter output: found %d files", len(files))
	}
}

// Compile-time interface check.
var _ port.AudioPreprocessor = (*Preprocessor)(nil)
