package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/voxis/backend/internal/port"
)

const maxStderrBytes = 64 * 1024 // 64 KB

const (
	defaultPreprocessorPath  = "ffmpeg"
	defaultPreprocessTimeout = 5 * time.Minute
	defaultOutputSampleRate  = 16000
	preprocessorName         = "ffmpeg"
)

// ErrPreprocessOutputTruncated reports that the preprocessed WAV reached the
// ffmpeg output size cap (-fs), so ffmpeg stopped writing before the end of the
// audio. At 48 kHz that happens after about 6.2 hours, inside the media limit.
// Callers fall back to the unenhanced path rather than transcribe a cut copy.
var ErrPreprocessOutputTruncated = errors.New("preprocessed audio reached the output size cap and would be truncated")

// Preprocessor implements port.AudioPreprocessor using ffmpeg.
type Preprocessor struct {
	sandboxRequired   bool
	binPath           string
	timeout           time.Duration
	outputSampleRate  int
	maxOutputDuration time.Duration
	// maxOutputBytes is the -fs cap every ffmpeg output carries. It is a field
	// only so tests can exercise the truncation check with small files.
	maxOutputBytes int64
	logger         *slog.Logger
}

// PreprocessorOption configures the Preprocessor.
type PreprocessorOption func(*Preprocessor)

// WithPreprocessorSandboxRequired applies the same isolation as recording conversion.
func WithPreprocessorSandboxRequired(required bool) PreprocessorOption {
	return func(p *Preprocessor) { p.sandboxRequired = required }
}

// WithPreprocessorMaxOutputDuration applies the same ffmpeg output-duration
// cap as WithMaxOutputDuration.
func WithPreprocessorMaxOutputDuration(d time.Duration) PreprocessorOption {
	return func(p *Preprocessor) { p.maxOutputDuration = d }
}

// WithPreprocessorPath sets a custom path to the ffmpeg binary.
func WithPreprocessorPath(path string) PreprocessorOption {
	return func(p *Preprocessor) { p.binPath = path }
}

// WithPreprocessTimeout sets the maximum time ffmpeg is allowed to run.
func WithPreprocessTimeout(d time.Duration) PreprocessorOption {
	return func(p *Preprocessor) { p.timeout = d }
}

// WithOutputSampleRate sets the output sample rate in Hz.
// Invalid values (zero or negative) are ignored and the default (16000) is kept.
func WithOutputSampleRate(rate int) PreprocessorOption {
	return func(p *Preprocessor) {
		if rate > 0 {
			p.outputSampleRate = rate
		}
	}
}

// NewPreprocessor creates a new Preprocessor.
func NewPreprocessor(logger *slog.Logger, opts ...PreprocessorOption) *Preprocessor {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Preprocessor{
		binPath:          defaultPreprocessorPath,
		timeout:          defaultPreprocessTimeout,
		outputSampleRate: defaultOutputSampleRate,
		maxOutputBytes:   maxFFmpegOutputBytes,
		logger:           logger,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Available reports whether the ffmpeg binary is accessible.
func (p *Preprocessor) Available() bool {
	_, err := exec.LookPath(p.binPath)
	return err == nil
}

// Name returns a human-readable identifier for this preprocessor.
func (p *Preprocessor) Name() string {
	return preprocessorName
}

// Preprocess applies audio processing to inputPath, writing to outputPath.
// The output is WAV format (configurable sample rate, mono, s16).
// Filter chain: highpass=f=80 → loudnorm.
func (p *Preprocessor) Preprocess(ctx context.Context, inputPath, outputPath string) (*port.PreprocessResult, error) {
	// Apply timeout if context has no deadline.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	sampleRateStr := strconv.Itoa(p.outputSampleRate)

	args := []string{
		"-i", inputPath,
		"-af", "highpass=f=80,loudnorm",
		"-ar", sampleRateStr,
		"-ac", "1",
		"-sample_fmt", "s16",
		"-f", "wav",
		"-y",
		outputPath,
	}

	p.logger.Info("preprocessing audio",
		"input", inputPath,
		"output", outputPath,
		"sample_rate", p.outputSampleRate,
		"filter_chain", "highpass=f=80,loudnorm",
	)

	//nolint:gosec // binary path is controlled by the caller via WithPreprocessorPath option
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- argv exec (no shell); binPath is operator config, args are literal flags plus our own temp paths.
	cmd, err := New(p.logger, WithBinPath(p.binPath), WithSandboxRequired(p.sandboxRequired),
		WithMaxOutputDuration(p.maxOutputDuration)).command(ctx, args)
	if err != nil {
		return nil, err
	}
	stderr := &limitedWriter{max: maxStderrBytes}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffmpeg preprocess timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffmpeg preprocess failed: %s: %w", stderr.String(), err)
	}
	// ffmpeg exits successfully when -fs stops it, so check the size here.
	if err := p.checkOutputNotTruncated(outputPath); err != nil {
		return nil, err
	}

	duration := p.probeDuration(ctx, outputPath)

	p.logger.Info("preprocessing complete",
		"input", inputPath,
		"output", outputPath,
		"duration_secs", duration,
	)

	return &port.PreprocessResult{
		OutputPath:       outputPath,
		Applied:          true,
		PreprocessorName: preprocessorName,
		DurationSecs:     duration,
	}, nil
}

// checkOutputNotTruncated fails when ffmpeg's -fs cap cut the output short.
// ffmpeg stops only once the file has reached the cap, so a complete output is
// always smaller. The truncated file is removed.
func (p *Preprocessor) checkOutputNotTruncated(outputPath string) error {
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("stat preprocessed audio: %w", err)
	}
	if info.Size() < p.maxOutputBytes {
		return nil
	}
	_ = os.Remove(outputPath) //nolint:errcheck // best-effort cleanup of the cut file
	return fmt.Errorf("%w (%d bytes at %d Hz)", ErrPreprocessOutputTruncated, info.Size(), p.outputSampleRate)
}

// probeDuration uses ffprobe to get the audio duration in seconds.
// Returns 0 on any error (best-effort).
func (p *Preprocessor) probeDuration(ctx context.Context, path string) float64 {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}

	//nolint:gosec // ffprobe path is resolved from LookPath
	out, err := New(p.logger, WithBinPath(ffprobePath), WithSandboxRequired(p.sandboxRequired)).RunProbe(ctx, []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
	}, path)
	if err != nil {
		return 0
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return duration
}

// limitedWriter caps the amount of data written to prevent unbounded memory use.
type limitedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *limitedWriter) String() string { return w.buf.String() }

// Compile-time interface check.
var _ port.AudioPreprocessor = (*Preprocessor)(nil)
