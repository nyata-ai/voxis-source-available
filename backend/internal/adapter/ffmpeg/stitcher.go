package ffmpeg

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	defaultBinPath = "ffmpeg"
	defaultTimeout = 45 * time.Minute
	// Recording output is Opus at 128 kbit/s, so an eight-hour recording is
	// below 500 MiB. This ceiling leaves ample headroom while stopping malformed
	// inputs from filling the host through decompression or muxing expansion.
	maxFFmpegOutputBytes int64 = 2 << 30

	// mp4BoxHeaderSize is the size of a 32-bit ISO-BMFF box header:
	// a uint32 size at offset 0 followed by a 4-byte type at offset 4.
	mp4BoxHeaderSize = 8
)

var ebmlHeader = []byte{0x1A, 0x45, 0xDF, 0xA3}

// mp4InitBoxTypes are the top-level box types that open a self-contained MP4
// stream. A chunk starting with one of these can be demuxed on its own; a
// MediaRecorder fragment continuation (moof/mdat/styp/sidx) cannot.
var mp4InitBoxTypes = []string{"ftyp", "moov"}

// rawConcatFormat identifies a chunked-stream container whose chunks must be
// byte-appended into a single stream before ffmpeg can demux them. The concat
// demuxer requires every input to be independently demuxable, which browser
// MediaRecorder timeslice chunks are not: only chunk 0 carries the container
// initialization.
type rawConcatFormat int

const (
	rawConcatNone rawConcatFormat = iota
	rawConcatWebM
	rawConcatMP4
)

// label returns a short container name for logs and error messages.
func (f rawConcatFormat) label() string {
	switch f {
	case rawConcatWebM:
		return "webm"
	case rawConcatMP4:
		return "mp4"
	default:
		return "none"
	}
}

// tempExt returns the file extension used for the rebuilt intermediate stream.
func (f rawConcatFormat) tempExt() string {
	if f == rawConcatMP4 {
		return ".mp4"
	}
	return ".webm"
}

// Stitcher implements port.AudioStitcher using the ffmpeg binary.
type Stitcher struct {
	binPath           string
	sandboxPath       string
	sandboxRequired   bool
	timeout           time.Duration
	maxOutputDuration time.Duration
	logger            *slog.Logger
}

// Option configures the Stitcher.
type Option func(*Stitcher)

// WithBinPath sets a custom path to the ffmpeg binary.
func WithBinPath(path string) Option {
	return func(s *Stitcher) { s.binPath = path }
}

// WithTimeout sets the maximum time ffmpeg is allowed to run.
func WithTimeout(d time.Duration) Option {
	return func(s *Stitcher) { s.timeout = d }
}

// WithSandboxRequired makes Available fail unless bubblewrap can isolate the
// ffmpeg process. Production enables this so parser bugs cannot reach the API
// service's network or writable filesystem.
func WithSandboxRequired(required bool) Option {
	return func(s *Stitcher) { s.sandboxRequired = required }
}

// WithMaxOutputDuration caps the duration of every ffmpeg output (-t), so a
// file whose container header understates its length cannot expand without
// bound. Callers pass MEDIA_MAX_DURATION plus domain.MediaDurationGrace.
func WithMaxOutputDuration(d time.Duration) Option {
	return func(s *Stitcher) {
		if d > 0 {
			s.maxOutputDuration = d
		}
	}
}

// WithSandboxPath overrides the bubblewrap binary path. Intended for tests and
// installations that do not expose bwrap on PATH.
func WithSandboxPath(path string) Option {
	return func(s *Stitcher) { s.sandboxPath = path }
}

// New creates a new Stitcher.
func New(logger *slog.Logger, opts ...Option) *Stitcher {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Stitcher{
		binPath:           defaultBinPath,
		timeout:           defaultTimeout,
		maxOutputDuration: domain.DefaultMaxMediaDuration + domain.MediaDurationGrace,
		logger:            logger,
	}
	if sandboxPath, err := exec.LookPath("bwrap"); err == nil {
		s.sandboxPath = sandboxPath
	}
	for _, opt := range opts {
		opt(s)
	}
	if resolved, err := exec.LookPath(s.binPath); err == nil {
		s.binPath = resolved
	}
	return s
}

