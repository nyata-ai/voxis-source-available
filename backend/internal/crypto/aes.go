package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	// ChunkSize is the plaintext chunk size (5MB).
	ChunkSize = 5 * 1024 * 1024

	// NonceSize is the AES-GCM nonce length (12 bytes).
	NonceSize = 12

	// KeySize is the AES-256 key length (32 bytes).
	KeySize = 32

	// gcmTagSize is the GCM authentication tag length.
	gcmTagSize = 16

	// chunkLenSize is the 4-byte big-endian chunk length prefix.
	chunkLenSize = 4
)

// EncryptionMeta holds metadata produced by chunked encryption.
type EncryptionMeta struct {
	Algorithm     string `json:"algorithm"`      // "AES-256-GCM-CHUNKED"
	ChunkSize     int    `json:"chunk_size"`     // plaintext bytes per chunk
	ChunkCount    int    `json:"chunk_count"`    // total chunks written
	PlaintextSize int64  `json:"plaintext_size"` // original plaintext size
}

// ChunkedEncryptor encrypts and decrypts streams in fixed-size chunks
// using AES-256-GCM. Each chunk is independently authenticated.
// Nonces are counter-based (safe because each stream uses a unique DEK).
type ChunkedEncryptor struct {
	chunkSize int
}

// NewChunkedEncryptor creates an encryptor with the default 5MB chunk size.
func NewChunkedEncryptor() *ChunkedEncryptor {
	return &ChunkedEncryptor{chunkSize: ChunkSize}
}

// NewChunkedEncryptorForTest creates an encryptor with a custom chunk size.
// Exported for testing only — production code should use NewChunkedEncryptor.
func NewChunkedEncryptorForTest(chunkSize int) *ChunkedEncryptor {
	return &ChunkedEncryptor{chunkSize: chunkSize}
}

// Encrypt reads plaintext from src, writes chunked ciphertext to dst.
// Each chunk: [4-byte length (big-endian)][ciphertext + 16-byte GCM tag].
// Nonces are deterministic: 8 zero bytes + 4-byte chunk index.
func (e *ChunkedEncryptor) Encrypt(src io.Reader, dst io.Writer, key []byte) (*EncryptionMeta, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, e.chunkSize)
	var chunkIndex uint32
	var totalPlaintext int64
	lenBuf := make([]byte, chunkLenSize)

	for {
		n, readErr := io.ReadFull(src, buf)
		if n > 0 {
			nonce := makeNonce(chunkIndex)
			ciphertext := gcm.Seal(nil, nonce, buf[:n], nil)

			// Write length prefix
			binary.BigEndian.PutUint32(lenBuf, uint32(len(ciphertext))) //nolint:gosec // bounded by chunkSize+gcmTagSize
			if _, err := dst.Write(lenBuf); err != nil {
				return nil, fmt.Errorf("crypto: write chunk length: %w", err)
			}

			// Write ciphertext + tag
			if _, err := dst.Write(ciphertext); err != nil {
				return nil, fmt.Errorf("crypto: write chunk data: %w", err)
			}

			if chunkIndex == math.MaxUint32 {
				return nil, fmt.Errorf("crypto: chunk count exceeds maximum (%d); nonce space exhausted", math.MaxUint32)
			}
			chunkIndex++
			totalPlaintext += int64(n)
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("crypto: read plaintext: %w", readErr)
		}
	}

	return &EncryptionMeta{
		Algorithm:     "AES-256-GCM-CHUNKED",
		ChunkSize:     e.chunkSize,
		ChunkCount:    int(chunkIndex),
		PlaintextSize: totalPlaintext,
	}, nil
}

// Decrypt reads chunked ciphertext from src, decrypts it, and writes plaintext
// to dst. It requires the EncryptionMeta produced during encryption.
func (e *ChunkedEncryptor) Decrypt(ctx context.Context, src io.Reader, dst io.Writer, key []byte, meta *EncryptionMeta) error {
	if meta == nil {
		return fmt.Errorf("crypto: encryption metadata must not be nil")
	}

	gcm, err := newGCM(key)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, chunkLenSize)
	maxCipher := e.chunkSize + gcmTagSize + 1024

	for i := 0; i < meta.ChunkCount; i++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("crypto: context canceled during decrypt: %w", err)
		}
		if _, err := io.ReadFull(src, lenBuf); err != nil {
			return fmt.Errorf("crypto: read chunk %d length: %w", i, err)
		}

		chunkLen := binary.BigEndian.Uint32(lenBuf)
		if chunkLen == 0 || int(chunkLen) > maxCipher {
			return fmt.Errorf("crypto: invalid length prefix %d for chunk %d", chunkLen, i)
		}

		ciphertext := make([]byte, chunkLen)
		if _, err := io.ReadFull(src, ciphertext); err != nil {
			return fmt.Errorf("crypto: read chunk %d data: %w", i, err)
		}

		nonce := makeNonce(uint32(i)) //nolint:gosec // ChunkCount bounded by file size / chunkSize
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return fmt.Errorf("crypto: decrypt chunk %d: %w", i, err)
		}

		if _, err := dst.Write(plaintext); err != nil {
			return fmt.Errorf("crypto: write chunk %d plaintext: %w", i, err)
		}
	}

	// Verify the source stream is fully consumed — a tampered ChunkCount
	// that is lower than actual would otherwise silently truncate output.
	var trailer [1]byte
	if n, _ := src.Read(trailer[:]); n > 0 { //nolint:errcheck // only checking if data remains
		return fmt.Errorf("crypto: unexpected trailing data after %d chunks", meta.ChunkCount)
	}

	return nil
}

