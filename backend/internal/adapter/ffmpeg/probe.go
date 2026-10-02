package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RunProbe reuses the media sandbox with only a read-only input and an empty,
// private scratch directory. It never exposes the application's shared tempdir.
func (s *Stitcher) RunProbe(ctx context.Context, args []string, input string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	work, err := os.MkdirTemp("", "voxis-probe-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }() //nolint:errcheck // best-effort cleanup of our private directory
	input, err = filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	flags := []string{"-protocol_whitelist", "file,crypto,data", "-probesize", "10000000", "-analyzeduration", "30000000", "-max_alloc", "268435456", "-threads", "2"}
	flags = append(flags, args...)
	flags = append(flags, "-i", input)
	cmd, err := s.isolatedCommand(ctx, flags, work)
	if err != nil {
		return nil, err
	}
	stdout := &limitedWriter{max: 4 << 20}
	stderr := &limitedWriter{max: maxStderrBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("media probe: %s: %w", stderr.String(), err)
	}
	if stdout.buf.Len() == stdout.max {
		return nil, fmt.Errorf("media probe output exceeds limit")
	}
	return stdout.buf.Bytes(), nil
}

// RunIsolated runs the configured binary under the same sandbox, for tools whose
// input is a positional argument rather than an "-i" operand. Every path in
// inputs is bound read-only at /inputs/N and workDir is bound writable at
// /work; args carry the host paths and are rewritten in place to those sandbox
// locations, so whatever the binary writes into /work lands in workDir on the
// host. Callers own workDir and must pass absolute paths.
func (s *Stitcher) RunIsolated(ctx context.Context, args, inputs []string, workDir string) error {
	cmd, err := s.isolatedInputCommand(ctx, args, inputs, workDir)
	if err != nil {
		return err
	}
	stderr := &limitedWriter{max: maxStderrBytes}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// isolatedInputCommand is isolatedCommand for positional-input binaries: the
// read-only bindings come from the explicit inputs list instead of being
// inferred from a preceding "-i" flag.
func (s *Stitcher) isolatedInputCommand(
	ctx context.Context,
	args, inputs []string,
	directory string,
) (*exec.Cmd, error) {
	work, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox work directory: %w", err)
	}
	if s.sandboxPath == "" {
		return s.hostCommand(ctx, args, work)
	}
	sandboxArgs, bindings, err := rewriteInputPaths(args, inputs, work)
	if err != nil {
		return nil, err
	}
	return s.sandboxCommand(ctx, sandboxArgs, bindings, work), nil
}

// rewriteInputPaths maps an argument list onto the sandbox: an argument equal to
// a declared input becomes its /inputs/N path, an argument inside workDir
// becomes its /work path, and any other absolute path is refused because the
// sandbox would not expose it.
func rewriteInputPaths(args, inputs []string, workDir string) ([]string, []sandboxBinding, error) {
	bindings := make([]sandboxBinding, 0, len(inputs))
	sandboxPaths := make(map[string]string, len(inputs))
	for _, input := range inputs {
		hostPath, err := filepath.Abs(input)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve sandbox input %q: %w", input, err)
		}
		if _, seen := sandboxPaths[hostPath]; seen {
			continue
		}
		sandboxPath := fmt.Sprintf("/inputs/%d", len(bindings))
		sandboxPaths[hostPath] = sandboxPath
		bindings = append(bindings, sandboxBinding{hostPath: hostPath, sandboxPath: sandboxPath})
	}

	rewritten := slices.Clone(args)
	for i, arg := range rewritten {
		if !filepath.IsAbs(arg) {
			continue
		}
		if sandboxPath, ok := sandboxPaths[arg]; ok {
			rewritten[i] = sandboxPath
			continue
		}
		rel, err := filepath.Rel(workDir, arg)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, nil, fmt.Errorf("sandbox path %q is neither a declared input nor inside the work directory", arg)
		}
		// Sandbox paths are always slash-separated; pathpkg.Join keeps the
		// "/work" prefix intact regardless of host separator.
		rewritten[i] = pathpkg.Join("/work", filepath.ToSlash(rel))
	}
	return rewritten, bindings, nil
}