// Available reports whether the ffmpeg binary is accessible.
func (s *Stitcher) Available() bool {
	if _, err := exec.LookPath(s.binPath); err != nil {
		return false
	}
	if !s.sandboxRequired {
		return true
	}
	return s.sandboxPath != "" && s.sandboxProbe() == nil
}

// ConcatToWebMOpus concatenates ordered audio segments into a single WebM/Opus file.
func (s *Stitcher) ConcatToWebMOpus(ctx context.Context, inputPaths []string, outputPath string) error {
	if len(inputPaths) == 0 {
		return fmt.Errorf("no input paths provided")
	}
	if outputPath == "" {
		return fmt.Errorf("output path cannot be empty")
	}
	if err := validateConcatInputs(inputPaths); err != nil {
		return err
	}

	// Intermediate artifacts live beside the output so they land in the
	// caller's free-space-checked stitch directory rather than the system tmp.
	workDir := filepath.Dir(outputPath)

	format, err := detectRawConcatFormat(inputPaths)
	if err != nil {
		return err
	}
	if format != rawConcatNone {
		return s.rebuildStreamAndConvert(ctx, format, inputPaths, workDir, outputPath)
	}

	listPath, err := writeConcatList(workDir, inputPaths)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listPath) }() //nolint:errcheck // best-effort cleanup

	args := []string{
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-vn",
		"-y",
		outputPath,
	}

	s.logger.Info("concatenating recording segments to WebM/Opus",
		"inputs", len(inputPaths),
		"output", outputPath,
	)
	if err := s.runFFmpeg(ctx, args, "ffmpeg concat failed"); err != nil {
		return err
	}

	s.logger.Info("recording concat complete",
		"inputs", len(inputPaths),
		"output", outputPath,
	)
	return nil
}

// validateConcatInputs asserts every input path is non-empty and present on disk.
func validateConcatInputs(inputPaths []string) error {
	for _, inputPath := range inputPaths {
		if inputPath == "" {
			return fmt.Errorf("empty input path provided")
		}
		if _, statErr := os.Stat(inputPath); statErr != nil {
			return fmt.Errorf("stat concat input %q: %w", inputPath, statErr)
		}
	}
	return nil
}

// writeConcatList writes an ffmpeg concat demuxer list file inside dir and
// returns its path. Callers own removal of the returned file.
func writeConcatList(dir string, inputPaths []string) (string, error) {
	listFile, err := os.CreateTemp(dir, "voxis-ffmpeg-concat-*.txt")
	if err != nil {
		return "", fmt.Errorf("create ffmpeg concat list: %w", err)
	}
	listPath := listFile.Name()

	for _, inputPath := range inputPaths {
		listedPath := inputPath
		if rel, relErr := filepath.Rel(dir, inputPath); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			listedPath = rel
		}
		escaped := strings.ReplaceAll(listedPath, "'", "'\\''")
		if _, writeErr := fmt.Fprintf(listFile, "file '%s'\n", escaped); writeErr != nil {
			_ = listFile.Close()    //nolint:errcheck // cleanup path
			_ = os.Remove(listPath) //nolint:errcheck // cleanup path
			return "", fmt.Errorf("write ffmpeg concat list: %w", writeErr)
		}
	}
	if err := listFile.Close(); err != nil {
		_ = os.Remove(listPath) //nolint:errcheck // cleanup path
		return "", fmt.Errorf("close ffmpeg concat list: %w", err)
	}
	return listPath, nil
}

