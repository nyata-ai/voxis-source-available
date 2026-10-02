package handler

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"sync"
	"syscall"

	"github.com/voxis/backend/internal/domain"
)

func copyUploadSpool(dst io.WriteCloser, src io.Reader) (int64, error) {
	written, copyErr := io.Copy(dst, io.LimitReader(src, domain.MaxMediaSize+1))
	closeErr := dst.Close()
	if written > domain.MaxMediaSize {
		return written, &http.MaxBytesError{Limit: domain.MaxMediaSize}
	}
	return written, errors.Join(copyErr, closeErr)
}

// The database shares this volume. Keep 2 GiB free and reserve the full
// per-file limit for every spool in flight, even for chunked HTTP bodies.
const uploadDiskHeadroom uint64 = 2 << 30

// Process-wide cap on simultaneous upload spools (UPLOAD_MAX_CONCURRENT).
// The disk budget above still applies underneath it, so raising the cap on a
// small volume only changes which limit refuses first.
const (
	defaultMaxConcurrentUploads = 4
	maxConcurrentUploadsCeiling = 64
)

type uploadSpoolBudget struct {
	mu        sync.Mutex
	active    int
	maxActive int
	free      func() (uint64, error)
}

func newUploadSpoolBudget(maxActive int) *uploadSpoolBudget {
	return &uploadSpoolBudget{
		maxActive: clampMaxConcurrentUploads(maxActive),
		free: func() (uint64, error) {
			var stat syscall.Statfs_t
			if err := syscall.Statfs(os.TempDir(), &stat); err != nil {
				return 0, err
			}
			return stat.Bavail * uint64(stat.Bsize), nil //nolint:gosec // filesystem block size is positive
		},
	}
}

// clampMaxConcurrentUploads keeps a misconfigured value from disabling uploads
// (0 or negative) or from defeating the disk budget's intent (absurdly large).
func clampMaxConcurrentUploads(n int) int {
	if n < 1 {
		return defaultMaxConcurrentUploads
	}
	return min(n, maxConcurrentUploadsCeiling)
}

func (b *uploadSpoolBudget) reserve() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active >= b.maxActive {
		return fmt.Errorf("upload concurrency limit reached (%d)", b.maxActive)
	}
	free, err := b.free()
	if err != nil {
		return err
	}
	needed := uploadDiskHeadroom + uint64(b.active+1)*uint64(domain.MaxMediaSize) //nolint:gosec // active is bounded above
	if free < needed {
		return fmt.Errorf("insufficient upload scratch space")
	}
	b.active++
	return nil
}

func (b *uploadSpoolBudget) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.active--
}

// maxConcurrentUploadsPerUser keeps one user from holding every process-wide
// upload slot: the rest of the organization can still upload.
const maxConcurrentUploadsPerUser = 2

// userUploadSlots counts in-flight uploads per user. An entry exists only
// while that user has an upload in flight, so the map is bounded by the number
// of concurrent upload requests.
type userUploadSlots struct {
	mu     sync.Mutex
	active map[string]int
}

func newUserUploadSlots() *userUploadSlots {
	return &userUploadSlots{active: make(map[string]int)}
}

func (s *userUploadSlots) acquire(user string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[user] >= maxConcurrentUploadsPerUser {
		return false
	}
	s.active[user]++
	return true
}

func (s *userUploadSlots) release(user string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[user] <= 1 {
		delete(s.active, user)
		return
	}
	s.active[user]--
}

func nextUploadPart(r *http.Request) (*multipart.Part, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	for range 8 {
		part, err := reader.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" && part.FileName() != "" {
			return part, nil
		}
		// Bound ignored form fields instead of letting multipart spill them to disk.
		n, err := io.Copy(io.Discard, io.LimitReader(part, (64<<10)+1))
		if err != nil {
			return nil, err
		}
		if n > 64<<10 {
			return nil, &http.MaxBytesError{Limit: 64 << 10}
		}
		if err := part.Close(); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("too many multipart fields")
}
