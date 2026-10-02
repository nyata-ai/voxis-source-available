package handler

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
)

func TestLocalCiphertextUploadBytesIncludesEveryChunkRecord(t *testing.T) {
	const expectedMaxCiphertextBytes uint64 = 1_073_745_924
	require.Equal(t, expectedMaxCiphertextBytes, localCiphertextUploadBytes())
}

func TestLocalCiphertextRecordOverheadMatchesChunkedEncryptor(t *testing.T) {
	plaintext := bytes.Repeat([]byte{1}, crypto.ChunkSize+1)
	var ciphertext bytes.Buffer
	meta, err := crypto.NewChunkedEncryptor().Encrypt(bytes.NewReader(plaintext), &ciphertext, make([]byte, crypto.KeySize))
	require.NoError(t, err)
	require.Equal(t, 2, meta.ChunkCount)
	require.Equal(t, uint64(len(plaintext))+uint64(meta.ChunkCount)*localCiphertextRecordOverhead, uint64(ciphertext.Len()))
}

func TestLocalUploadSpoolBudgetReservesBothCopiesOnOneFilesystem(t *testing.T) {
	perUpload := uint64(domain.MaxMediaSize) + localCiphertextUploadBytes()
	budget := newLocalUploadSpoolBudget(4, "scratch", "media")
	budget.free = func(string) (localUploadFilesystem, error) {
		return localUploadFilesystem{device: 1, available: uploadDiskHeadroom + 2*perUpload}, nil
	}

	require.NoError(t, budget.reserve())
	require.NoError(t, budget.reserve())
	require.Error(t, budget.reserve(), "a third maximum upload would consume the headroom")
	budget.release()
	require.NoError(t, budget.reserve(), "a released reservation permits the next upload")

	budget = newLocalUploadSpoolBudget(1, "scratch", "media")
	budget.free = func(path string) (localUploadFilesystem, error) {
		available := uploadDiskHeadroom + perUpload
		if path == "media" {
			available--
		}
		return localUploadFilesystem{device: 1, available: available}, nil
	}
	require.Error(t, budget.reserve(), "the lower same-device observation must refuse the upload")
}

func TestLocalUploadSpoolBudgetChecksSeparateScratchAndMediaFilesystems(t *testing.T) {
	plaintext := uint64(domain.MaxMediaSize)
	ciphertext := localCiphertextUploadBytes()
	budget := newLocalUploadSpoolBudget(4, "scratch", "media")
	budget.free = func(path string) (localUploadFilesystem, error) {
		if path == "scratch" {
			return localUploadFilesystem{device: 1, available: uploadDiskHeadroom + 2*plaintext}, nil
		}
		return localUploadFilesystem{device: 2, available: uploadDiskHeadroom + 2*ciphertext}, nil
	}

	require.NoError(t, budget.reserve())
	require.NoError(t, budget.reserve())
	require.Error(t, budget.reserve(), "the next reservation exceeds both distinct budgets")

	budget = newLocalUploadSpoolBudget(1, "scratch", "media")
	budget.free = func(path string) (localUploadFilesystem, error) {
		if path == "scratch" {
			return localUploadFilesystem{device: 1, available: uploadDiskHeadroom + plaintext}, nil
		}
		return localUploadFilesystem{device: 2, available: uploadDiskHeadroom + ciphertext - 1}, nil
	}
	require.Error(t, budget.reserve(), "a media-only shortage must refuse the upload")
}

func TestLocalUploadSpoolBudgetFailsClosedWhenFilesystemStatFails(t *testing.T) {
	for _, failedPath := range []string{"scratch", "media"} {
		t.Run(failedPath, func(t *testing.T) {
			budget := newLocalUploadSpoolBudget(1, "scratch", "media")
			budget.free = func(path string) (localUploadFilesystem, error) {
				if path == failedPath {
					return localUploadFilesystem{}, errors.New("stat failed")
				}
				return localUploadFilesystem{device: 1, available: uploadDiskHeadroom + uint64(domain.MaxMediaSize) + localCiphertextUploadBytes()}, nil
			}
			require.Error(t, budget.reserve())
		})
	}
}
