package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/handler/middleware"
)

// chunkedReader returns an io.ReadCloser that reports ContentLength=-1
// (chunked transfer encoding) and yields total bytes when read.
type chunkedReader struct {
	io.Reader
}

func (chunkedReader) Close() error { return nil }

func TestMaxBodyBytes_AllowsSmallBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.MaxBodyBytes(1 << 10)) // 1 KiB
	router.POST("/mcp", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Data(http.StatusOK, "application/json", body)
	})

	payload := bytes.Repeat([]byte("x"), 256)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, rec.Body.Bytes(), len(payload))
}

func TestMaxBodyBytes_RejectsContentLengthOversize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.MaxBodyBytes(1024))
	router.POST("/mcp", func(c *gin.Context) {
		t.Error("handler should not be invoked when ContentLength exceeds limit")
		c.Status(http.StatusOK)
	})

	payload := bytes.Repeat([]byte("x"), 4096)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "payload_too_large")
}

func TestMaxBodyBytes_RejectsChunkedOversize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.MaxBodyBytes(1024))
	router.POST("/mcp", func(c *gin.Context) {
		// Force a read; MaxBytesReader trips when limit is exceeded.
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "payload_too_large",
				"message": "request body exceeds maximum size",
			})
			return
		}
		c.Status(http.StatusOK)
	})

	payload := strings.Repeat("y", 4096)
	req := httptest.NewRequest(http.MethodPost, "/mcp", chunkedReader{Reader: strings.NewReader(payload)})
	req.ContentLength = -1 // simulate chunked encoding
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestMaxBodyBytes_GETPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.MaxBodyBytes(8))
	router.GET("/mcp", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/mcp", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMaxBodyBytes_ZeroLimitIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.MaxBodyBytes(0))
	router.POST("/mcp", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Data(http.StatusOK, "application/octet-stream", body)
	})

	payload := bytes.Repeat([]byte("z"), 8192)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
