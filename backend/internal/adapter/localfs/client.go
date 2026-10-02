// Package localfs implements port.StorageClient against a local filesystem
// directory tree. It is the single-copy storage backend for deployments without
// an object store, such as those that keep encrypted media only on a local
// encrypted disk with no replicated copy. Because the on-disk object is the ONLY copy,
// Upload is crash-safe: bytes are streamed to a temp file, fsync'd, then atomically
// renamed into place, and the parent directory is fsync'd so the new entry is
// durable. A crash or ENOSPC mid-write can never leave a truncated final object.
package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// dirPerm restricts every created directory to the owner (defense in depth: the
// data is encrypted, but a single-copy media store should not be world-readable).
const dirPerm os.FileMode = 0o700

// maxWalkEntries hard-bounds the ListByPrefix/BucketSize tree walks (Power-of-10
// rule 2). A tree larger than this returns an error rather than an unbounded scan.
const maxWalkEntries = 5_000_000

// Client stores encrypted objects on a local filesystem directory tree.
type Client struct {
	baseDir string
	logger  *slog.Logger
}

// New creates a localfs storage client rooted at baseDir. The directory is
// created (0o700) if absent and must be an existing, writable directory. baseDir
// must be absolute so key resolution is unambiguous.
func New(baseDir string, logger *slog.Logger) (*Client, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("localfs base dir is required")
	}
	if !filepath.IsAbs(baseDir) {
		return nil, fmt.Errorf("localfs base dir must be absolute: %q", baseDir)
	}
	clean := filepath.Clean(baseDir)
	if err := os.MkdirAll(clean, dirPerm); err != nil {
		return nil, fmt.Errorf("create localfs base dir %q: %w", clean, err)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("stat localfs base dir %q: %w", clean, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("localfs base dir %q is not a directory", clean)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{baseDir: clean, logger: logger}, nil
}

// resolve validates a storage key and returns its absolute on-disk path within
// baseDir. It rejects empty keys, absolute keys, and keys that escape the base
// dir via "..". Every method that accepts a key MUST route through resolve
// (path-traversal is the critical safety boundary for a filesystem backend).
func (c *Client) resolve(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("empty storage key: %w", domain.ErrInvalidInput)
	}
	if path.IsAbs(key) || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("absolute storage key %q rejected: %w", key, domain.ErrInvalidInput)
	}
	// Keys are forward-slash relative paths; clean in slash space, then reject any
	// residual parent traversal.
	cleaned := path.Clean(key)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("storage key %q escapes base dir: %w", key, domain.ErrInvalidInput)
	}
	abs := filepath.Join(c.baseDir, filepath.FromSlash(cleaned))
	// Defense in depth: the joined path must still live under baseDir.
	if abs != c.baseDir && !strings.HasPrefix(abs, c.baseDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage key %q escapes base dir: %w", key, domain.ErrInvalidInput)
	}
	return abs, nil
}

// Upload writes src to the object at key via a crash-safe atomic replace.
// contentType and metadata are intentionally ignored: port.StorageClient's
// Download/DownloadRange return only bytes, so there is no retrieval path for them;
// encryption metadata travels in the media row, not the object.
func (c *Client) Upload(ctx context.Context, key string, src io.Reader, _ string, _ map[string]string) error {
	if src == nil {
		return fmt.Errorf("nil upload source: %w", domain.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dest, err := c.resolve(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(dest)
	if mkErr := os.MkdirAll(dir, dirPerm); mkErr != nil {
		return fmt.Errorf("create dir for %q: %w", key, mkErr)
	}

	// Temp file in the SAME directory → same filesystem → atomic os.Rename.
	tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
	if err != nil {
		return wrapWriteErr("create temp file", key, err)
	}
	tmpName := tmp.Name()
	committed := false
	// Best-effort cleanup on ANY failure path: never leak a partial temp file.
	defer func() {
		if !committed {
			_ = tmp.Close()        //nolint:errcheck // best-effort; real error already returned
			_ = os.Remove(tmpName) //nolint:errcheck // best-effort cleanup
		}
	}()

	if _, err = io.Copy(tmp, src); err != nil {
		return wrapWriteErr("copy upload data", key, err)
	}
	// fsync the data, then close, then rename, then fsync the parent dir so the new
	// directory entry (the rename) is itself durable across a crash.
	if err = tmp.Sync(); err != nil {
		return wrapWriteErr("fsync upload data", key, err)
	}
	if err = tmp.Close(); err != nil {
		return wrapWriteErr("close temp file", key, err)
	}
	if err = os.Rename(tmpName, dest); err != nil {
		return wrapWriteErr("commit upload", key, err)
	}
	committed = true
	if err = syncDir(dir); err != nil {
		return fmt.Errorf("fsync dir for %q: %w", key, err)
	}

	c.logger.Info("uploaded object to localfs", "base", c.baseDir, "key", key)
	return nil
}

// Download opens the object at key. The caller closes the returned reader.
// Returns domain.ErrNotFound if the key does not exist.
func (c *Client) Download(_ context.Context, key string) (io.ReadCloser, error) {
	dest, err := c.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(dest) //nolint:gosec // dest is validated by resolve() against path traversal
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("download %q: %w", key, err)
	}
	return f, nil
}

// DownloadRange returns a byte range of the object at key. If length < 0 it reads
// to EOF (mirrors *gcs.Client.DownloadRange / storage.NewRangeReader semantics the
// chunked DecryptRange block math depends on). Returns domain.ErrNotFound if the
// key does not exist.
func (c *Client) DownloadRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative offset %d: %w", offset, domain.ErrInvalidInput)
	}
	dest, err := c.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(dest) //nolint:gosec // dest is validated by resolve() against path traversal
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("download range %q: %w", key, err)
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close() //nolint:errcheck // best-effort; returning the seek error
		return nil, fmt.Errorf("seek %q to %d: %w", key, offset, err)
	}
	if length < 0 {
		return f, nil // read to EOF — the file itself is the reader
	}
	return &limitedFileReader{r: io.LimitReader(f, length), file: f}, nil
}

