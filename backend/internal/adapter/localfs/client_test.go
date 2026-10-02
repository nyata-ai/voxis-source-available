package localfs_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/adapter/localfs"
	"github.com/voxis/backend/internal/domain"
)

func newTestClient(t *testing.T) *localfs.Client {
	t.Helper()
	c, err := localfs.New(t.TempDir(), nil)
	require.NoError(t, err)
	return c
}

func TestNew_RejectsRelativeAndEmpty(t *testing.T) {
	t.Parallel()

	_, err := localfs.New("", nil)
	require.Error(t, err)

	_, err = localfs.New("relative/dir", nil)
	require.Error(t, err)
}

func TestNew_CreatesBaseDir(t *testing.T) {
	t.Parallel()

	base := filepath.Join(t.TempDir(), "nested", "media")
	c, err := localfs.New(base, nil)
	require.NoError(t, err)
	require.NotNil(t, c)

	info, statErr := os.Stat(base)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestClient_UploadAndDownload(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()

	data := []byte("hello, encrypted audio data")
	key := "orgs/org-123/media/m-1/encrypted.bin"

	require.NoError(t, c.Upload(ctx, key, bytes.NewReader(data), "application/octet-stream",
		map[string]string{"org_id": "org-123"}))

	rc, err := c.Download(ctx, key)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestClient_Upload_OverwriteIsAtomic(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()
	key := "orgs/o/media/m/encrypted.bin"

	require.NoError(t, c.Upload(ctx, key, bytes.NewReader([]byte("first-version")), "", nil))
	require.NoError(t, c.Upload(ctx, key, bytes.NewReader([]byte("second")), "", nil))

	rc, err := c.Download(ctx, key)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), got)
}

func TestClient_Upload_NoTempFilesLeftBehind(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	c, err := localfs.New(base, nil)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, c.Upload(ctx, "orgs/o/media/m/encrypted.bin",
		bytes.NewReader([]byte("payload")), "", nil))

	// The upload dir must contain exactly the committed object, no ".upload-*.tmp".
	entries, readErr := os.ReadDir(filepath.Join(base, "orgs", "o", "media", "m"))
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "encrypted.bin", entries[0].Name())
}

func TestClient_Download_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	_, err := c.Download(context.Background(), "nonexistent/key")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestClient_DownloadRange_MidFileWithLength(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()
	require.NoError(t, c.Upload(ctx, "key", bytes.NewReader([]byte("hello world")), "", nil))

	rc, err := c.DownloadRange(ctx, "key", 6, 5)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, []byte("world"), got)
}

func TestClient_DownloadRange_LengthCapsAtEOF(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()
	require.NoError(t, c.Upload(ctx, "key", bytes.NewReader([]byte("hello world")), "", nil))

	// Length exceeds remaining bytes: LimitReader must stop at EOF, not error.
	rc, err := c.DownloadRange(ctx, "key", 6, 100)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, []byte("world"), got)
}

func TestClient_DownloadRange_ToEOF(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()
	require.NoError(t, c.Upload(ctx, "key", bytes.NewReader([]byte("hello world")), "", nil))

	rc, err := c.DownloadRange(ctx, "key", 6, -1)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, []byte("world"), got)
}

func TestClient_DownloadRange_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	_, err := c.DownloadRange(context.Background(), "missing", 0, 10)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestClient_DownloadRange_NegativeOffset(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()
	require.NoError(t, c.Upload(ctx, "key", bytes.NewReader([]byte("data")), "", nil))

	_, err := c.DownloadRange(ctx, "key", -1, 2)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestClient_Delete(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()
	require.NoError(t, c.Upload(ctx, "media/file.enc", bytes.NewReader([]byte("data")), "", nil))

	require.NoError(t, c.Delete(ctx, "media/file.enc"))

	_, err := c.Download(ctx, "media/file.enc")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// Delete mirrors *gcs.Client.Delete: a missing object returns domain.ErrNotFound.
func TestClient_Delete_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	err := c.Delete(context.Background(), "nonexistent/key")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestClient_SignedURL_NotSupported(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	_, err := c.SignedURL(context.Background(), "media/file.enc", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestClient_ListByPrefix(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, c.Upload(ctx, "orgs/a/media/1/encrypted.bin", bytes.NewReader([]byte("x")), "", nil))
	require.NoError(t, c.Upload(ctx, "orgs/a/media/2/encrypted.bin", bytes.NewReader([]byte("y")), "", nil))
	require.NoError(t, c.Upload(ctx, "orgs/b/media/3/encrypted.bin", bytes.NewReader([]byte("z")), "", nil))

	keys, err := c.ListByPrefix(ctx, "orgs/a/")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"orgs/a/media/1/encrypted.bin",
		"orgs/a/media/2/encrypted.bin",
	}, keys)

	all, err := c.ListByPrefix(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestClient_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()

	cases := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"dotdot", ".."},
		{"leading dotdot", "../secret"},
		{"absolute unix", "/etc/passwd"},
		{"embedded escape", "a/../../b"},
		{"deep escape", "orgs/../../../../etc/passwd"},
		{"dot", "."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := c.Upload(ctx, tc.key, bytes.NewReader([]byte("x")), "", nil)
			require.ErrorIs(t, err, domain.ErrInvalidInput, "Upload(%q) must be rejected", tc.key)

			_, err = c.Download(ctx, tc.key)
			require.ErrorIs(t, err, domain.ErrInvalidInput, "Download(%q) must be rejected", tc.key)

			_, err = c.DownloadRange(ctx, tc.key, 0, 1)
			require.ErrorIs(t, err, domain.ErrInvalidInput, "DownloadRange(%q) must be rejected", tc.key)

			err = c.Delete(ctx, tc.key)
			assert.ErrorIs(t, err, domain.ErrInvalidInput, "Delete(%q) must be rejected", tc.key)
		})
	}
}

// TestClient_PathTraversalDoesNotEscape proves a traversal key cannot read a file
// that exists outside the base dir.
func TestClient_PathTraversalDoesNotEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(root, "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("top secret"), 0o600))

	base := filepath.Join(root, "media")
	c, err := localfs.New(base, nil)
	require.NoError(t, err)

	_, err = c.Download(context.Background(), "../secret.txt")
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestClient_BucketSize(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := context.Background()

	total, count, err := c.BucketSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, int64(0), count)

	require.NoError(t, c.Upload(ctx, "orgs/a/media/1/encrypted.bin", bytes.NewReader([]byte("12345")), "", nil))
	require.NoError(t, c.Upload(ctx, "orgs/a/media/2/encrypted.bin", bytes.NewReader([]byte("678")), "", nil))

	total, count, err = c.BucketSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(8), total)
	assert.Equal(t, int64(2), count)
}

func TestClient_Ping_Good(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	assert.NoError(t, c.Ping(context.Background()))
}

func TestClient_Ping_MissingDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	c, err := localfs.New(base, nil)
	require.NoError(t, err)

	// Remove the base dir after construction: Ping must now fail.
	require.NoError(t, os.RemoveAll(base))
	assert.Error(t, c.Ping(context.Background()))
}

func TestClient_Close(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	assert.NoError(t, c.Close())
}
