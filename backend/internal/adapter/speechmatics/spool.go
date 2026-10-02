package speechmatics

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// spoolTokenPrefix marks the opaque staging token Upload returns. It is not a
// URL and is never persisted: the worker threads it straight into Submit.
const spoolTokenPrefix = "smspool:"

// Staging layout. These two names are a cross-component contract: the server's
// global temp sweeper reaps abandoned "voxis-sm-spool-*" directories on its own
// schedule, so neither half may be renamed without the other.
//
// Each process gets its OWN directory from os.MkdirTemp — an unpredictable
// suffix the process creates itself. A fixed path under os.TempDir() could be
// pre-created (or symlinked elsewhere) by any local user before the service
// starts, which would hand decrypted audio to whoever won that race.
const (
	spoolDirPrefix  = "voxis-sm-spool-"
	spoolFilePrefix = "audio-"
	spoolFileSuffix = ".bin"
	spoolFilePatt   = spoolFilePrefix + "*" + spoolFileSuffix
)

// maxSpoolBytes is the staged-file ceiling. Speechmatics accepts up to 1 GB in
// the request body; the headroom below that covers multipart framing so an
// oversized job fails locally with a clear error instead of after a long
// upload. The 60-minute chunker keeps real jobs far under this (a 60-min
// 48 kHz mono 16-bit WAV is ~345 MB).
const maxSpoolBytes int64 = 900 << 20

// staleSpoolDirMaxAge is how old a SIBLING spool directory must be before the
// startup sweep reaps it. A sibling belongs to a different process, so it can
// hold no in-flight submission of ours; an hour is only wide enough to avoid
// racing a sibling that is starting up at the same moment. Anything longer
// would leave decrypted audio on disk for no benefit.
const staleSpoolDirMaxAge = time.Hour

// maxSweepEntries bounds both sweep loops (Power of 10 rule 2). Neither count
// is ours to reason about: os.TempDir() is shared with every other process on
// the host, and a spool directory's file count comes from the filesystem rather
// than from a value this package controls. 10k is far past any plausible real
// count, so the cap only ever fires on a pathological directory — where
// walking the whole thing is exactly what we do not want to do.
const maxSweepEntries = 10000

// spoolFile stages src to a temp file and returns its path. The caller owns the
// file and must remove it. It stops one byte past maxBytes rather than filling
// the disk first, so an oversized job fails locally instead of after a long
// upload.
func spoolFile(dir string, src io.Reader, maxBytes int64, logger *slog.Logger) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("speechmatics: create spool dir: %w", err)
	}

	f, err := os.CreateTemp(dir, spoolFilePatt)
	if err != nil {
		return "", fmt.Errorf("speechmatics: create spool file: %w", err)
	}
	path := f.Name()

	// One extra byte past the ceiling proves the source is oversized.
	written, copyErr := io.Copy(f, io.LimitReader(src, maxBytes+1))
	closeErr := f.Close()

	switch {
	case copyErr != nil:
		removeSpoolFile(path, logger)
		return "", fmt.Errorf("speechmatics: spool audio: %w", copyErr)
	case closeErr != nil:
		removeSpoolFile(path, logger)
		return "", fmt.Errorf("speechmatics: close spool file: %w", closeErr)
	case written > maxBytes:
		removeSpoolFile(path, logger)
		return "", fmt.Errorf("speechmatics: audio exceeds the %d byte submission ceiling: %w",
			maxBytes, domain.ErrInvalidInput)
	}

	return path, nil
}

// spoolToken wraps a spool path in the opaque staging token.
func spoolToken(path string) string { return spoolTokenPrefix + path }

