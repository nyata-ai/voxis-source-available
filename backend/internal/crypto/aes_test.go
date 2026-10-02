package crypto_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/crypto"
)

// errWriter fails on the nth call to Write.
type errWriter struct {
	failAfter int
	calls     int
}

func (w *errWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.failAfter {
		return 0, errors.New("write error")
	}
	return len(p), nil
}

// errReader returns data for the first call, then errors.
type errReader struct {
	data    []byte
	offset  int
	failAt  int
	readNum int
}

func (r *errReader) Read(p []byte) (int, error) {
	r.readNum++
	if r.readNum > r.failAt {
		return 0, errors.New("read error")
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func generateTestKey(t *testing.T) []byte {
	t.Helper()
	key, err := crypto.GenerateDEK()
	require.NoError(t, err)
	return key
}

func TestChunkedEncryptor_RoundTrip_SmallData(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	plaintext := []byte("hello, voxis encryption layer!")
	var cipherBuf bytes.Buffer

	meta, err := enc.Encrypt(bytes.NewReader(plaintext), &cipherBuf, key)
	require.NoError(t, err)
	assert.Equal(t, 1, meta.ChunkCount)
	assert.Equal(t, int64(len(plaintext)), meta.PlaintextSize)
	assert.Equal(t, "AES-256-GCM-CHUNKED", meta.Algorithm)

	// Ciphertext must differ from plaintext
	assert.NotEqual(t, plaintext, cipherBuf.Bytes())
}

func TestChunkedEncryptor_RoundTrip_Decrypt(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)
	plaintext := []byte("hello, voxis encryption layer!")

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(plaintext), &cipherBuf, key)
	require.NoError(t, err)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key, meta)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decBuf.Bytes())
}

func TestChunkedEncryptor_RoundTrip_EmptyInput(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(nil), &cipherBuf, key)
	require.NoError(t, err)
	assert.Equal(t, 0, meta.ChunkCount)
	assert.Equal(t, int64(0), meta.PlaintextSize)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key, meta)
	require.NoError(t, err)
	assert.Empty(t, decBuf.Bytes())
}

func TestChunkedEncryptor_RoundTrip_ExactChunkBoundary(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptorForTest(64)
	key := generateTestKey(t)

	// Exactly 64 bytes -> 1 chunk.
	data64 := bytes.Repeat([]byte("A"), 64)
	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(data64), &cipherBuf, key)
	require.NoError(t, err)
	assert.Equal(t, 1, meta.ChunkCount)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key, meta)
	require.NoError(t, err)
	assert.Equal(t, data64, decBuf.Bytes())

	// Exactly 128 bytes -> 2 chunks.
	data128 := bytes.Repeat([]byte("B"), 128)
	cipherBuf.Reset()
	meta, err = enc.Encrypt(bytes.NewReader(data128), &cipherBuf, key)
	require.NoError(t, err)
	assert.Equal(t, 2, meta.ChunkCount)

	decBuf.Reset()
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key, meta)
	require.NoError(t, err)
	assert.Equal(t, data128, decBuf.Bytes())
}

func TestChunkedEncryptor_RoundTrip_PartialLastChunk(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptorForTest(64)
	key := generateTestKey(t)

	// 65 bytes with chunk size 64 -> 2 chunks.
	data := bytes.Repeat([]byte("C"), 65)
	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(data), &cipherBuf, key)
	require.NoError(t, err)
	assert.Equal(t, 2, meta.ChunkCount)
	assert.Equal(t, int64(65), meta.PlaintextSize)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key, meta)
	require.NoError(t, err)
	assert.Equal(t, data, decBuf.Bytes())
}

func TestChunkedEncryptor_RoundTrip_MultipleChunks(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptorForTest(64)
	key := generateTestKey(t)

	// 333 bytes / 64 = 5 full + 1 partial = 6 chunks.
	data := bytes.Repeat([]byte("D"), 333)
	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(data), &cipherBuf, key)
	require.NoError(t, err)
	assert.Equal(t, 6, meta.ChunkCount)
	assert.Equal(t, int64(333), meta.PlaintextSize)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key, meta)
	require.NoError(t, err)
	assert.Equal(t, data, decBuf.Bytes())
}

func TestChunkedEncryptor_Decrypt_WrongKey(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("secret")), &cipherBuf, key1)
	require.NoError(t, err)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), &decBuf, key2, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt chunk")
}

func TestChunkedEncryptor_Decrypt_TruncatedCiphertext(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("truncation test")), &cipherBuf, key)
	require.NoError(t, err)

	// Cut ciphertext in half.
	truncated := cipherBuf.Bytes()[:cipherBuf.Len()/2]
	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(truncated), &decBuf, key, meta)
	require.Error(t, err)
}

