package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/handler/middleware"
)

func TestConcurrencyLimiter_UserMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := middleware.NewConcurrencyLimiter(1)
	entered := make(chan struct{})
	release := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("subject", c.GetHeader("X-Test-Subject"))
		c.Next()
	})
	router.POST("/upload", limiter.UserMiddleware(), func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/upload", http.NoBody)
	firstReq.Header.Set("X-Test-Subject", "user-1")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		router.ServeHTTP(first, firstReq)
	}()
	<-entered

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/upload", http.NoBody)
	secondReq.Header.Set("X-Test-Subject", "user-1")
	router.ServeHTTP(second, secondReq)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Equal(t, "1", second.Header().Get("Retry-After"))

	close(release)
	wg.Wait()
	require.Equal(t, http.StatusNoContent, first.Code)

	third := httptest.NewRecorder()
	thirdReq := httptest.NewRequest(http.MethodPost, "/upload", http.NoBody)
	thirdReq.Header.Set("X-Test-Subject", "user-1")
	// Use a separate router whose handler cannot block; this proves release.
	checkRouter := gin.New()
	checkRouter.Use(func(c *gin.Context) {
		c.Set("subject", "user-1")
		c.Next()
	})
	checkRouter.POST("/upload", limiter.UserMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	checkRouter.ServeHTTP(third, thirdReq)
	require.Equal(t, http.StatusNoContent, third.Code)
}

func TestConcurrencyLimiter_DifferentUsersAreIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := middleware.NewConcurrencyLimiter(1)
	entered := make(chan struct{})
	release := make(chan struct{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("subject", c.GetHeader("X-Test-Subject"))
		c.Next()
	})
	router.POST("/", limiter.UserMiddleware(), func(c *gin.Context) {
		if c.GetString("subject") == "user-1" {
			close(entered)
			<-release
		}
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	firstReq.Header.Set("X-Test-Subject", "user-1")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		router.ServeHTTP(first, firstReq)
	}()
	<-entered

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	secondReq.Header.Set("X-Test-Subject", "user-2")
	router.ServeHTTP(second, secondReq)
	require.Equal(t, http.StatusNoContent, second.Code)

	close(release)
	wg.Wait()
}