// detectRawConcatFormat reports which chunked-stream container the inputs form,
// or rawConcatNone when every input is independently demuxable.
func detectRawConcatFormat(inputPaths []string) (rawConcatFormat, error) {
	useRawWebM, err := shouldUseRawWebMConcat(inputPaths)
	if err != nil {
		return rawConcatNone, err
	}
	if useRawWebM {
		return rawConcatWebM, nil
	}

	useRawMP4, err := shouldUseRawMP4Concat(inputPaths)
	if err != nil {
		return rawConcatNone, err
	}
	if useRawMP4 {
		return rawConcatMP4, nil
	}

	return rawConcatNone, nil
}

// rebuildStreamAndConvert byte-appends chunked stream inputs into one stream
// inside workDir, then re-encodes it to the canonical WebM/Opus output.
func (s *Stitcher) rebuildStreamAndConvert(
	ctx context.Context,
	format rawConcatFormat,
	inputPaths []string,
	workDir, outputPath string,
) error {
	s.logger.Info("detected chunked stream inputs; rebuilding stream before conversion",
		"container", format.label(),
		"inputs", len(inputPaths),
		"output", outputPath,
	)

	rawPath, err := buildRawStream(inputPaths, workDir, format.tempExt())
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(rawPath) }() //nolint:errcheck // best-effort cleanup

	if convertErr := s.ConvertToWebMOpus(ctx, rawPath, outputPath); convertErr != nil {
		return fmt.Errorf("convert rebuilt %s stream: %w", format.label(), convertErr)
	}
	return nil
}

func shouldUseRawWebMConcat(inputPaths []string) (bool, error) {
	if len(inputPaths) < 2 {
		return false, nil
	}

	firstHasHeader, err := hasEBMLHeader(inputPaths[0])
	if err != nil {
		return false, err
	}
	if !firstHasHeader {
		return false, nil
	}

	for i := 1; i < len(inputPaths); i++ {
		hasHeader, checkErr := hasEBMLHeader(inputPaths[i])
		if checkErr != nil {
			return false, checkErr
		}
		if !hasHeader {
			return true, nil
		}
	}

	return false, nil
}

