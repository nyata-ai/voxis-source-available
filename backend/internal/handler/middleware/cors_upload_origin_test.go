package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An application at https://app.example.org can upload through
// https://upload.app.example.org. Those cross-origin requests carry an
// Authorization header and therefore trigger a CORS preflight.
//
// These tests pin the two properties that deployment depends on:
//   - the preflight is answered on the upload routes even though only POST is
//     registered there, and it is answered BEFORE authentication (a preflight
//     carries no credentials, so an auth-first order would 401 it);
//   - which origins are allowed is purely configuration
//     (CORS_ALLOWED_ORIGINS), so the application origin has to be listed
//     explicitly. Nothing is allowed by default.
const (
	appOrigin           = "https://app.example.org"
	mediaUploadPath     = "/api/v1/media/upload"
	recordingChunksPath = "/api/v1/recordings/rec-1/chunks"
)

// uploadRouter mirrors the production wiring: CORS first, then an auth
// middleware that rejects anything reaching it, then POST-only upload routes.
// authReached reports whether the auth middleware ran.
func uploadRouter(allowedOrigins []string, authReached *bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS(allowedOrigins))
	router.Use(func(c *gin.Context) {
		*authReached = true
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	router.POST(mediaUploadPath, ok)
	router.POST("/api/v1/recordings/:id/chunks", ok)
	return router
}

func preflight(t *testing.T, router *gin.Engine, origin, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, path, http.NoBody)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCORS_UploadRoutesAnswerPreflightForAppOrigin(t *testing.T) {
	t.Parallel()

	for _, path := range []string{mediaUploadPath, recordingChunksPath} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			authReached := false
			rec := preflight(t, uploadRouter([]string{appOrigin}, &authReached), appOrigin, path)

			require.Equal(t, http.StatusNoContent, rec.Code)
			assert.False(t, authReached, "preflight must be answered before authentication")
			assert.Equal(t, appOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
			assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), http.MethodPost)
			assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")
			assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
			assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
		})
	}
}

// The upload routes register POST only, so an OPTIONS request matches no
// route. Gin still runs the global middleware chain for unmatched requests,
// which is what lets the CORS layer answer the preflight — a property worth
// pinning, because losing it breaks every browser upload.
func TestCORS_PreflightSurvivesUnmatchedMethod(t *testing.T) {
	t.Parallel()

	authReached := false
	router := uploadRouter([]string{appOrigin}, &authReached)
	rec := preflight(t, router, appOrigin, "/api/v1/media/upload-unregistered-path")

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, appOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
}

// Off by default: an origin absent from CORS_ALLOWED_ORIGINS gets no
// allow-origin header, so a deployment that never lists the app origin (for
// example local development) keeps exactly today's behavior.
func TestCORS_UploadRoutesRejectUnlistedOrigin(t *testing.T) {
	t.Parallel()

	authReached := false
	router := uploadRouter([]string{"http://localhost:5173"}, &authReached)
	rec := preflight(t, router, appOrigin, mediaUploadPath)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

// The upload response is read cross-origin, so the browser must be able to
// see the headers the client relies on.
func TestCORS_ExposesResponseHeadersToUploadOrigin(t *testing.T) {
	t.Parallel()

	authReached := false
	rec := preflight(t, uploadRouter([]string{appOrigin}, &authReached), appOrigin, mediaUploadPath)

	// Preflight advertises the allow-list; the actual response carries
	// Expose-Headers. Both come from the same middleware config, so assert the
	// configured exposure survives here.
	req := httptest.NewRequest(http.MethodPost, mediaUploadPath, http.NoBody)
	req.Header.Set("Origin", appOrigin)
	actual := httptest.NewRecorder()
	uploadRouter([]string{appOrigin}, &authReached).ServeHTTP(actual, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, appOrigin, actual.Header().Get("Access-Control-Allow-Origin"))
	// Header NAMES are case-insensitive (RFC 9110); gin-contrib/cors normalizes
	// the configured "X-Request-ID" to "X-Request-Id", so compare case-folded.
	assert.Contains(t,
		strings.ToLower(actual.Header().Get("Access-Control-Expose-Headers")),
		strings.ToLower("X-Request-ID"))
}
