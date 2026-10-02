package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/port"
)

type stubVerifier struct {
	claims *port.TokenClaims
	err    error
}

func (v stubVerifier) Verify(context.Context, string) (*port.TokenClaims, error) {
	return v.claims, v.err
}

func serveDualAuth(t *testing.T, verifier port.TokenVerifier) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	reached := false
	router := gin.New()
	router.GET("/api/v1/me", DualAuth(verifier, nil), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	req.Header.Set("Authorization", "Bearer header.payload.signature")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec, reached
}

func TestDualAuth_JWTWithUserRolePasses(t *testing.T) {
	rec, reached := serveDualAuth(t, stubVerifier{claims: &port.TokenClaims{
		Subject: "user-1", RealmRoles: []string{"offline_access", UserRole},
	}})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, reached)
}

func TestDualAuth_JWTWithoutUserRoleIsForbidden(t *testing.T) {
	for name, roles := range map[string][]string{
		"no roles":        nil,
		"admin role only": {AdminRole},
		"unrelated roles": {"offline_access", "uma_authorization"},
	} {
		t.Run(name, func(t *testing.T) {
			rec, reached := serveDualAuth(t, stubVerifier{claims: &port.TokenClaims{Subject: "user-1", RealmRoles: roles}})

			require.Equal(t, http.StatusForbidden, rec.Code)
			assert.False(t, reached)
			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "role_required", body["error"])
			assert.NotEmpty(t, body["message"])
		})
	}
}

func TestDualAuth_InvalidJWTIsUnauthorizedBeforeRoleCheck(t *testing.T) {
	rec, reached := serveDualAuth(t, stubVerifier{err: errors.New("bad signature")})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, reached)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unauthorized", body["error"])
}
