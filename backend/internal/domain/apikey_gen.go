package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// base58Alphabet excludes 0, O, l, I to avoid visual ambiguity.
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// GenerateAPIKey creates a new API key with format vxs_{env}_{8}_{32}.
// Returns the full key, the display prefix, and the SHA-256 hash for storage.
func GenerateAPIKey(env string) (fullKey, prefix, hash string, err error) {
	short, err := randomBase58(8)
	if err != nil {
		return "", "", "", fmt.Errorf("generate short token: %w", err)
	}
	long, err := randomBase58(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate long token: %w", err)
	}

	prefix = fmt.Sprintf("vxs_%s_%s", env, short)
	fullKey = fmt.Sprintf("%s_%s", prefix, long)
	hash = HashAPIKey(fullKey)
	return fullKey, prefix, hash, nil
}

// HashAPIKey returns the hex-encoded SHA-256 hash of the key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// VerifyAPIKey checks if a key matches a stored hash using constant-time comparison.
func VerifyAPIKey(fullKey, storedHash string) bool {
	computed := HashAPIKey(fullKey)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// ParseKeyPrefix extracts the prefix (vxs_{env}_{8}) from a full API key.
func ParseKeyPrefix(fullKey string) (string, error) {
	// Expected format: vxs_{env}_{8}_{32}
	parts := strings.SplitN(fullKey, "_", 4)
	if len(parts) != 4 {
		return "", fmt.Errorf("invalid API key format: expected 4 segments")
	}
	if parts[0] != "vxs" {
		return "", fmt.Errorf("invalid API key format: must start with vxs")
	}
	if len(parts[2]) != 8 {
		return "", fmt.Errorf("invalid API key format: short token must be 8 chars")
	}
	if len(parts[3]) < 16 {
		return "", fmt.Errorf("invalid API key format: long token too short")
	}
	return fmt.Sprintf("vxs_%s_%s", parts[1], parts[2]), nil
}

func randomBase58(length int) (string, error) {
	alphabetSize := big.NewInt(int64(len(base58Alphabet)))
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", err
		}
		result[i] = base58Alphabet[n.Int64()]
	}
	return string(result), nil
}