func hasEBMLHeader(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // validated temp input path
	if err != nil {
		return false, fmt.Errorf("open input %q: %w", path, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // best-effort cleanup

	buf := make([]byte, len(ebmlHeader))
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read input header %q: %w", path, err)
	}
	if n < len(ebmlHeader) {
		return false, nil
	}
	return bytes.Equal(buf, ebmlHeader), nil
}

// shouldUseRawMP4Concat reports whether the inputs form a fragmented MP4
// timeslice stream (Safari's MediaRecorder fallback): chunk 0 carries the
// ftyp/moov initialization and every later chunk is a moof/mdat continuation
// that references chunk 0's empty moov, so no demuxer can open it alone.
func shouldUseRawMP4Concat(inputPaths []string) (bool, error) {
	if len(inputPaths) < 2 {
		return false, nil
	}

	firstType, err := topLevelBoxType(inputPaths[0])
	if err != nil {
		return false, err
	}
	if firstType != "ftyp" {
		return false, nil
	}

	for i := 1; i < len(inputPaths); i++ {
		boxType, checkErr := topLevelBoxType(inputPaths[i])
		if checkErr != nil {
			return false, checkErr
		}
		if !isMP4InitBoxType(boxType) {
			return true, nil
		}
	}

	return false, nil
}

// isMP4InitBoxType reports whether boxType opens a self-contained MP4 stream.
func isMP4InitBoxType(boxType string) bool {
	return slices.Contains(mp4InitBoxTypes, boxType)
}

// topLevelBoxType returns the type of the first ISO-BMFF box in path, or an
// empty string when the file does not begin with a structurally valid box
// header (which includes every non-MP4 container).
func topLevelBoxType(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // validated temp input path
	if err != nil {
		return "", fmt.Errorf("open input %q: %w", path, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // best-effort cleanup

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat input %q: %w", path, err)
	}

	buf := make([]byte, mp4BoxHeaderSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("read input header %q: %w", path, err)
	}
	if n < mp4BoxHeaderSize {
		return "", nil
	}

	boxType := string(buf[4:mp4BoxHeaderSize])
	if !isPrintableBoxType(boxType) {
		return "", nil
	}

	// Size 1 means a 64-bit extended size follows the type; size 0 means the
	// box runs to end of file. Any other value must be a header-sized-or-larger
	// span that fits inside the file.
	size := binary.BigEndian.Uint32(buf[0:4])
	if size > 1 && (size < mp4BoxHeaderSize || int64(size) > info.Size()) {
		return "", nil
	}

	return boxType, nil
}

// isPrintableBoxType reports whether all four bytes are printable ASCII, the
// rule ISO-BMFF box types follow. It rejects arbitrary binary read from a
// non-MP4 container that happens to sit at the box-type offset.
func isPrintableBoxType(boxType string) bool {
	if len(boxType) != 4 {
		return false
	}
	for i := 0; i < len(boxType); i++ {
		if boxType[i] < 0x20 || boxType[i] > 0x7E {
			return false
		}
	}
	return true
}

// buildRawStream byte-appends inputPaths in order into a single temp file
// created inside dir. Callers own removal of the returned path.
func buildRawStream(inputPaths []string, dir, ext string) (string, error) {
	rawFile, err := os.CreateTemp(dir, "voxis-raw-stream-*"+ext)
	if err != nil {
		return "", fmt.Errorf("create raw stream temp file: %w", err)
	}
	rawPath := rawFile.Name()

	for _, inputPath := range inputPaths {
		in, openErr := os.Open(inputPath) //nolint:gosec // validated temp input path
		if openErr != nil {
			_ = rawFile.Close()    //nolint:errcheck // cleanup path
			_ = os.Remove(rawPath) //nolint:errcheck // cleanup path
			return "", fmt.Errorf("open concat input %q: %w", inputPath, openErr)
		}
		_, copyErr := io.Copy(rawFile, in)
		closeErr := in.Close()
		if copyErr != nil || closeErr != nil {
			_ = rawFile.Close()    //nolint:errcheck // cleanup path
			_ = os.Remove(rawPath) //nolint:errcheck // cleanup path
			if copyErr != nil && closeErr != nil {
				return "", fmt.Errorf("append raw stream input %q: %w", inputPath, errors.Join(copyErr, closeErr))
			}
			if copyErr != nil {
				return "", fmt.Errorf("append raw stream input %q: %w", inputPath, copyErr)
			}
			return "", fmt.Errorf("close raw stream input %q: %w", inputPath, closeErr)
		}
	}

	if err := rawFile.Close(); err != nil {
		_ = os.Remove(rawPath) //nolint:errcheck // cleanup path
		return "", fmt.Errorf("close raw stream temp file: %w", err)
	}

	return rawPath, nil
}

// prepareSplitOutput validates the split arguments and creates outputDir.
func prepareSplitOutput(inputPath, outputDir string, maxDurationSec int) error {
	if inputPath == "" {
		return fmt.Errorf("input path cannot be empty")
	}
	if outputDir == "" {
		return fmt.Errorf("output directory cannot be empty")
	}
	if maxDurationSec <= 0 {
		return fmt.Errorf("max duration must be greater than zero")
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return fmt.Errorf("create chunk output directory: %w", err)
	}
	return nil
}

// collectChunks globs the chunk files ffmpeg's segment muxer wrote and returns
// them in chronological order (the %04d pattern makes that a lexical sort).
func collectChunks(outputDir, ext string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(outputDir, "chunk_*"+ext))
	if err != nil {
		return nil, fmt.Errorf("find split chunks: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

// SplitByDuration splits an audio file into sequential chunks.
func (s *Stitcher) SplitByDuration(
	ctx context.Context,
	inputPath, outputDir string,
	maxDurationSec int,
	sourceContentType string,
) ([]string, error) {
	if err := prepareSplitOutput(inputPath, outputDir, maxDurationSec); err != nil {
		return nil, err
	}

	s.logger.Info("splitting audio into chunks",
		"input", inputPath,
		"output_dir", outputDir,
		"max_duration_sec", maxDurationSec,
		"source_content_type", sourceContentType,
	)

	copyExt, segmentFormat, copyEnabled := splitCopyContainer(sourceContentType)
	if copyEnabled {
		copyPattern := filepath.Join(outputDir, "chunk_%04d"+copyExt)
		copyArgs := []string{
			"-i", inputPath,
			"-map", "0:a",
			"-f", "segment",
			"-segment_format", segmentFormat,
			"-segment_time", fmt.Sprintf("%d", maxDurationSec),
			"-reset_timestamps", "1",
			"-c", "copy",
			"-y",
			copyPattern,
		}

		if err := s.runFFmpeg(ctx, copyArgs, "ffmpeg split (stream copy) failed"); err == nil {
			copyChunks, globErr := collectChunks(outputDir, copyExt)
			if globErr != nil {
				return nil, globErr
			}
			if len(copyChunks) > 0 {
				return copyChunks, nil
			}
			s.logger.Warn("copy split produced no chunks, falling back to re-encode")
		} else {
			s.logger.Warn("copy split failed, falling back to re-encode", "error", err)
		}
	} else {
		s.logger.Info("copy split unsupported for source container, using re-encode fallback")
	}

	reencodePattern := filepath.Join(outputDir, "chunk_%04d.webm")
	reencodeArgs := []string{
		"-i", inputPath,
		"-map", "0:a",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", maxDurationSec),
		"-reset_timestamps", "1",
		"-c:a", "libopus",
		"-b:a", "128k",
		"-y",
		reencodePattern,
	}
	if err := s.runFFmpeg(ctx, reencodeArgs, "ffmpeg split (re-encode) failed"); err != nil {
		return nil, err
	}

	paths, err := collectChunks(outputDir, ".webm")
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("ffmpeg split produced no chunks")
	}

	return paths, nil
}

// SplitByDurationWAV splits an audio file into sequential 16-bit 16 kHz mono
// WAV chunks.
//
// Unlike SplitByDuration there is no stream-copy fast path: the point is the
// output format, not preserving the source container. Speechmatics documents
// 16 kHz mono PCM as its optimal input, and the generic split would otherwise
// hand it a lossy Opus/WebM re-encode in a container it does not list as
// supported.
func (s *Stitcher) SplitByDurationWAV(
	ctx context.Context,
	inputPath, outputDir string,
	maxDurationSec int,
) ([]string, error) {
	if err := prepareSplitOutput(inputPath, outputDir, maxDurationSec); err != nil {
		return nil, err
	}

	s.logger.Info("splitting audio into 16 kHz mono WAV chunks",
		"input", inputPath,
		"output_dir", outputDir,
		"max_duration_sec", maxDurationSec,
	)

	args := []string{
		"-i", inputPath,
		"-map", "0:a",
		"-f", "segment",
		"-segment_format", "wav",
		"-segment_time", fmt.Sprintf("%d", maxDurationSec),
		"-reset_timestamps", "1",
		"-c:a", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		filepath.Join(outputDir, "chunk_%04d.wav"),
	}
	if err := s.runFFmpeg(ctx, args, "ffmpeg split (wav) failed"); err != nil {
		return nil, err
	}

	paths, err := collectChunks(outputDir, ".wav")
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("ffmpeg wav split produced no chunks")
	}

	return paths, nil
}

func splitCopyContainer(sourceContentType string) (ext, format string, enabled bool) {
	mediaType, _, err := mime.ParseMediaType(sourceContentType)
	if err != nil {
		return "", "", false
	}

	switch mediaType {
	case "audio/mpeg":
		return ".mp3", "mp3", true
	case "audio/webm":
		return ".webm", "webm", true
	case "audio/ogg", "audio/opus":
		return ".ogg", "ogg", true
	default:
		return "", "", false
	}
}

// ConvertToWebMOpus converts an audio file to WebM/Opus format.
func (s *Stitcher) ConvertToWebMOpus(ctx context.Context, inputPath, outputPath string) error {
	args := []string{
		"-i", inputPath,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-vn", // no video
		"-y",  // overwrite output
		outputPath,
	}

	s.logger.Info("converting audio to WebM/Opus",
		"input", inputPath,
		"output", outputPath,
	)
	if err := s.runFFmpeg(ctx, args, "ffmpeg conversion failed"); err != nil {
		return err
	}

	s.logger.Info("audio conversion complete",
		"input", inputPath,
		"output", outputPath,
	)
	return nil
}

func (s *Stitcher) runFFmpeg(ctx context.Context, args []string, errorPrefix string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	cmd, err := s.command(ctx, args)
	if err != nil {
		return err
	}
	stderr := &limitedWriter{max: maxStderrBytes}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ffmpeg timed out: %w", ctx.Err())
		}
		return fmt.Errorf("%s: %s: %w", errorPrefix, stderr.String(), err)
	}
	return nil
}

