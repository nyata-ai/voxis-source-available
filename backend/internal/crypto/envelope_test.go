package crypto_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/port"
)

// testVault is test-only key material. It is deliberately not an OSS runtime
// provider: production startup requires persistent transit storage.
type testVault struct {
	mu   sync.RWMutex
	keks map[string][]byte
}

func newTestVault() *testVault { return &testVault{keks: make(map[string][]byte)} }

func (v *testVault) Ping(context.Context) error { return nil }

func (v *testVault) GenerateKEK(_ context.Context, orgID string) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	v.mu.Lock()
	v.keks[orgID] = append([]byte(nil), key...)
	v.mu.Unlock()
	return orgID, nil
}

func (v *testVault) GetKEK(_ context.Context, orgID string) ([]byte, error) {
	v.mu.RLock()
	key, ok := v.keks[orgID]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("test KEK missing for %s", orgID)
	}
	return append([]byte(nil), key...), nil
}

func (v *testVault) GetKEKVersions(ctx context.Context, orgID string) ([][]byte, error) {
	key, err := v.GetKEK(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return [][]byte{key}, nil
}

func (*testVault) Close() error { return nil }

var _ port.VaultClient = (*testVault)(nil)

// setupEnvelope creates test-only KEKs and returns an EnvelopeService.
func setupEnvelope(t *testing.T) (*crypto.EnvelopeService, *testVault) {
	t.Helper()
	v := newTestVault()
	_, err := v.GenerateKEK(context.Background(), "org-test")
	require.NoError(t, err)

	logger := slog.Default()
	svc := crypto.NewEnvelopeService(v, logger)
	return svc, v
}

// failVault is a port.VaultClient that always returns an error.
type failVault struct {
	err error
}

func (f *failVault) Ping(_ context.Context) error { return f.err }
func (f *failVault) GenerateKEK(_ context.Context, _ string) (string, error) {
	return "", f.err
}
func (f *failVault) GetKEK(_ context.Context, _ string) ([]byte, error) { return nil, f.err }
func (f *failVault) GetKEKVersions(_ context.Context, _ string) ([][]byte, error) {
	return nil, f.err
}
func (f *failVault) Close() error { return nil }

var _ port.VaultClient = (*failVault)(nil)

func TestEnvelopeService_SealStream_OpenStream_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	// 11KB of random data (multiple chunks won't happen with default 5MB chunk,
	// but it tests the flow).
	plaintext := make([]byte, 11*1024)
	_, err := rand.Read(plaintext)
	require.NoError(t, err)

	// Seal
	var cipherBuf bytes.Buffer
	meta, err := svc.SealStream(ctx, "org-test", bytes.NewReader(plaintext), &cipherBuf)
	require.NoError(t, err)

	assert.NotEmpty(t, meta.WrappedDEK, "WrappedDEK should be populated")
	assert.NotEmpty(t, meta.WrappingNonce, "WrappingNonce should be populated")
	assert.Equal(t, 1, meta.ChunkCount, "11KB should be 1 chunk with 5MB chunk size")
	assert.Equal(t, "AES-256-GCM-CHUNKED", meta.Algorithm)
	assert.Equal(t, int64(11*1024), meta.PlaintextSize)

	// Open
	var plainBuf bytes.Buffer
	err = svc.OpenStream(ctx, "org-test", &cipherBuf, &plainBuf, meta)
	require.NoError(t, err)

	assert.Equal(t, plaintext, plainBuf.Bytes(), "round-trip plaintext must match")
}

func TestEnvelopeService_SealField_OpenField_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	original := []byte("sensitive transcript text")

	meta, err := svc.SealField(ctx, "org-test", original)
	require.NoError(t, err)

	assert.Equal(t, "AES-256-GCM", meta.Algorithm)
	assert.NotEmpty(t, meta.Ciphertext)
	assert.NotEmpty(t, meta.Nonce)
	assert.NotEmpty(t, meta.WrappedDEK)
	assert.NotEmpty(t, meta.WrappingNonce)

	plaintext, err := svc.OpenField(ctx, "org-test", meta)
	require.NoError(t, err)

	assert.Equal(t, original, plaintext, "round-trip field plaintext must match")
}

