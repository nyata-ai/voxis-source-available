package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
)

func TestSpoolErrorMapsMediaTooLongTo422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &MediaHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	h.respondSpoolError(c, fmt.Errorf("audio is 9h0m0s long; the maximum is 8h0m0s: %w", domain.ErrMediaTooLong))

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "media_too_long" || body["message"] != "audio is 9h0m0s long; the maximum is 8h0m0s" {
		t.Fatalf("body = %v", body)
	}
}