// parseSpoolToken extracts the staged file path from a token produced by Upload
// and proves it names a file this client staged.
//
// The token reaches Submit through a database column, so it is attacker-shaped
// input as far as this package is concerned. Without the two checks below, a
// crafted AudioURL would make Submit open — and then delete — any path on the
// host readable by the service account.
func parseSpoolToken(token, spoolDir string) (string, error) {
	if !strings.HasPrefix(token, spoolTokenPrefix) {
		return "", fmt.Errorf("speechmatics: AudioURL is not a Speechmatics staging token "+
			"(the transcription was staged by a different provider): %w", domain.ErrInvalidInput)
	}
	raw := strings.TrimPrefix(token, spoolTokenPrefix)
	if raw == "" {
		return "", fmt.Errorf("speechmatics: empty staging token: %w", domain.ErrInvalidInput)
	}

	path := filepath.Clean(raw)
	if filepath.Dir(path) != filepath.Clean(spoolDir) {
		return "", fmt.Errorf("speechmatics: staging token points outside the spool directory: %w",
			domain.ErrInvalidInput)
	}
	matched, matchErr := filepath.Match(spoolFilePatt, filepath.Base(path))
	if matchErr != nil || !matched {
		return "", fmt.Errorf("speechmatics: staging token is not a %s file: %w",
			spoolFilePatt, domain.ErrInvalidInput)
	}
	return path, nil
}

// removeSpoolFile deletes a staged file, logging anything but "already gone".
// Plaintext audio must not outlive the submission it was staged for, so this
// runs on every exit path including early errors.
func removeSpoolFile(path string, logger *slog.Logger) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && logger != nil {
		logger.Warn("failed to remove Speechmatics spool file", "path", path, "error", err)
	}
}

// sweepStaleSpoolDirs removes spool directories left behind by earlier
// processes: every "voxis-sm-spool-*" directory under root except self, once it
// is older than maxAge. A crash between Upload and Submit must not leave
// decrypted audio on disk indefinitely.
//
// Only spool-shaped files are removed, and only from a spool-shaped directory,
// so a name collision with something else cannot turn this into a delete
// primitive. Symlinks are skipped: DirEntry.IsDir is lstat-shaped, so a symlink
// pointing at, say, /var/lib never matches. Failures are logged, never fatal —
// a sweep is hygiene, not a precondition for transcribing.
func sweepStaleSpoolDirs(root, self string, maxAge time.Duration, logger *slog.Logger) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && logger != nil {
			logger.Warn("failed to read the Speechmatics spool root", "error", err)
		}
		return
	}

	cutoff := time.Now().Add(-maxAge)
	selfClean := filepath.Clean(self)
	removed := 0

	limit := len(entries)
	if limit > maxSweepEntries {
		limit = maxSweepEntries
	}

	for i := 0; i < limit; i++ {
		if !entries[i].IsDir() || !strings.HasPrefix(entries[i].Name(), spoolDirPrefix) {
			continue
		}
		dir := filepath.Join(root, entries[i].Name())
		if filepath.Clean(dir) == selfClean {
			continue // this process's own directory holds live submissions
		}
		info, infoErr := entries[i].Info()
		if infoErr != nil || info.ModTime().After(cutoff) {
			continue
		}
		removed += purgeSpoolDir(dir, logger)
	}

	if removed > 0 && logger != nil {
		logger.Warn("removed orphaned Speechmatics spool files", "count", removed)
	}
}

// purgeSpoolDir removes the staged files in one abandoned spool directory and
// then the directory itself, returning how many files it deleted. A directory
// holding anything else is left in place: os.Remove fails on a non-empty
// directory, which is the outcome we want.
func purgeSpoolDir(dir string, logger *slog.Logger) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && logger != nil {
			logger.Warn("failed to read an orphaned Speechmatics spool dir", "dir", dir, "error", err)
		}
		return 0
	}

	removed := 0
	limit := len(entries)
	if limit > maxSweepEntries {
		limit = maxSweepEntries
	}

	for i := 0; i < limit; i++ {
		if !entries[i].Type().IsRegular() {
			continue
		}
		matched, matchErr := filepath.Match(spoolFilePatt, entries[i].Name())
		if matchErr != nil || !matched {
			continue
		}
		removeSpoolFile(filepath.Join(dir, entries[i].Name()), logger)
		removed++
	}

	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) && logger != nil {
		logger.Warn("failed to remove an orphaned Speechmatics spool dir", "dir", dir, "error", err)
	}
	return removed
}