func TestEnvelopeService_SealStream_VaultUnavailable(t *testing.T) {
	t.Parallel()
	fv := &failVault{err: fmt.Errorf("vault: KEK service down")}
	svc := crypto.NewEnvelopeService(fv, slog.Default())
	ctx := context.Background()

	src := bytes.NewReader([]byte("hello"))
	var dst bytes.Buffer
	_, err := svc.SealStream(ctx, "org-test", src, &dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KEK")
}

func TestEnvelopeService_OpenStream_VaultUnavailable(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	// Seal with working vault
	plaintext := []byte("data to encrypt")
	var cipherBuf bytes.Buffer
	meta, err := svc.SealStream(ctx, "org-test", bytes.NewReader(plaintext), &cipherBuf)
	require.NoError(t, err)

	// Open with broken vault
	fv := &failVault{err: fmt.Errorf("vault: KEK service down")}
	brokenSvc := crypto.NewEnvelopeService(fv, slog.Default())

	var plainBuf bytes.Buffer
	err = brokenSvc.OpenStream(ctx, "org-test", &cipherBuf, &plainBuf, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KEK")
}

func TestEnvelopeService_OpenStream_WrongOrg(t *testing.T) {
	t.Parallel()
	svc, v := setupEnvelope(t)
	ctx := context.Background()

	// Generate KEK for org-b
	_, err := v.GenerateKEK(ctx, "org-b")
	require.NoError(t, err)

	// Seal with org-test
	plaintext := []byte("org-a secret data")
	var cipherBuf bytes.Buffer
	meta, err := svc.SealStream(ctx, "org-test", bytes.NewReader(plaintext), &cipherBuf)
	require.NoError(t, err)

	// Try to open with org-b (different KEK)
	var plainBuf bytes.Buffer
	err = svc.OpenStream(ctx, "org-b", &cipherBuf, &plainBuf, meta)
	require.Error(t, err, "cross-org decrypt should fail")
}

func TestEnvelopeService_SealField_VaultUnavailable(t *testing.T) {
	t.Parallel()
	fv := &failVault{err: fmt.Errorf("vault: KEK unreachable")}
	svc := crypto.NewEnvelopeService(fv, slog.Default())
	ctx := context.Background()

	_, err := svc.SealField(ctx, "org-test", []byte("secret"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KEK")
}

func TestEnvelopeService_OpenStream_NilMeta(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	var buf bytes.Buffer
	err := svc.OpenStream(ctx, "org-test", &buf, &buf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata")
}

func TestEnvelopeService_OpenField_NilMeta(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	_, err := svc.OpenField(ctx, "org-test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata")
}

func TestEnvelopeService_ConcurrentSeal(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	const goroutines = 10
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			data := make([]byte, 1024)
			_, _ = rand.Read(data)

			var cipherBuf bytes.Buffer
			meta, err := svc.SealStream(ctx, "org-test", bytes.NewReader(data), &cipherBuf)
			if err != nil {
				errs[idx] = err
				return
			}

			var plainBuf bytes.Buffer
			if err := svc.OpenStream(ctx, "org-test", &cipherBuf, &plainBuf, meta); err != nil {
				errs[idx] = err
				return
			}

			if !bytes.Equal(data, plainBuf.Bytes()) {
				errs[idx] = fmt.Errorf("goroutine %d: round-trip mismatch", idx)
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d failed", i)
	}
}

func TestEnvelopeService_SealField_DifferentOrgs(t *testing.T) {
	t.Parallel()
	v := newTestVault()
	ctx := context.Background()

	_, err := v.GenerateKEK(ctx, "org-x")
	require.NoError(t, err)
	_, err = v.GenerateKEK(ctx, "org-y")
	require.NoError(t, err)

	svc := crypto.NewEnvelopeService(v, slog.Default())

	original := []byte("shared secret text")

	// Seal with org-x
	metaX, err := svc.SealField(ctx, "org-x", original)
	require.NoError(t, err)

	// Seal with org-y
	metaY, err := svc.SealField(ctx, "org-y", original)
	require.NoError(t, err)

	// Each org can decrypt its own
	ptX, err := svc.OpenField(ctx, "org-x", metaX)
	require.NoError(t, err)
	assert.Equal(t, original, ptX)

	ptY, err := svc.OpenField(ctx, "org-y", metaY)
	require.NoError(t, err)
	assert.Equal(t, original, ptY)

	// Cross-org decrypt must fail: org-x tries to open org-y's data
	_, err = svc.OpenField(ctx, "org-x", metaY)
	require.Error(t, err, "cross-org field decrypt should fail")

	// Cross-org decrypt must fail: org-y tries to open org-x's data
	_, err = svc.OpenField(ctx, "org-y", metaX)
	require.Error(t, err, "cross-org field decrypt should fail")
}

func TestEnvelopeService_SealStream_ReadError(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	// Reader that fails mid-stream.
	r := &errReader{data: bytes.Repeat([]byte("X"), 8), failAt: 1}
	var dst bytes.Buffer
	_, err := svc.SealStream(ctx, "org-test", r, &dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encrypt stream")
}

func TestEnvelopeService_OpenStream_CorruptedCiphertext(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	plaintext := []byte("data that will be corrupted")
	var cipherBuf bytes.Buffer
	meta, err := svc.SealStream(ctx, "org-test", bytes.NewReader(plaintext), &cipherBuf)
	require.NoError(t, err)

	// Corrupt a byte in the ciphertext (after 4-byte length prefix).
	corrupted := make([]byte, cipherBuf.Len())
	copy(corrupted, cipherBuf.Bytes())
	corrupted[10] ^= 0xFF

	var plainBuf bytes.Buffer
	err = svc.OpenStream(ctx, "org-test", bytes.NewReader(corrupted), &plainBuf, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt stream")
}

func TestEnvelopeService_OpenField_CorruptedCiphertext(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	meta, err := svc.SealField(ctx, "org-test", []byte("sensitive data"))
	require.NoError(t, err)

	// Corrupt a byte in the ciphertext.
	meta.Ciphertext[0] ^= 0xFF

	_, err = svc.OpenField(ctx, "org-test", meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt field")
}

// badKEKVault returns a KEK of the wrong length.
type badKEKVault struct {
	keyLen int
}

func (v *badKEKVault) Ping(_ context.Context) error { return nil }
func (v *badKEKVault) GenerateKEK(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (v *badKEKVault) GetKEK(_ context.Context, _ string) ([]byte, error) {
	return make([]byte, v.keyLen), nil
}
func (v *badKEKVault) GetKEKVersions(_ context.Context, _ string) ([][]byte, error) {
	return [][]byte{make([]byte, v.keyLen)}, nil
}
func (v *badKEKVault) Close() error { return nil }

var _ port.VaultClient = (*badKEKVault)(nil)

func TestEnvelopeService_WrapDEK_InvalidKEKLength(t *testing.T) {
	t.Parallel()
	svc := crypto.NewEnvelopeService(&badKEKVault{keyLen: 16}, slog.Default())
	ctx := context.Background()

	// SealStream should fail because KEK is 16 bytes, not 32.
	src := bytes.NewReader([]byte("test data"))
	var dst bytes.Buffer
	_, err := svc.SealStream(ctx, "org-test", src, &dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length")
}

func TestEnvelopeService_UnwrapDEK_InvalidKEKLength(t *testing.T) {
	t.Parallel()
	// First seal with a valid vault.
	goodSvc, _ := setupEnvelope(t)
	ctx := context.Background()

	meta, err := goodSvc.SealField(ctx, "org-test", []byte("secret"))
	require.NoError(t, err)

	// Try to open with a vault that returns wrong-length KEK.
	// The fallback logic skips invalid-length keys and reports no version could decrypt.
	badSvc := crypto.NewEnvelopeService(&badKEKVault{keyLen: 24}, slog.Default())
	_, err = badSvc.OpenField(ctx, "org-test", meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no KEK version could decrypt")
}

func TestEnvelopeService_OpenStreamRange_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()
	plaintext := bytes.Repeat([]byte("hello"), 10000)

	var cipherBuf bytes.Buffer
	meta, err := svc.SealStream(ctx, "org-test", bytes.NewReader(plaintext), &cipherBuf)
	require.NoError(t, err)

	startChunk := 0
	chunkCount := 1
	r := io.NewSectionReader(bytes.NewReader(cipherBuf.Bytes()), 0, int64(cipherBuf.Len()))

	var out bytes.Buffer
	require.NoError(t, svc.OpenStreamRange(ctx, "org-test", r, &out, meta, startChunk, chunkCount))
	assert.Equal(t, plaintext, out.Bytes())
}

func TestEnvelopeService_OpenStreamRange_NilMeta(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnvelope(t)
	ctx := context.Background()

	err := svc.OpenStreamRange(ctx, "org-test", nil, nil, nil, 0, 1)
	require.Error(t, err)
}

// rotatedVault simulates a KEK rotation: GetKEK returns the new (wrong) key,
// but GetKEKVersions returns both new and original keys.
type rotatedVault struct {
	originalKey []byte
	rotatedKey  []byte
}

func (v *rotatedVault) Ping(_ context.Context) error { return nil }
func (v *rotatedVault) GenerateKEK(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (v *rotatedVault) GetKEK(_ context.Context, _ string) ([]byte, error) {
	key := make([]byte, len(v.rotatedKey))
	copy(key, v.rotatedKey)
	return key, nil
}
func (v *rotatedVault) GetKEKVersions(_ context.Context, _ string) ([][]byte, error) {
	// Return newest-first: rotated key, then original key.
	k1 := make([]byte, len(v.rotatedKey))
	copy(k1, v.rotatedKey)
	k2 := make([]byte, len(v.originalKey))
	copy(k2, v.originalKey)
	return [][]byte{k1, k2}, nil
}
func (v *rotatedVault) Close() error { return nil }

var _ port.VaultClient = (*rotatedVault)(nil)

func TestEnvelopeService_UnwrapDEK_FallbackAfterKeyRotation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Step 1: Seal a field with the original KEK.
	originalKey := make([]byte, 32)
	_, err := rand.Read(originalKey)
	require.NoError(t, err)

	originalVault := &rotatedVault{originalKey: originalKey, rotatedKey: originalKey}
	svc1 := crypto.NewEnvelopeService(originalVault, slog.Default())

	plaintext := []byte("secret data for rotation test")
	meta, err := svc1.SealField(ctx, "org-rotation", plaintext)
	require.NoError(t, err)

	// Step 2: Simulate KEK rotation — GetKEK now returns a different key.
	rotatedKey := make([]byte, 32)
	_, err = rand.Read(rotatedKey)
	require.NoError(t, err)

	rotatedVaultObj := &rotatedVault{originalKey: originalKey, rotatedKey: rotatedKey}
	svc2 := crypto.NewEnvelopeService(rotatedVaultObj, slog.Default())

	// Step 3: OpenField should succeed using the fallback (original key).
	result, err := svc2.OpenField(ctx, "org-rotation", meta)
	require.NoError(t, err, "OpenField should succeed by falling back to original KEK version")
	assert.Equal(t, plaintext, result)
}

func TestEnvelopeService_UnwrapDEK_FallbackStream(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Step 1: Seal a stream with the original KEK.
	originalKey := make([]byte, 32)
	_, err := rand.Read(originalKey)
	require.NoError(t, err)

	originalVault := &rotatedVault{originalKey: originalKey, rotatedKey: originalKey}
	svc1 := crypto.NewEnvelopeService(originalVault, slog.Default())

	plaintext := bytes.Repeat([]byte("stream rotation test "), 1000)
	var cipherBuf bytes.Buffer
	meta, err := svc1.SealStream(ctx, "org-rotation", bytes.NewReader(plaintext), &cipherBuf)
	require.NoError(t, err)

	// Step 2: Simulate KEK rotation.
	rotatedKey := make([]byte, 32)
	_, err = rand.Read(rotatedKey)
	require.NoError(t, err)

	rotatedVaultObj := &rotatedVault{originalKey: originalKey, rotatedKey: rotatedKey}
	svc2 := crypto.NewEnvelopeService(rotatedVaultObj, slog.Default())

	// Step 3: OpenStream should succeed via fallback.
	var out bytes.Buffer
	err = svc2.OpenStream(ctx, "org-rotation", bytes.NewReader(cipherBuf.Bytes()), &out, meta)
	require.NoError(t, err, "OpenStream should succeed by falling back to original KEK version")
	assert.Equal(t, plaintext, out.Bytes())
}
