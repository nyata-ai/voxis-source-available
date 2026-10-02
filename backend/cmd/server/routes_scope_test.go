package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeleteTranscriptionRequiresSummaryWriteForScopedCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	chain := []gin.HandlerFunc{func(c *gin.Context) {
		c.Set("auth_method", "api_key")
		c.Set("scopes", strings.Split(c.GetHeader("X-Test-Scopes"), ","))
	}}
	chain = append(chain, requireScopes(deleteTranscriptionScopes)...)
	chain = append(chain, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.DELETE("/transcriptions/:id", chain...)

	cases := []struct {
		scopes string
		want   int
	}{
		{"transcription:write", http.StatusForbidden},
		{"summary:write", http.StatusForbidden},
		{"transcription:write,summary:write", http.StatusNoContent},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodDelete, "/transcriptions/t-1", http.NoBody)
		req.Header.Set("X-Test-Scopes", tc.scopes)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("scopes %q: status %d, want %d", tc.scopes, rec.Code, tc.want)
		}
	}
}
