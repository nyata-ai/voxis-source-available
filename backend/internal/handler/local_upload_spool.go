package handler

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
)

// AES-GCM stream records contain a four-byte length and a 16-byte tag.
const localCiphertextRecordOverhead uint64 = 4 + 16

type localUploadFilesystem struct {
	device    uint64
	available uint64
}

type uploadSpoolReservation interface {
	reserve() error
	release()
}

type localUploadSpoolBudget struct {
	mu         sync.Mutex
	active     int
	maxActive  int
	scratchDir string
	mediaDir   string
	free       func(string) (localUploadFilesystem, error)
}

func newLocalUploadSpoolBudget(maxActive int, scratchDir, mediaDir string) *localUploadSpoolBudget {
	return &localUploadSpoolBudget{
		maxActive:  clampMaxConcurrentUploads(maxActive),
		scratchDir: scratchDir,
		mediaDir:   mediaDir,
		free:       localUploadFilesystemFree,
	}
}

func localUploadFilesystemFree(path string) (localUploadFilesystem, error) {
	info, err := os.Stat(path)
	if err != nil {
		return localUploadFilesystem{}, fmt.Errorf("stat upload path: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return localUploadFilesystem{}, fmt.Errorf("read upload filesystem device")
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(path, &filesystem); err != nil {
		return localUploadFilesystem{}, fmt.Errorf("stat upload filesystem: %w", err)
	}
	return localUploadFilesystem{
		device:    stat.Dev,
		available: filesystem.Bavail * uint64(filesystem.Bsize), //nolint:gosec // filesystem block size is positive
	}, nil
}

func localCiphertextUploadBytes() uint64 {
	plaintext := uint64(domain.MaxMediaSize)
	chunkSize := uint64(crypto.ChunkSize)
	chunkCount := (plaintext + chunkSize - 1) / chunkSize
	return plaintext + chunkCount*localCiphertextRecordOverhead
}

func (b *localUploadSpoolBudget) reserve() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active >= b.maxActive {
		return fmt.Errorf("upload concurrency limit reached (%d)", b.maxActive)
	}
	scratch, err := b.free(b.scratchDir)
	if err != nil {
		return err
	}
	media, err := b.free(b.mediaDir)
	if err != nil {
		return err
	}
	active := b.active
	if active < 0 {
		return fmt.Errorf("invalid active upload count")
	}
	if err := checkLocalUploadSpace(scratch, media, uint64(active)+1); err != nil {
		return err
	}
	b.active++
	return nil
}

func checkLocalUploadSpace(scratch, media localUploadFilesystem, active uint64) error {
	plaintext := uint64(domain.MaxMediaSize)
	ciphertext := localCiphertextUploadBytes()
	if scratch.device == media.device {
		required := uploadDiskHeadroom + active*(plaintext+ciphertext)
		available := scratch.available
		if media.available < available {
			available = media.available
		}
		if available < required {
			return fmt.Errorf("insufficient local upload space")
		}
		return nil
	}
	if scratch.available < uploadDiskHeadroom+active*plaintext {
		return fmt.Errorf("insufficient upload scratch space")
	}
	if media.available < uploadDiskHeadroom+active*ciphertext {
		return fmt.Errorf("insufficient local media space")
	}
	return nil
}

func (b *localUploadSpoolBudget) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active > 0 {
		b.active--
	}
}
