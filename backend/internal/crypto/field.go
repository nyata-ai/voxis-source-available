package crypto

import (
	"crypto/rand"
	"fmt"
)

// FieldEncryptor encrypts and decrypts small byte slices (database fields)
// using AES-256-GCM in a single operation. Not suitable for large data —
// use ChunkedEncryptor for streams.
type FieldEncryptor struct{}

// NewFieldEncryptor creates a new FieldEncryptor.
func NewFieldEncryptor() *FieldEncryptor {
	return &FieldEncryptor{}
}

// Encrypt encrypts plaintext with the given key using AES-256-GCM.
// Returns ciphertext (including GCM tag) and the random nonce used.
func (f *FieldEncryptor) Encrypt(plaintext, key []byte) (ciphertext, nonce []byte, err error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce) //nolint:errcheck // crypto/rand.Read always succeeds on supported OS

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext with the given key and nonce using AES-256-GCM.
func (f *FieldEncryptor) Decrypt(ciphertext, key, nonce []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("crypto: nonce must be %d bytes, got %d", gcm.NonceSize(), len(nonce))
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt field: %w", err)
	}

	return plaintext, nil
}
