package port

import (
	"context"
	"io"
	"time"
)

// StorageClient handles encrypted file storage.
type StorageClient interface {
	HealthChecker

	// Upload writes data from src to the given storage key.
	// The contentType and metadata are stored alongside the object.
	Upload(ctx context.Context, key string, src io.Reader, contentType string, metadata map[string]string) error

	// Download retrieves the object at the given storage key.
	// The caller is responsible for closing the returned reader.
	// Returns domain.ErrNotFound if the key does not exist.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// DownloadRange retrieves a byte range of the object at the given key.
	// If length < 0, it reads to EOF.
	// Returns domain.ErrNotFound if the key does not exist.
	DownloadRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)

	// Delete removes the object at the given storage key.
	// Returns domain.ErrNotFound if the key does not exist.
	Delete(ctx context.Context, key string) error

	// SignedURL generates a time-limited URL for direct client access
	// to the object at the given storage key.
	SignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// ListByPrefix returns all object keys matching the given prefix.
	ListByPrefix(ctx context.Context, prefix string) ([]string, error)

	// Close cleanly shuts down the client.
	Close() error
}
