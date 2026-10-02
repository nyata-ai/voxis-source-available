package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/handler/middleware"
)

func TestExtendReadDeadline_AllowsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ExtendReadDeadline(time.Minute))
	router.POST("/upload", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/upload", http.NoBody))
	require.Equal(t, http.StatusNoContent, recorder.Code)
}
