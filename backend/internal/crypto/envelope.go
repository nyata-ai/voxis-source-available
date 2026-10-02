package crypto

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/voxis/backend/internal/port"
)

// SealedMeta holds encryption metadata plus the wrapped DEK.
// Stored alongside the ciphertext (e.g., in the database row).
type SealedMeta struct {
	EncryptionMeta        // embedded stream metadata
	WrappedDEK     []byte `json:"wrapped_dek"`    // DEK encrypted with the org's KEK
	WrappingNonce  []byte `json:"wrapping_nonce"` // nonce used to wrap the DEK
}

// SealedFieldMeta holds encryption metadata for a single field.
type SealedFieldMeta struct {
	Algorithm     string `json:"algorithm"` // "AES-256-GCM"
	Ciphertext    []byte `json:"ciphertext"`
	Nonce         []byte `json:"nonce"`          // nonce used to encrypt the field
	WrappedDEK    []byte `json:"wrapped_dek"`    // DEK encrypted with the org's KEK
	WrappingNonce []byte `json:"wrapping_nonce"` // nonce used to wrap the DEK
}

// EnvelopeService orchestrates the DEK lifecycle (generate, wrap, unwrap)
// using Vault for KEK management and the existing encryptors for data ops.
type EnvelopeService struct {
	vault   port.VaultClient
	chunked *ChunkedEncryptor
	field   *FieldEncryptor
	logger  *slog.Logger
}

// NewEnvelopeService creates an EnvelopeService with default encryptors.
func NewEnvelopeService(vault port.VaultClient, logger *slog.Logger) *EnvelopeService {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnvelopeService{
		vault:   vault,
		chunked: NewChunkedEncryptor(),
		field:   NewFieldEncryptor(),
		logger:  logger,
	}
}

