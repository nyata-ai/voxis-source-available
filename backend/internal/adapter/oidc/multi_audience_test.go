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

func TestNewVerifierWithAudiences_AcceptsMatchingAudience(t *testing.T) {
	m, err := mockoidc.Run()
	require.NoError(t, err)
	defer m.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Construct with the real client id as one of several allowed audiences.
	// Token from mockoidc has aud = m.ClientID, so the verifier should accept.
	verifier, err := oidcadapter.NewVerifierWithAudiences(ctx, m.Issuer(), "", []string{
		m.ClientID,
		"another-audience",
	})
	require.NoError(t, err)

	user := mockoidc.DefaultUser()
	session, err := m.SessionStore.NewSession("openid profile email", "nonce", user, "", "")
	require.NoError(t, err)
	token, err := session.IDToken(m.Config(), m.Keypair, time.Now())
	require.NoError(t, err)

	claims, err := verifier.Verify(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, user.Subject, claims.Subject)
}

func TestNewVerifierWithAudiences_RejectsWrongAudience(t *testing.T) {
	m, err := mockoidc.Run()
	require.NoError(t, err)
	defer m.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Construct with audiences that do NOT include m.ClientID. The mockoidc
	// token has aud = m.ClientID, so verification must fail with an aud error.
	verifier, err := oidcadapter.NewVerifierWithAudiences(ctx, m.Issuer(), "", []string{
		"only-wrong-audience",
	})
	require.NoError(t, err)

	user := mockoidc.DefaultUser()
	session, err := m.SessionStore.NewSession("openid", "nonce", user, "", "")
	require.NoError(t, err)
	token, err := session.IDToken(m.Config(), m.Keypair, time.Now())
	require.NoError(t, err)

	_, err = verifier.Verify(ctx, token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audience")
}

func TestNewVerifierWithAudiences_EmptyList_Errors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Passing an empty allowed-audiences list is almost certainly a config
	// mistake; the constructor must refuse it so the operator notices.
	_, err := oidcadapter.NewVerifierWithAudiences(ctx, "http://issuer.example", "", []string{})
	require.Error(t, err)
}