func (s *Stitcher) command(ctx context.Context, args []string) (*exec.Cmd, error) {
	hardened := hardenFFmpegArgs(args, s.maxOutputDuration)
	if len(args) == 0 {
		return nil, fmt.Errorf("ffmpeg sandbox requires an output path")
	}
	return s.isolatedCommand(ctx, hardened, filepath.Dir(args[len(args)-1]))
}

func (s *Stitcher) isolatedCommand(ctx context.Context, args []string, directory string) (*exec.Cmd, error) {
	if s.sandboxPath == "" {
		return s.hostCommand(ctx, args, "")
	}
	workDir, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve ffmpeg work directory: %w", err)
	}
	sandboxArgs, inputBindings, err := rewriteSandboxPaths(args, workDir)
	if err != nil {
		return nil, err
	}
	return s.sandboxCommand(ctx, sandboxArgs, inputBindings, workDir), nil
}

// hostCommand runs the binary directly, with a minimal environment but none of
// the namespace isolation. It refuses when the caller demanded a sandbox.
func (s *Stitcher) hostCommand(ctx context.Context, args []string, tmpDir string) (*exec.Cmd, error) {
	if s.sandboxRequired {
		return nil, fmt.Errorf("media sandbox required but bwrap is unavailable")
	}
	//nolint:gosec // binary path is controlled by the caller via WithBinPath option
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- argv exec (no shell); binPath is operator config and args went through hardenFFmpegArgs.
	cmd := exec.CommandContext(ctx, s.binPath, args...)
	cmd.Env = minimalProcessEnv(tmpDir)
	return cmd, nil
}