// SealStream generates a DEK, wraps it with the org's KEK (validating Vault
// connectivity first), then encrypts the stream. The plaintext DEK is zeroized
// before returning.
func (e *EnvelopeService) SealStream(ctx context.Context, orgID string, src io.Reader, dst io.Writer) (*SealedMeta, error) {
	start := time.Now()

	dek := mustGenerateDEK()
	defer ZeroizeKey(dek)

	// Wrap DEK BEFORE encrypting — if Vault is down, fail fast without
	// writing orphaned ciphertext to dst.
	wrappedDEK, nonce, err := e.wrapDEK(ctx, orgID, dek)
	if err != nil {
		return nil, err
	}

	meta, err := e.chunked.Encrypt(src, dst, dek)
	if err != nil {
		return nil, fmt.Errorf("envelope: encrypt stream: %w", err)
	}

	e.logger.Info("sealed stream",
		"org_id", orgID,
		"chunks", meta.ChunkCount,
		"plaintext_bytes", meta.PlaintextSize,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return &SealedMeta{
		EncryptionMeta: *meta,
		WrappedDEK:     wrappedDEK,
		WrappingNonce:  nonce,
	}, nil
}

// OpenStream unwraps the DEK using the org's KEK, decrypts the stream,
// and zeroizes the plaintext DEK before returning.
func (e *EnvelopeService) OpenStream(ctx context.Context, orgID string, src io.Reader, dst io.Writer, meta *SealedMeta) error {
	if meta == nil {
		return fmt.Errorf("envelope: sealed metadata must not be nil")
	}

	start := time.Now()

	dek, err := e.unwrapDEK(ctx, orgID, meta.WrappedDEK, meta.WrappingNonce)
	if err != nil {
		return err
	}
	defer ZeroizeKey(dek)

	if err := e.chunked.Decrypt(ctx, src, dst, dek, &meta.EncryptionMeta); err != nil {
		return fmt.Errorf("envelope: decrypt stream: %w", err)
	}

	e.logger.Info("opened stream",
		"org_id", orgID,
		"chunks", meta.ChunkCount,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return nil
}

// OpenStreamRange unwraps the DEK and decrypts a contiguous chunk range.
func (e *EnvelopeService) OpenStreamRange(ctx context.Context, orgID string, src io.Reader, dst io.Writer, meta *SealedMeta, startChunk, chunkCount int) error {
	if meta == nil {
		return fmt.Errorf("envelope: sealed metadata must not be nil")
	}

	dek, err := e.unwrapDEK(ctx, orgID, meta.WrappedDEK, meta.WrappingNonce)
	if err != nil {
		return err
	}
	defer ZeroizeKey(dek)

	if err := e.chunked.DecryptRange(ctx, src, dst, dek, &meta.EncryptionMeta, startChunk, chunkCount); err != nil {
		return fmt.Errorf("envelope: decrypt range: %w", err)
	}
	return nil
}

// SealField generates a DEK, encrypts a small field value, wraps the DEK,
// and zeroizes the plaintext DEK before returning.
func (e *EnvelopeService) SealField(ctx context.Context, orgID string, plaintext []byte) (*SealedFieldMeta, error) {
	dek := mustGenerateDEK()
	defer ZeroizeKey(dek)

	// field.Encrypt cannot fail with a valid 32-byte key from mustGenerateDEK.
	ciphertext, nonce, _ := e.field.Encrypt(plaintext, dek) //nolint:errcheck // guaranteed by key length

	wrappedDEK, wrappingNonce, err := e.wrapDEK(ctx, orgID, dek)
	if err != nil {
		return nil, err
	}

	return &SealedFieldMeta{
		Algorithm:     "AES-256-GCM",
		Ciphertext:    ciphertext,
		Nonce:         nonce,
		WrappedDEK:    wrappedDEK,
		WrappingNonce: wrappingNonce,
	}, nil
}

// OpenField unwraps the DEK using the org's KEK, decrypts the field value,
// and zeroizes the plaintext DEK before returning.
func (e *EnvelopeService) OpenField(ctx context.Context, orgID string, meta *SealedFieldMeta) ([]byte, error) {
	if meta == nil {
		return nil, fmt.Errorf("envelope: sealed field metadata must not be nil")
	}

	dek, err := e.unwrapDEK(ctx, orgID, meta.WrappedDEK, meta.WrappingNonce)
	if err != nil {
		return nil, err
	}
	defer ZeroizeKey(dek)

	plaintext, err := e.field.Decrypt(meta.Ciphertext, dek, meta.Nonce)
	if err != nil {
		return nil, fmt.Errorf("envelope: decrypt field: %w", err)
	}

	return plaintext, nil
}

// wrapDEK encrypts the DEK with the org's KEK using AES-256-GCM (via FieldEncryptor).
func (e *EnvelopeService) wrapDEK(ctx context.Context, orgID string, dek []byte) (wrappedDEK, nonce []byte, err error) {
	kek, err := e.vault.GetKEK(ctx, orgID)
	if err != nil {
		return nil, nil, fmt.Errorf("envelope: get KEK for org %s: %w", orgID, err)
	}
	defer ZeroizeKey(kek)

	if len(kek) != KeySize {
		return nil, nil, fmt.Errorf("envelope: KEK for org %s has invalid length %d, want %d", orgID, len(kek), KeySize)
	}

	wrappedDEK, nonce, err = e.field.Encrypt(dek, kek)
	if err != nil {
		return nil, nil, fmt.Errorf("envelope: wrap DEK: %w", err)
	}

	return wrappedDEK, nonce, nil
}

// unwrapDEK decrypts the wrapped DEK using the org's KEK.
// If the latest KEK version fails (e.g., after key rotation), it tries
// all available versions in descending order as fallback.
func (e *EnvelopeService) unwrapDEK(ctx context.Context, orgID string, wrappedDEK, nonce []byte) ([]byte, error) {
	// Fast path: try latest KEK (cached).
	kek, err := e.vault.GetKEK(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("envelope: get KEK for org %s: %w", orgID, err)
	}

	if len(kek) == KeySize {
		dek, decErr := e.field.Decrypt(wrappedDEK, kek, nonce)
		ZeroizeKey(kek)
		if decErr == nil {
			return dek, nil
		}
		// Latest version failed — fall through to try all versions.
		e.logger.Warn("latest KEK failed to unwrap DEK, trying all versions",
			"org_id", orgID)
	} else {
		ZeroizeKey(kek)
	}

	// Fallback: try all KEK versions (handles key rotation).
	versions, versionsErr := e.vault.GetKEKVersions(ctx, orgID)
	if versionsErr != nil {
		return nil, fmt.Errorf("envelope: get KEK versions for org %s: %w", orgID, versionsErr)
	}

	for i, candidate := range versions {
		if len(candidate) != KeySize {
			ZeroizeKey(candidate)
			continue
		}
		dek, decErr := e.field.Decrypt(wrappedDEK, candidate, nonce)
		ZeroizeKey(candidate)
		// Zeroize remaining candidates we won't try.
		if decErr == nil {
			for j := i + 1; j < len(versions); j++ {
				ZeroizeKey(versions[j])
			}
			return dek, nil
		}
	}

	return nil, fmt.Errorf("envelope: unwrap DEK: no KEK version could decrypt (org %s, tried %d versions)", orgID, len(versions))
}