// DecryptRange decrypts a contiguous range of chunks starting at startChunk.
// It does NOT enforce full-stream consumption (range reads are partial by design).
func (e *ChunkedEncryptor) DecryptRange(ctx context.Context, src io.Reader, dst io.Writer, key []byte, meta *EncryptionMeta, startChunk, chunkCount int) error {
	if meta == nil {
		return fmt.Errorf("crypto: encryption metadata must not be nil")
	}
	if startChunk < 0 || chunkCount <= 0 || startChunk+chunkCount > meta.ChunkCount {
		return fmt.Errorf("crypto: invalid chunk range: start=%d count=%d", startChunk, chunkCount)
	}

	gcm, err := newGCM(key)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, chunkLenSize)
	maxCipher := e.chunkSize + gcmTagSize + 1024

	for i := 0; i < chunkCount; i++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("crypto: context canceled during decrypt range: %w", err)
		}
		if _, err := io.ReadFull(src, lenBuf); err != nil {
			return fmt.Errorf("crypto: read chunk %d length: %w", startChunk+i, err)
		}

		chunkLen := binary.BigEndian.Uint32(lenBuf)
		if chunkLen == 0 || int(chunkLen) > maxCipher {
			return fmt.Errorf("crypto: invalid length prefix %d for chunk %d", chunkLen, startChunk+i)
		}

		ciphertext := make([]byte, chunkLen)
		if _, err := io.ReadFull(src, ciphertext); err != nil {
			return fmt.Errorf("crypto: read chunk %d data: %w", startChunk+i, err)
		}

		nonce := makeNonce(uint32(startChunk + i)) //nolint:gosec // bounded by meta.ChunkCount
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return fmt.Errorf("crypto: decrypt chunk %d: %w", startChunk+i, err)
		}

		if _, err := dst.Write(plaintext); err != nil {
			return fmt.Errorf("crypto: write chunk %d plaintext: %w", startChunk+i, err)
		}
	}

	return nil
}

// newGCM validates the key length and creates an AES-256-GCM cipher.
// After key-length validation, aes.NewCipher and cipher.NewGCM are
// guaranteed to succeed for AES-256, so we treat errors as unreachable.
func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(key))
	}

	// aes.NewCipher only fails for invalid key lengths (16, 24, 32 are valid).
	// We validated KeySize==32 above, so this cannot fail.
	block, _ := aes.NewCipher(key) //nolint:errcheck // guaranteed by key length check above

	// cipher.NewGCM cannot fail with a standard AES block cipher.
	gcm, _ := cipher.NewGCM(block) //nolint:errcheck // guaranteed by AES block cipher

	return gcm, nil
}

// makeNonce builds a 12-byte nonce: 8 zero bytes + 4-byte big-endian chunk index.
func makeNonce(chunkIndex uint32) []byte {
	nonce := make([]byte, NonceSize)
	binary.BigEndian.PutUint32(nonce[8:], chunkIndex)
	return nonce
}

// GenerateDEK generates a random 32-byte Data Encryption Key.
// crypto/rand.Read uses the OS CSPRNG and is documented as always
// returning len(p) bytes on supported platforms (Go 1.25+).
func GenerateDEK() ([]byte, error) {
	key := make([]byte, KeySize)
	_, _ = rand.Read(key) //nolint:errcheck // crypto/rand.Read always succeeds on supported OS
	return key, nil
}

// mustGenerateDEK generates a random 32-byte DEK.
// Panics only if the OS CSPRNG is broken (should never happen).
func mustGenerateDEK() []byte {
	key := make([]byte, KeySize)
	_, _ = rand.Read(key) //nolint:errcheck // crypto/rand.Read always succeeds on supported OS
	return key
}

// ZeroizeKey overwrites a key slice with zeros.
func ZeroizeKey(key []byte) {
	clear(key)
}
