package oidc_test

import (
	"context"
	"testing"
	"time"

	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	oidcadapter "github.com/voxis/backend/internal/adapter/oidc"
)

func TestOIDCVerifier_Verify_ValidToken(t *testing.T) {
	m, err := mockoidc.Run()
	require.NoError(t, err)
	defer m.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// fetchURL and expectedIssuer are the same (normal case)
	verifier, err := oidcadapter.NewVerifier(ctx, m.Issuer(), "", m.ClientID)
	require.NoError(t, err)

	user := mockoidc.DefaultUser()
	session, err := m.SessionStore.NewSession("openid profile email", "nonce", user, "", "")
	require.NoError(t, err)

	token, err := session.IDToken(m.Config(), m.Keypair, time.Now())
	require.NoError(t, err)

	claims, err := verifier.Verify(ctx, token)
	require.NoError(t, err)

	assert.Equal(t, user.Subject, claims.Subject)
	assert.Equal(t, user.Email, claims.Email)
}

func TestOIDCVerifier_Verify_InvalidToken(t *testing.T) {
	m, err := mockoidc.Run()
	require.NoError(t, err)
	defer m.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	verifier, err := oidcadapter.NewVerifier(ctx, m.Issuer(), "", m.ClientID)
	require.NoError(t, err)

	_, err = verifier.Verify(ctx, "invalid.token.here")
	assert.Error(t, err)
}

func TestOIDCVerifier_Verify_ExpiredToken(t *testing.T) {
	m, err := mockoidc.Run()
	require.NoError(t, err)
	defer m.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	verifier, err := oidcadapter.NewVerifier(ctx, m.Issuer(), "", m.ClientID)
	require.NoError(t, err)

	user := mockoidc.DefaultUser()
	session, err := m.SessionStore.NewSession("openid", "nonce", user, "", "")
	require.NoError(t, err)

	// Generate token with a past time to make it expired
	expiredTime := time.Now().Add(-2 * time.Hour)
	token, err := session.IDToken(m.Config(), m.Keypair, expiredTime)
	require.NoError(t, err)

	_, err = verifier.Verify(ctx, token)
	assert.Error(t, err)
}

func TestNewVerifier_InvalidIssuer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := oidcadapter.NewVerifier(ctx, "http://invalid-issuer:9999", "", "client-id")
	assert.Error(t, err)
}