// Delete removes the object at key. Mirrors *gcs.Client.Delete exactly: a missing
// object returns domain.ErrNotFound (production callers treat "already gone" as
// success at their own layer).
func (c *Client) Delete(_ context.Context, key string) error {
	dest, err := c.resolve(key)
	if err != nil {
		return err
	}
	if err = os.Remove(dest); err != nil {
		if os.IsNotExist(err) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("delete %q: %w", key, err)
	}
	c.logger.Info("deleted object from localfs", "base", c.baseDir, "key", key)
	return nil
}

// SignedURL is not supported by the localfs backend. Voxis serves audio through
// its own HMAC stream-token endpoint, so no production path requests one, and a
// local filesystem has no signed-URL equivalent.
func (c *Client) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", fmt.Errorf("signed URLs are not supported by the localfs storage backend")
}

// ListByPrefix returns all object keys (base-relative, forward-slash) that start
// with prefix. Transient temp/probe files are skipped. The walk is bounded and
// skips unreadable entries rather than aborting the whole listing.
func (c *Client) ListByPrefix(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	count := 0
	walkErr := filepath.WalkDir(c.baseDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			c.logger.Warn("localfs list: skipping unreadable path", "path", p, "error", err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || isTransientName(d.Name()) {
			return nil
		}
		count++
		if count > maxWalkEntries {
			return fmt.Errorf("localfs list: exceeds %d entries", maxWalkEntries)
		}
		rel, relErr := filepath.Rel(c.baseDir, p)
		if relErr != nil {
			return fmt.Errorf("relativize %q: %w", p, relErr)
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(keys)
	return keys, nil
}

// BucketSize sums the sizes and counts of every stored object by walking the tree.
// It mirrors *gcs.Client.BucketSize (same signature) so the same BucketSizer
// wiring in main.go feeds the admin storage stat for localfs. O(objects); callers
// cache the result.
func (c *Client) BucketSize(_ context.Context) (totalBytes, objectCount int64, err error) {
	walkErr := filepath.WalkDir(c.baseDir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			c.logger.Warn("localfs bucket size: skipping unreadable path", "path", p, "error", walkErr)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || isTransientName(d.Name()) {
			return nil
		}
		objectCount++
		if objectCount > maxWalkEntries {
			return fmt.Errorf("localfs bucket size: exceeds %d objects", maxWalkEntries)
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("stat %q: %w", p, infoErr)
		}
		totalBytes += info.Size()
		return nil
	})
	if walkErr != nil {
		return 0, 0, walkErr
	}
	return totalBytes, objectCount, nil
}

// Ping verifies the base dir exists, is a directory, and is writable (a read-only
// mount — e.g. a failed LUKS remount — passes Stat but fails the probe write).
func (c *Client) Ping(_ context.Context) error {
	info, err := os.Stat(c.baseDir)
	if err != nil {
		return fmt.Errorf("localfs health: base dir %q: %w", c.baseDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("localfs health: base dir %q is not a directory", c.baseDir)
	}
	probe, err := os.CreateTemp(c.baseDir, ".health-*.probe")
	if err != nil {
		return fmt.Errorf("localfs health: base dir %q not writable: %w", c.baseDir, err)
	}
	name := probe.Name()
	if err = probe.Close(); err != nil {
		_ = os.Remove(name) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("localfs health: close probe: %w", err)
	}
	if err = os.Remove(name); err != nil {
		return fmt.Errorf("localfs health: remove probe: %w", err)
	}
	return nil
}

// Close is a no-op: the client holds no long-lived resources.
func (c *Client) Close() error {
	return nil
}

// limitedFileReader reads at most N bytes from an underlying file and closes the
// file on Close. io.LimitReader is not itself an io.Closer, and the file must be
// closed to avoid a descriptor leak.
type limitedFileReader struct {
	r    io.Reader // io.LimitReader over file
	file *os.File
}

func (l *limitedFileReader) Read(p []byte) (int, error) { return l.r.Read(p) }

func (l *limitedFileReader) Close() error { return l.file.Close() }

// syncDir fsyncs a directory so a create/rename within it is durable across a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // dir derives from a resolve()-validated dest path
	if err != nil {
		return fmt.Errorf("open dir for fsync: %w", err)
	}
	if syncErr := d.Sync(); syncErr != nil {
		_ = d.Close() //nolint:errcheck // best-effort; returning the sync error
		return fmt.Errorf("fsync dir: %w", syncErr)
	}
	if err = d.Close(); err != nil {
		return fmt.Errorf("close dir after fsync: %w", err)
	}
	return nil
}

// wrapWriteErr classifies write-path failures, surfacing ENOSPC (disk full)
// explicitly so a full LUKS disk is diagnosable rather than a silent corruption.
func wrapWriteErr(op, key string, err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%s for %q: disk full (ENOSPC): %w", op, key, err)
	}
	return fmt.Errorf("%s for %q: %w", op, key, err)
}

// isTransientName reports whether name is an in-flight temp or health-probe file
// that must be excluded from listings and size accounting.
func isTransientName(name string) bool {
	if strings.HasPrefix(name, ".upload-") && strings.HasSuffix(name, ".tmp") {
		return true
	}
	return strings.HasPrefix(name, ".health-") && strings.HasSuffix(name, ".probe")
}

var _ port.StorageClient = (*Client)(nil)
