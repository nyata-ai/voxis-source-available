package streamtoken_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/adapter/streamtoken"
	"github.com/voxis/backend/internal/port"
)

func TestHMACSigner_SignVerify(t *testing.T) {
	signer, err := streamtoken.NewHMACSigner("super-secret-that-is-at-least-32-characters-long")
	require.NoError(t, err)

	token, err := signer.Sign(port.StreamTokenPayload{
		MediaID: "media-1",
		OrgID:   "org-1",
		Sub:     "user-1",
		Exp:     time.Now().Add(1 * time.Minute),
	})
	require.NoError(t, err)

	payload, err := signer.Verify(token)
	require.NoError(t, err)
	require.Equal(t, "media-1", payload.MediaID)
	require.Equal(t, "org-1", payload.OrgID)
	require.Equal(t, "user-1", payload.Sub)
}

func TestHMACSigner_Expired(t *testing.T) {
	signer, err := streamtoken.NewHMACSigner("super-secret-that-is-at-least-32-characters-long")
	require.NoError(t, err)

	token, err := signer.Sign(port.StreamTokenPayload{
		MediaID: "media-1",
		OrgID:   "org-1",
		Sub:     "user-1",
		Exp:     time.Now().Add(-1 * time.Minute),
	})
	require.NoError(t, err)

	_, err = signer.Verify(token)
	require.Error(t, err)
}

func TestHMACSigner_TamperedSignature(t *testing.T) {
	signer, err := streamtoken.NewHMACSigner("super-secret-that-is-at-least-32-characters-long")
	require.NoError(t, err)

	token, err := signer.Sign(port.StreamTokenPayload{
		MediaID: "media-1",
		OrgID:   "org-1",
		Sub:     "user-1",
		Exp:     time.Now().Add(1 * time.Minute),
	})
	require.NoError(t, err)

	// Tamper with signature
	_, err = signer.Verify(token + "x")
	require.Error(t, err)
}

func TestHMACSigner_ShortSecret(t *testing.T) {
	_, err := streamtoken.NewHMACSigner("")
	require.Error(t, err)

	_, err = streamtoken.NewHMACSigner("too-short")
	require.Error(t, err)
}