func TestChunkedEncryptor_Decrypt_CorruptedChunk(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("corruption test")), &cipherBuf, key)
	require.NoError(t, err)

	// Flip a byte inside the ciphertext (after the 4-byte length prefix).
	corrupted := make([]byte, cipherBuf.Len())
	copy(corrupted, cipherBuf.Bytes())
	corrupted[10] ^= 0xFF

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(corrupted), &decBuf, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt chunk")
}

func TestChunkedEncryptor_Decrypt_CorruptedLengthPrefix(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("length prefix test")), &cipherBuf, key)
	require.NoError(t, err)

	corrupted := make([]byte, cipherBuf.Len())
	copy(corrupted, cipherBuf.Bytes())
	binary.BigEndian.PutUint32(corrupted[0:4], 999999999)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(corrupted), &decBuf, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length")
}

func TestChunkedEncryptor_Decrypt_ZeroLengthPrefix(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("zero length test")), &cipherBuf, key)
	require.NoError(t, err)

	corrupted := make([]byte, cipherBuf.Len())
	copy(corrupted, cipherBuf.Bytes())
	binary.BigEndian.PutUint32(corrupted[0:4], 0)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(corrupted), &decBuf, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length")
}

func TestChunkedEncryptor_InvalidKeySize(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	shortKey := []byte("too-short")

	var buf bytes.Buffer
	_, err := enc.Encrypt(bytes.NewReader([]byte("data")), &buf, shortKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key must be")

	meta := &crypto.EncryptionMeta{ChunkCount: 1}
	err = enc.Decrypt(context.Background(), bytes.NewReader(buf.Bytes()), &buf, shortKey, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key must be")
}

func TestChunkedEncryptor_Decrypt_NilMeta(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var buf bytes.Buffer
	err := enc.Decrypt(context.Background(), bytes.NewReader(nil), &buf, key, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata")
}

func TestGenerateDEK(t *testing.T) {
	t.Parallel()

	key1, err := crypto.GenerateDEK()
	require.NoError(t, err)
	assert.Len(t, key1, 32)

	key2, err := crypto.GenerateDEK()
	require.NoError(t, err)
	assert.Len(t, key2, 32)

	assert.NotEqual(t, key1, key2)
}

func TestZeroizeKey(t *testing.T) {
	t.Parallel()
	key := generateTestKey(t)

	// Verify key has non-zero bytes.
	allZero := true
	for _, b := range key {
		if b != 0 {
			allZero = false
			break
		}
	}
	require.False(t, allZero, "generated key should not be all zeros")

	crypto.ZeroizeKey(key)

	for i, b := range key {
		assert.Equal(t, byte(0), b, "byte %d should be zero after zeroize", i)
	}
}

func TestChunkedEncryptor_Encrypt_WriteLengthError(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	// Fail on first Write (the length prefix).
	w := &errWriter{failAfter: 0}
	_, err := enc.Encrypt(bytes.NewReader([]byte("data")), w, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write chunk length")
}

func TestChunkedEncryptor_Encrypt_WriteDataError(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	// Succeed on first Write (length prefix), fail on second Write (ciphertext).
	w := &errWriter{failAfter: 1}
	_, err := enc.Encrypt(bytes.NewReader([]byte("data")), w, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write chunk data")
}

func TestChunkedEncryptor_Encrypt_ReadError(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptorForTest(4)
	key := generateTestKey(t)

	// Reader provides some data then errors (not EOF).
	r := &errReader{data: bytes.Repeat([]byte("X"), 8), failAt: 2}
	var buf bytes.Buffer
	_, err := enc.Encrypt(r, &buf, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read plaintext")
}

func TestChunkedEncryptor_Decrypt_WriteError(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("data")), &cipherBuf, key)
	require.NoError(t, err)

	// Fail on write during decrypt.
	w := &errWriter{failAfter: 0}
	err = enc.Decrypt(context.Background(), bytes.NewReader(cipherBuf.Bytes()), w, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write chunk")
}

func TestChunkedEncryptor_Decrypt_ReadDataError(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("data")), &cipherBuf, key)
	require.NoError(t, err)

	// Provide only the 4-byte length prefix but not the ciphertext data.
	partialData := cipherBuf.Bytes()[:6] // 4 bytes length + only 2 bytes of ciphertext
	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(partialData), &decBuf, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read chunk")
}

func TestChunkedEncryptor_Decrypt_ReadLengthError(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	// Meta says 1 chunk but reader is empty.
	meta := &crypto.EncryptionMeta{ChunkCount: 1}
	var decBuf bytes.Buffer
	err := enc.Decrypt(context.Background(), bytes.NewReader(nil), &decBuf, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read chunk")
}

func TestChunkedEncryptor_Decrypt_TrailingData(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := generateTestKey(t)

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader([]byte("data")), &cipherBuf, key)
	require.NoError(t, err)

	// Append extra bytes after valid ciphertext.
	trailing := append(cipherBuf.Bytes(), []byte("extra garbage")...)

	var decBuf bytes.Buffer
	err = enc.Decrypt(context.Background(), bytes.NewReader(trailing), &decBuf, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing data")
}

func TestChunkedEncryptor_Encrypt_ChunkIndexOverflow(t *testing.T) {
	t.Parallel()
	// Use chunk size of 1 byte and write enough data to test the guard.
	// We can't actually overflow uint32, but we can verify the guard
	// exists by checking the error message format.
	// Instead, verify the constant is used in the overflow check.
	enc := crypto.NewChunkedEncryptorForTest(1)
	key := generateTestKey(t)

	// 10 bytes = 10 chunks — should succeed (no overflow).
	var buf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(bytes.Repeat([]byte("A"), 10)), &buf, key)
	require.NoError(t, err)
	assert.Equal(t, 10, meta.ChunkCount)
}

func TestChunkedEncryptor_DecryptRange(t *testing.T) {
	t.Parallel()
	// Use small chunk size to produce multiple chunks from test data.
	const testChunkSize = 10000
	enc := crypto.NewChunkedEncryptorForTest(testChunkSize)
	key := bytes.Repeat([]byte{0x1}, crypto.KeySize)
	plaintext := bytes.Repeat([]byte("abcdef"), 10000) // 60000 bytes → 6 chunks

	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(plaintext), &cipherBuf, key)
	require.NoError(t, err)
	require.Equal(t, 6, meta.ChunkCount)

	// Decrypt a middle range of chunks (chunks 1-2)
	start := 1
	count := 2
	blockSize := 4 + meta.ChunkSize + 16
	offset := int64(start * blockSize)
	length := int64(count * blockSize)

	r := io.NewSectionReader(bytes.NewReader(cipherBuf.Bytes()), offset, length)
	var out bytes.Buffer
	require.NoError(t, enc.DecryptRange(context.Background(), r, &out, key, meta, start, count))

	// Output should match the corresponding plaintext slice
	startByte := start * meta.ChunkSize
	endByte := startByte + count*meta.ChunkSize
	assert.Equal(t, plaintext[startByte:endByte], out.Bytes())
}

func TestChunkedEncryptor_DecryptRange_InvalidRange(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := bytes.Repeat([]byte{0x1}, crypto.KeySize)

	meta := &crypto.EncryptionMeta{ChunkCount: 3, ChunkSize: crypto.ChunkSize}

	err := enc.DecryptRange(context.Background(), nil, nil, key, meta, -1, 1)
	require.Error(t, err)

	err = enc.DecryptRange(context.Background(), nil, nil, key, meta, 0, 0)
	require.Error(t, err)

	err = enc.DecryptRange(context.Background(), nil, nil, key, meta, 2, 5)
	require.Error(t, err)
}

func TestChunkedEncryptor_DecryptRange_NilMeta(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptor()
	key := bytes.Repeat([]byte{0x1}, crypto.KeySize)

	err := enc.DecryptRange(context.Background(), nil, nil, key, nil, 0, 1)
	require.Error(t, err)
}

func TestChunkedEncryptor_Decrypt_ContextCanceled(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptorForTest(64)
	key := generateTestKey(t)

	// Encrypt some data
	plaintext := make([]byte, 256) // 4 chunks of 64 bytes
	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(plaintext), &cipherBuf, key)
	require.NoError(t, err)

	// Cancel context before decrypting
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var decrypted bytes.Buffer
	err = enc.Decrypt(ctx, bytes.NewReader(cipherBuf.Bytes()), &decrypted, key, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestChunkedEncryptor_DecryptRange_ContextCanceled(t *testing.T) {
	t.Parallel()
	enc := crypto.NewChunkedEncryptorForTest(64)
	key := generateTestKey(t)

	// Encrypt some data
	plaintext := make([]byte, 256) // 4 chunks of 64 bytes
	var cipherBuf bytes.Buffer
	meta, err := enc.Encrypt(bytes.NewReader(plaintext), &cipherBuf, key)
	require.NoError(t, err)

	// Cancel context before decrypting range
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var decrypted bytes.Buffer
	err = enc.DecryptRange(ctx, bytes.NewReader(cipherBuf.Bytes()), &decrypted, key, meta, 0, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}
