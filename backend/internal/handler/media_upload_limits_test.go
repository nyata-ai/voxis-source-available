package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type stubProber struct {
	result    *port.AudioProbeResult
	err       error
	available bool
}

func (p stubProber) Probe(context.Context, string) (*port.AudioProbeResult, error) {
	return p.result, p.err
}

func (p stubProber) Available() bool { return p.available }

type countingSpool struct {
	reserveErr error
	reserved   int
	released   int
}

func (s *countingSpool) reserve() error {
	if s.reserveErr != nil {
		return s.reserveErr
	}
	s.reserved++
	return nil
}

func (s *countingSpool) release() { s.released++ }

func newLimitTestMediaHandler(prober port.AudioProber, spool uploadSpoolReservation) *MediaHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewMediaHandler(service.NewMediaService(nil, nil, nil, nil, nil, nil, logger),
		service.NewOrganizationService(nil, nil, nil, nil), prober, nil, 0, nil, nil, logger)
	h.uploadSpace = spool
	return h
}

func serveUpload(h *MediaHandler, subject string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/media/upload", func(c *gin.Context) {
		c.Set("auth_method", "api_key")
		c.Set("org_id", "org-1")
		c.Set("subject", subject)
		h.Upload(c)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/upload", http.NoBody)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	router.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body["error"]
}

func TestUserUploadSlotsCapEachUser(t *testing.T) {
	slots := newUserUploadSlots()

	for range maxConcurrentUploadsPerUser {
		require.True(t, slots.acquire("user-1"))
	}
	assert.False(t, slots.acquire("user-1"), "a user cannot exceed the per-user cap")
	assert.True(t, slots.acquire("user-2"), "other users keep their own slots")

	slots.release("user-1")
	assert.True(t, slots.acquire("user-1"), "a finished upload frees a slot")
	for range maxConcurrentUploadsPerUser {
		slots.release("user-1")
	}
	slots.release("user-2")
	assert.Empty(t, slots.active, "idle users leave no entries behind")
}

func TestUpload_PerUserLimitRejectsBeforeTakingAGlobalSlot(t *testing.T) {
	spool := &countingSpool{}
	h := newLimitTestMediaHandler(stubProber{available: true}, spool)
	for range maxConcurrentUploadsPerUser {
		require.True(t, h.userUploads.acquire("user-1"))
	}

	rec := serveUpload(h, "user-1")

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "upload_limit", errorCode(t, rec))
	assert.Equal(t, "15", rec.Header().Get("Retry-After"))
	assert.Zero(t, spool.reserved, "the global spool budget was not touched")
}

func TestUpload_ReleasesBothSlotsAfterTheRequest(t *testing.T) {
	spool := &countingSpool{}
	h := newLimitTestMediaHandler(stubProber{available: true}, spool)

	rec := serveUpload(h, "user-1") // malformed multipart body: rejected after reserving

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, spool.reserved)
	assert.Equal(t, 1, spool.released)
	assert.Empty(t, h.userUploads.active)
}

func TestUpload_GlobalCapacityFailureReturnsTheUserSlot(t *testing.T) {
	spool := &countingSpool{reserveErr: errors.New("upload concurrency limit reached (4)")}
	h := newLimitTestMediaHandler(stubProber{available: true}, spool)

	rec := serveUpload(h, "user-1")

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "temporarily_unavailable", errorCode(t, rec))
	assert.Empty(t, h.userUploads.active)
}

func TestProbeUpload_RejectsEveryFailure(t *testing.T) {
	tests := []struct {
		name   string
		prober port.AudioProber
		want   error
	}{
		{"not audio", stubProber{available: true, err: domain.ErrInvalidInput}, domain.ErrInvalidInput},
		{"timeout", stubProber{available: true, err: context.DeadlineExceeded}, errUploadProbeTimeout},
		{"parse failure", stubProber{available: true, err: errors.New("parse ffprobe output")}, errUploadProbeFailed},
		{"no result", stubProber{available: true}, errUploadProbeFailed},
		{"unavailable", stubProber{available: false}, errUploadProbeFailed},
		{"missing prober", nil, errUploadProbeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLimitTestMediaHandler(tt.prober, &countingSpool{})

			result, err := h.probeUpload(context.Background(), "/tmp/upload.tmp")

			assert.Nil(t, result)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestProbeUpload_ReturnsResult(t *testing.T) {
	want := &port.AudioProbeResult{Duration: 12.5}
	h := newLimitTestMediaHandler(stubProber{available: true, result: want}, &countingSpool{})

	result, err := h.probeUpload(context.Background(), "/tmp/upload.tmp")

	require.NoError(t, err)
	assert.Same(t, want, result)
}

func TestRespondSpoolError_ProbeFailuresAreRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newLimitTestMediaHandler(nil, &countingSpool{})
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{errUploadProbeTimeout, http.StatusBadRequest, "bad_request"},
		{errUploadProbeFailed, http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		h.respondSpoolError(c, tt.err)

		assert.Equal(t, tt.wantStatus, rec.Code)
		assert.Equal(t, tt.wantCode, errorCode(t, rec))
	}
}
