package streamtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/voxis/backend/internal/port"
)

// HMACSigner signs and verifies stream tokens using HMAC-SHA256.
type HMACSigner struct {
	secret []byte
}

// NewHMACSigner creates a new HMACSigner with the given secret.
// The secret must be at least 32 characters for HMAC-SHA256 security.
func NewHMACSigner(secret string) (*HMACSigner, error) {
	if len(secret) < 32 {
		return nil, errors.New("stream token secret must be at least 32 characters")
	}
	return &HMACSigner{secret: []byte(secret)}, nil
}

// Sign creates a signed token from the payload.
func (s *HMACSigner) Sign(payload port.StreamTokenPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	msg := base64.RawURLEncoding.EncodeToString(body)

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return msg + "." + sig, nil
}

// Verify validates a token and returns the payload.
func (s *HMACSigner) Verify(token string) (*port.StreamTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}

	msg := parts[0]
	sig := parts[1]

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(msg))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, errors.New("invalid token signature")
	}

	data, err := base64.RawURLEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}

	var payload port.StreamTokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if time.Now().After(payload.Exp) {
		return nil, errors.New("token expired")
	}

	return &payload, nil
}

var _ port.StreamTokenSigner = (*HMACSigner)(nil)