// sandboxCommand assembles the bubblewrap invocation every media parser shares:
// a tmpfs root over read-only system directories, workDir bound writable at
// /work, and each binding bound read-only under /inputs. args must already
// reference those in-sandbox paths.
func (s *Stitcher) sandboxCommand(
	ctx context.Context,
	args []string,
	inputBindings []sandboxBinding,
	workDir string,
) *exec.Cmd {
	// --disable-userns is deliberately absent: bwrap implements it by writing
	// /proc/sys/user/max_user_namespaces, which is read-only inside the API
	// container, so the sandbox would fail to start there.
	bwrapArgs := sandboxRuntimeArgs()
	bwrapArgs = append(bwrapArgs,
		"--die-with-parent", "--new-session", "--unshare-user",
		"--unshare-net", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup",
		"--bind", workDir, "/work", "--dev", "/dev",
		"--setenv", "PATH", "/usr/local/bin:/usr/bin:/bin", "--setenv", "LANG", "C",
		"--setenv", "LC_ALL", "C", "--setenv", "HOME", "/nonexistent",
		"--setenv", "TMPDIR", "/work", "--chdir", "/work",
	)
	if len(inputBindings) > 0 {
		bwrapArgs = append(bwrapArgs, "--dir", "/inputs")
		for _, binding := range inputBindings {
			bwrapArgs = append(bwrapArgs, "--ro-bind", binding.hostPath, binding.sandboxPath)
		}
	}
	bwrapArgs = append(bwrapArgs, "--", s.binPath)
	bwrapArgs = append(bwrapArgs, args...)
	//nolint:gosec // sandbox path is discovered from PATH or supplied by trusted configuration
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- argv exec (no shell); sandboxPath is operator config and args went through hardenFFmpegArgs + rewriteSandboxPaths.
	cmd := exec.CommandContext(ctx, s.sandboxPath, bwrapArgs...)
	cmd.Env = minimalProcessEnv(workDir)
	return cmd
}

