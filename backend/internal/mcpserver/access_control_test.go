package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

func scopedContext(authMethod, subject string, scopes ...string) context.Context {
	ctx := context.WithValue(context.Background(), middleware.CtxKeyOrgID, "org-1")
	ctx = context.WithValue(ctx, middleware.CtxKeyAuthMethod, authMethod)
	ctx = context.WithValue(ctx, middleware.CtxKeySubject, subject)
	return context.WithValue(ctx, middleware.CtxKeyScopeList, scopes)
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

type detailTransRepo struct {
	port.TranscriptionRepository
}

func (detailTransRepo) GetByID(context.Context, string) (*domain.Transcription, error) {
	return &domain.Transcription{ID: "trans-1", OrganizationID: "org-1", MediaID: "media-1",
		Status: domain.TranscriptionStatusPending, CreatedAt: time.Now()}, nil
}

type detailMediaRepo struct {
	port.MediaRepository
}

func (detailMediaRepo) GetByID(context.Context, string) (*domain.Media, error) {
	return nil, domain.ErrNotFound
}

type detailSummaryRepo struct {
	port.SummaryRepository
}

func (detailSummaryRepo) ListByTranscription(context.Context, string) ([]*domain.Summary, error) {
	return []*domain.Summary{{ID: "summary-1", SummaryType: "general", Status: "completed", WordCount: 12}}, nil
}

func detailDeps() Dependencies {
	return Dependencies{TranscriptionService: service.NewTranscriptionService(
		detailTransRepo{}, detailMediaRepo{}, detailSummaryRepo{}, nil, nil)}
}

func detailSummaries(ctx context.Context, t *testing.T) []any {
	t.Helper()
	result, _, err := handleGetTranscriptionDetail(ctx, detailDeps(), GetTranscriptionDetailInput{TranscriptionID: "trans-1"})
	require.NoError(t, err)
	require.False(t, result.IsError, resultText(t, result))
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultText(t, result)), &body))
	summaries, _ := body["summaries"].([]any)
	return summaries
}

func TestGetTranscriptionDetail_OmitsSummariesWithoutSummaryRead(t *testing.T) {
	for _, method := range []string{"jwt", "api_key"} {
		t.Run(method, func(t *testing.T) {
			assert.Empty(t, detailSummaries(scopedContext(method, "user-1", "transcription:read"), t))
			assert.Len(t, detailSummaries(scopedContext(method, "user-1", "transcription:read", "summary:read"), t), 1)
		})
	}
}

func TestDeleteMedia_RequiresTranscriptionAndSummaryWrite(t *testing.T) {
	for _, scopes := range [][]string{
		{"media:write"},
		{"media:write", "transcription:write"},
		{"media:write", "summary:write"},
	} {
		// MediaService is nil: reaching the delete would panic, so a pass
		// proves the scope check refused first.
		result, _, err := handleDeleteMedia(scopedContext("jwt", "user-1", scopes...), Dependencies{}, DeleteMediaInput{MediaID: "media-1"})
		require.NoError(t, err)
		require.True(t, result.IsError, "scopes %v", scopes)
		assert.Contains(t, resultText(t, result), "forbidden")
	}
	assert.ElementsMatch(t, []string{"media:write", "transcription:write", "summary:write"}, buildMCPToolCatalog()["delete_media"].RequiredScopes)
}

type countingJobInserter struct{ calls int }

func (c *countingJobInserter) InsertTranscribeJob(context.Context, string, string, string) error {
	c.calls++
	return nil
}

func TestCreateTranscription_IsRateLimitedPerUserBeforeCreating(t *testing.T) {
	limiter := middleware.NewRateLimiter(0.0001, 1)
	require.True(t, limiter.AllowUser(middleware.TranscriptionCreateLimitKey, "user-1"), "spend the user's only token (e.g. via REST)")
	inserter := &countingJobInserter{}
	// TranscriptionService is nil: reaching Create would panic.
	deps := Dependencies{JobInserter: inserter, TranscriptionCreateLimiter: limiter}

	result, _, err := handleCreateTranscription(scopedContext("api_key", "user-1", "transcription:write"), deps, CreateTranscriptionInput{MediaID: "media-1"})

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "rate_limited")
	assert.Zero(t, inserter.calls)
}

func TestCreateTranscription_RefusesRequestWithoutSubjectWhenLimited(t *testing.T) {
	deps := Dependencies{JobInserter: &countingJobInserter{}, TranscriptionCreateLimiter: middleware.NewRateLimiter(1, 10)}

	result, _, err := handleCreateTranscription(scopedContext("api_key", "", "transcription:write"), deps, CreateTranscriptionInput{MediaID: "media-1"})

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "rate_limited")
}
