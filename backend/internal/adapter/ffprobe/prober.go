package ffprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/voxis/backend/internal/adapter/ffmpeg"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	defaultBinPath = "ffprobe"
	defaultTimeout = 30 * time.Second
)

type probeOutput struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeFormat struct {
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	Duration       string            `json:"duration"`
	BitRate        string            `json:"bit_rate"`
	ProbeScore     int               `json:"probe_score"`
	Tags           map[string]string `json:"tags"`
}

type probeStream struct {
	CodecType  string            `json:"codec_type"`
	CodecName  string            `json:"codec_name"`
	SampleRate string            `json:"sample_rate"`
	Channels   int               `json:"channels"`
	Duration   string            `json:"duration"`
	DurationTS probeScalar       `json:"duration_ts"`
	TimeBase   string            `json:"time_base"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

type probeScalar string

// UnmarshalJSON allows ffprobe fields that may be emitted as either strings or numbers.
func (p *probeScalar) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*p = ""
		return nil
	}
	if trimmed[0] == '"' {
		var raw string
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return err
		}
		*p = probeScalar(raw)
		return nil
	}

	*p = probeScalar(string(trimmed))
	return nil
}

// Prober extracts audio metadata using the ffprobe binary.
type Prober struct {
	runner          *ffmpeg.Stitcher
	sandboxRequired bool
	binPath         string
	timeout         time.Duration
	logger          *slog.Logger
}

// Option configures the Prober.
type Option func(*Prober)

// WithSandboxRequired makes media parsing fail closed without bubblewrap.
func WithSandboxRequired(required bool) Option {
	return func(p *Prober) { p.sandboxRequired = required }
}

// WithBinPath sets a custom path to the ffprobe binary.
func WithBinPath(path string) Option {
	return func(p *Prober) { p.binPath = path }
}

// WithTimeout sets the maximum time ffprobe is allowed to run.
func WithTimeout(d time.Duration) Option {
	return func(p *Prober) { p.timeout = d }
}

// New creates a new Prober.
func New(logger *slog.Logger, opts ...Option) *Prober {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Prober{
		binPath: defaultBinPath,
		timeout: defaultTimeout,
		logger:  logger,
	}
	for _, opt := range opts {
		opt(p)
	}
	p.runner = ffmpeg.New(logger, ffmpeg.WithBinPath(p.binPath), ffmpeg.WithSandboxRequired(p.sandboxRequired))
	return p
}

// Available reports whether the ffprobe binary is accessible.
func (p *Prober) Available() bool {
	return p.runner.Available()
}

// Probe extracts audio metadata from the file at the given path.
func (p *Prober) Probe(ctx context.Context, filePath string) (*port.AudioProbeResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	output, err := p.runFFprobe(ctx, filePath)
	if err != nil {
		return nil, err
	}

	audio := findAudioStream(output)
	if audio == nil {
		return nil, fmt.Errorf("no audio stream in %q: %w", filePath, domain.ErrInvalidInput)
	}

	duration, err := parseDuration(
		output.Format.Duration,
		audio.Duration,
		string(audio.DurationTS),
		audio.TimeBase,
		output.Format.Tags,
		audio.Tags,
	)
	if err != nil {
		return nil, err
	}

	return buildResult(audio, &output.Format, duration), nil
}

// runFFprobe executes the ffprobe binary and parses its JSON output.
func (p *Prober) runFFprobe(ctx context.Context, filePath string) (*probeOutput, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	}

	//nolint:gosec // binary path is controlled by the caller via WithBinPath option
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- argv exec (no shell); binPath is operator config, args are literal flags plus our own temp paths.
	stdout, err := p.runner.RunProbe(ctx, args, filePath)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffprobe timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffprobe failed for %q: %w", filePath, domain.ErrInvalidInput)
	}

	var output probeOutput
	if err := json.Unmarshal(stdout, &output); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}
	return &output, nil
}

// findAudioStream returns the first audio stream from ffprobe output, or nil.
func findAudioStream(output *probeOutput) *probeStream {
	for i := range output.Streams {
		if output.Streams[i].CodecType == "audio" {
			return &output.Streams[i]
		}
	}
	return nil
}

// parseDuration extracts and validates the audio duration from available ffprobe fields.
func parseDuration(
	formatDuration string,
	streamDuration string,
	streamDurationTS string,
	streamTimeBase string,
	formatTags map[string]string,
	streamTags map[string]string,
) (float64, error) {
	if duration, ok := parsePositiveFloat(formatDuration); ok {
		return duration, nil
	}
	if duration, ok := parsePositiveFloat(streamDuration); ok {
		return duration, nil
	}
	if duration, ok := parseDurationFromTimestamp(streamDurationTS, streamTimeBase); ok {
		return duration, nil
	}
	if duration, ok := parseDurationFromTags(formatTags); ok {
		return duration, nil
	}
	if duration, ok := parseDurationFromTags(streamTags); ok {
		return duration, nil
	}
	return 0, fmt.Errorf("could not determine duration: %w", domain.ErrInvalidInput)
}

func parsePositiveFloat(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "N/A") {
		return 0, false
	}

	duration, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || duration <= 0 {
		return 0, false
	}
	return duration, true
}

func parseDurationFromTimestamp(durationTS, timeBase string) (float64, bool) {
	trimmedDurationTS := strings.TrimSpace(durationTS)
	trimmedTimeBase := strings.TrimSpace(timeBase)
	if trimmedDurationTS == "" || strings.EqualFold(trimmedDurationTS, "N/A") ||
		trimmedTimeBase == "" || strings.EqualFold(trimmedTimeBase, "N/A") {
		return 0, false
	}

	ts, err := strconv.ParseFloat(trimmedDurationTS, 64)
	if err != nil || ts <= 0 {
		return 0, false
	}

	parts := strings.Split(trimmedTimeBase, "/")
	if len(parts) != 2 {
		return 0, false
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil || num <= 0 {
		return 0, false
	}
	den, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || den <= 0 {
		return 0, false
	}

	duration := ts * (num / den)
	if duration <= 0 {
		return 0, false
	}
	return duration, true
}

func parseDurationFromTags(tags map[string]string) (float64, bool) {
	if len(tags) == 0 {
		return 0, false
	}

	for _, key := range []string{"DURATION", "duration"} {
		raw, ok := tags[key]
		if !ok {
			continue
		}
		if duration, ok := parseClockDuration(raw); ok {
			return duration, true
		}
	}
	return 0, false
}

func parseClockDuration(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Split(trimmed, ":")
	if len(parts) != 3 {
		return 0, false
	}

	hours, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil || hours < 0 {
		return 0, false
	}
	minutes, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || minutes < 0 {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil || seconds < 0 {
		return 0, false
	}

	duration := hours*3600 + minutes*60 + seconds
	if duration <= 0 {
		return 0, false
	}
	return duration, true
}

// buildResult constructs an AudioProbeResult from parsed stream and format data.
func buildResult(audio *probeStream, format *probeFormat, duration float64) *port.AudioProbeResult {
	result := &port.AudioProbeResult{
		Duration:       duration,
		Codec:          audio.CodecName,
		Channels:       audio.Channels,
		FormatName:     format.FormatName,
		FormatLongName: format.FormatLongName,
		ProbeScore:     format.ProbeScore,
		FormatTags:     format.Tags,
		StreamTags:     audio.Tags,
	}

	if sr, err := strconv.Atoi(audio.SampleRate); err == nil {
		result.SampleRate = sr
	}

	bitrateStr := audio.BitRate
	if bitrateStr == "" || bitrateStr == "N/A" {
		bitrateStr = format.BitRate
	}
	if br, err := strconv.ParseInt(bitrateStr, 10, 64); err == nil {
		result.Bitrate = br
	}

	// Parse separate durations for consistency checking
	if fd, err := strconv.ParseFloat(format.Duration, 64); err == nil {
		result.FormatDuration = fd
	}
	if sd, err := strconv.ParseFloat(audio.Duration, 64); err == nil {
		result.StreamDuration = sd
	}

	return result
}

// Compile-time interface check.
var _ port.AudioProber = (*Prober)(nil)
