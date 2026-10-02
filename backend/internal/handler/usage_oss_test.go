package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/adapter/postgres"
	"github.com/voxis/backend/internal/handler"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type ossUsageRepositoryWithStats struct {
	*postgres.UsageRepository
}

func (r *ossUsageRepositoryWithStats) GetStats(context.Context, string) (*port.UsageStats, error) {
	return &port.UsageStats{}, nil
}

func TestOSSUsageRepositoryDisablesURLTranscriptionCounts(t *testing.T) {
	repository := postgres.NewUsageRepository(nil)
	counts, err := repository.GetURLTranscriptionCounts(context.Background(), "org", "user")
	require.NoError(t, err)
	require.Equal(t, port.URLTranscriptionCounts{}, counts)
}

func TestUsageHandlerReturnsStatsWhenOSSURLCountsAreDisabled(t *testing.T) {
	repository := &ossUsageRepositoryWithStats{UsageRepository: postgres.NewUsageRepository(nil)}
	usageHandler := handler.NewUsageHandler(
		repository,
		service.NewOrganizationService(nil, nil, nil, nil),
		0,
		0,
		nil,
	)
	router := gin.New()
	router.GET("/usage/stats", func(c *gin.Context) {
		c.Set("auth_method", "api_key")
		c.Set("org_id", "org-123")
		c.Set("subject", "user-123")
		usageHandler.GetStats(c)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage/stats", http.NoBody))
	require.Equal(t, http.StatusOK, recorder.Code)

	var stats port.UsageStats
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &stats))
	require.Zero(t, stats.URLTranscriptionsRemainingToday)
}