func (s *Stitcher) sandboxProbe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workDir, err := os.MkdirTemp("", "voxis-sandbox-probe-*")
	if err != nil {
		return fmt.Errorf("create bwrap probe directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }() //nolint:errcheck // best-effort cleanup of our private directory
	cmd := s.sandboxCommand(ctx, []string{"-version"}, nil, workDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run bwrap probe: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func sandboxRuntimeArgs() []string {
	return []string{
		"--cap-drop", "ALL", "--clearenv", "--tmpfs", "/",
		"--ro-bind", "/usr", "/usr", "--ro-bind-try", "/usr/local", "/usr/local",
		"--ro-bind-try", "/lib", "/lib", "--ro-bind-try", "/lib64", "/lib64",
		"--dir", "/etc", "--ro-bind-try", "/etc/ld.so.cache", "/etc/ld.so.cache",
		"--ro-bind-try", "/etc/alternatives", "/etc/alternatives",
	}
}

type sandboxBinding struct {
	hostPath    string
	sandboxPath string
}

func rewriteSandboxPaths(args []string, workDir string) ([]string, []sandboxBinding, error) {
	rewritten := slices.Clone(args)
	bindings := make([]sandboxBinding, 0, 1)
	for i, arg := range rewritten {
		if !filepath.IsAbs(arg) {
			continue
		}
		rel, err := filepath.Rel(workDir, arg)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if i == 0 || rewritten[i-1] != "-i" {
				return nil, nil, fmt.Errorf("ffmpeg path %q is outside sandbox work directory", arg)
			}
			sandboxPath := fmt.Sprintf("/inputs/%d", len(bindings))
			bindings = append(bindings, sandboxBinding{hostPath: arg, sandboxPath: sandboxPath})
			rewritten[i] = sandboxPath
			continue
		}
		// Sandbox paths are always slash-separated; pathpkg.Join keeps the
		// "/work" prefix intact regardless of host separator.
		rewritten[i] = pathpkg.Join("/work", filepath.ToSlash(rel))
	}
	return rewritten, bindings, nil
}

func minimalProcessEnv(tmpDir string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "HOME=/nonexistent"}
	if tmpDir != "" {
		env = append(env, "TMPDIR="+tmpDir)
	}
	return env
}

// hardenFFmpegArgs confines every stitcher invocation to local file protocols,
// bounded probing/allocation, a small CPU footprint, and a bounded output size
// and duration. All stitcher call sites pass exactly one output path as the
// final argument.
func hardenFFmpegArgs(args []string, maxOutputDuration time.Duration) []string {
	base := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-protocol_whitelist", "file,crypto,data",
		"-probesize", "10485760",
		"-analyzeduration", "30000000",
		"-max_alloc", "268435456",
		"-cpucount", "2",
		"-threads", "2",
		"-filter_threads", "2",
		"-filter_complex_threads", "2",
	}
	if len(args) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(args)+4)
	out = append(out, base...)
	out = append(out, args[:len(args)-1]...)
	if maxOutputDuration > 0 {
		out = append(out, "-t", strconv.FormatInt(int64(maxOutputDuration/time.Second), 10))
	}
	out = append(out, "-fs", strconv.FormatInt(maxFFmpegOutputBytes, 10), args[len(args)-1])
	return out
}

// Compile-time interface check.
var _ port.AudioStitcher = (*Stitcher)(nil)
var _ port.AudioChunker = (*Stitcher)(nil)
