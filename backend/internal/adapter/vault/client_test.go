package vault_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/adapter/vault"
)

func TestNewClient_MissingAddress(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{
		Token: "some-token",
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "address")
}

func TestNewClient_TokenAuth(t *testing.T) {
	t.Parallel()

	client, err := vault.NewClient(vault.Config{
		Address: "https://vault.example.com:8200",
		Token:   "test-token",
	}, nil)

	require.NoError(t, err)
	assert.NotNil(t, client)
	require.NoError(t, client.Close())
}

func TestNewClient_AppRoleAuth(t *testing.T) {
	t.Parallel()

	// Config with RoleID+SecretID (no Token) should NOT error on construction.
	// The actual AppRole login will fail (no real Vault), but NewClient should
	// accept this as a valid auth configuration.
	_, err := vault.NewClient(vault.Config{
		Address:  "https://vault.example.com:8200",
		RoleID:   "test-role-id",
		SecretID: "test-secret-id",
	}, nil)

	// NewClient with AppRole will attempt login which fails against a fake address.
	// That's expected — the key assertion is it doesn't fail with "no auth method".
	if err != nil {
		assert.NotContains(t, err.Error(), "no auth method")
		assert.NotContains(t, err.Error(), "token is required")
	}
}

func TestNewClient_NeitherTokenNorAppRole(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{
		Address: "https://vault.example.com:8200",
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth method")
}

func TestNewClient_PartialAppRole_MissingSecretID(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{
		Address: "https://vault.example.com:8200",
		RoleID:  "test-role-id",
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret_id")
}

func TestNewClient_PartialAppRole_MissingRoleID(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{
		Address:  "https://vault.example.com:8200",
		SecretID: "test-secret-id",
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "role_id")
}

func TestNewClient_AmbiguousAuth(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{
		Address:  "https://vault.example.com:8200",
		Token:    "test-token",
		RoleID:   "test-role-id",
		SecretID: "test-secret-id",
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestNewClient_AmbiguousAuth_TokenAndPartialAppRole(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{
		Address: "https://vault.example.com:8200",
		Token:   "test-token",
		RoleID:  "test-role-id",
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestNewClient_CustomMountPath(t *testing.T) {
	t.Parallel()

	client, err := vault.NewClient(vault.Config{
		Address:   "https://vault.example.com:8200",
		Token:     "test-token",
		MountPath: "custom-transit",
	}, nil)

	require.NoError(t, err)
	assert.NotNil(t, client)
	require.NoError(t, client.Close())
}

func TestNewClient_BothEmpty(t *testing.T) {
	t.Parallel()

	_, err := vault.NewClient(vault.Config{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address")
}

func TestClient_Close_StopsRenewal(t *testing.T) {
	t.Parallel()

	// Close on a token-based client should not panic or leak.
	client, err := vault.NewClient(vault.Config{
		Address: "https://vault.example.com:8200",
		Token:   "test-token",
	}, nil)
	require.NoError(t, err)

	// Close should be idempotent and safe.
	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
}

func TestGetKEK_UsesConfiguredCacheTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ttl       time.Duration
		wantReads int32
	}{
		{name: "long TTL serves the second read from cache", ttl: time.Hour, wantReads: 1},
		{name: "short TTL re-exports after expiry", ttl: time.Nanosecond, wantReads: 2},
		{name: "zero TTL falls back to the default", ttl: 0, wantReads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/transit/export/encryption-key/org-org-1" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				reads.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"keys": map[string]string{"1": base64.StdEncoding.EncodeToString(make([]byte, 32))},
				}})
			}))
			t.Cleanup(server.Close)
			client, err := vault.NewClient(vault.Config{Address: server.URL, Token: "test-token", CacheTTL: tt.ttl}, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })

			for range 2 {
				key, getErr := client.GetKEK(context.Background(), "org-1")
				require.NoError(t, getErr)
				require.Len(t, key, 32)
			}
			assert.Equal(t, tt.wantReads, reads.Load())
		})
	}
}
