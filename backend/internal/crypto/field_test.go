package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/crypto"
)

func TestFieldEncryptor_RoundTrip(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key := generateTestKey(t)

	plaintext := []byte("Transcription text: The witness stated that...")
	ciphertext, nonce, err := fe.Encrypt(plaintext, key)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, ciphertext)
	assert.Len(t, nonce, crypto.NonceSize)

	decrypted, err := fe.Decrypt(ciphertext, key, nonce)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestFieldEncryptor_RoundTrip_EmptyPlaintext(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key := generateTestKey(t)

	ciphertext, nonce, err := fe.Encrypt([]byte{}, key)
	require.NoError(t, err)

	decrypted, err := fe.Decrypt(ciphertext, key, nonce)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestFieldEncryptor_Decrypt_WrongKey(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	ciphertext, nonce, err := fe.Encrypt([]byte("secret"), key1)
	require.NoError(t, err)

	_, err = fe.Decrypt(ciphertext, key2, nonce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt field")
}

func TestFieldEncryptor_Decrypt_CorruptedCiphertext(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key := generateTestKey(t)

	ciphertext, nonce, err := fe.Encrypt([]byte("data"), key)
	require.NoError(t, err)

	// Flip a byte
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[0] ^= 0xFF

	_, err = fe.Decrypt(corrupted, key, nonce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt field")
}

func TestFieldEncryptor_Decrypt_TamperedNonce(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key := generateTestKey(t)

	ciphertext, nonce, err := fe.Encrypt([]byte("data"), key)
	require.NoError(t, err)

	// Modify nonce
	badNonce := make([]byte, len(nonce))
	copy(badNonce, nonce)
	badNonce[0] ^= 0xFF

	_, err = fe.Decrypt(ciphertext, key, badNonce)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt field")
}

func TestFieldEncryptor_InvalidKeySize(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()

	shortKey := []byte("short")
	_, _, err := fe.Encrypt([]byte("data"), shortKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key must be")

	_, err = fe.Decrypt([]byte("data"), shortKey, make([]byte, crypto.NonceSize))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key must be")
}

func TestFieldEncryptor_InvalidNonceSize(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key := generateTestKey(t)

	ciphertext, _, err := fe.Encrypt([]byte("data"), key)
	require.NoError(t, err)

	_, err = fe.Decrypt(ciphertext, key, []byte("short"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonce must be")
}

func TestFieldEncryptor_UniqueNonces(t *testing.T) {
	t.Parallel()
	fe := crypto.NewFieldEncryptor()
	key := generateTestKey(t)

	_, nonce1, err := fe.Encrypt([]byte("same data"), key)
	require.NoError(t, err)
	_, nonce2, err := fe.Encrypt([]byte("same data"), key)
	require.NoError(t, err)

	assert.NotEqual(t, nonce1, nonce2, "nonces must be random and unique")
}
