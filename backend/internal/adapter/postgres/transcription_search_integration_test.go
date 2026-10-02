//go:build integration

package postgres

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/db/migrations"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"os"
	"testing"
	"time"
)

const ossSearchIntegrationDatabaseURL = "OSS_TEST_DATABASE_URL"

func TestListCompletedForTranscriptSearchUsesFreshSchemaAndOrganizationKeyset(t *testing.T) {
	databaseURL := os.Getenv(ossSearchIntegrationDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s is not configured", ossSearchIntegrationDatabaseURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openFreshOSSIntegrationDatabase(t, ctx, databaseURL)
	defer pool.Close()

	applyOSSInitialSchema(t, ctx, pool)
	seedTranscriptSearchRows(t, ctx, pool)
	repository := NewTranscriptionRepository(pool)

	firstPage, err := repository.ListCompletedForTranscriptSearch(ctx, ossSearchOrgA, nil, 1)
	require.NoError(t, err)
	require.Len(t, firstPage, 1)
	require.Equal(t, ossSearchNewest, firstPage[0].ID)

	cursor := &port.TranscriptSearchCursor{CreatedAt: firstPage[0].CreatedAt, ID: firstPage[0].ID}
	secondPage, err := repository.ListCompletedForTranscriptSearch(ctx, ossSearchOrgA, cursor, 25)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Equal(t, ossSearchOldest, secondPage[0].ID)

	otherOrg, err := repository.ListCompletedForTranscriptSearch(ctx, ossSearchOrgB, nil, 25)
	require.NoError(t, err)
	require.Len(t, otherOrg, 1)
	require.Equal(t, ossSearchForeign, otherOrg[0].ID)

	assertSummaryUpdateNormalizesMissingMetadata(t, ctx, pool)
	assertFilteredRepositoriesUseFreshSchema(t, ctx, pool)
	assertSpeechmaticsCleanupSelectionUsesFreshSchema(t, ctx, pool)
	assertAdminRetentionStatsAndPreviewUseTimestamptzNow(t, ctx, pool)
}

func assertAdminRetentionStatsAndPreviewUseTimestamptzNow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	dueAt := now.Add(-7 * 24 * time.Hour)
	seedAdminRetentionRows(t, ctx, pool, now, dueAt)

	repository := NewAdminRepository(pool)
	policy := domain.LiveRecordingRetentionPolicy{Enabled: true, Days: 7, ApplyToExisting: true}
	preview, err := repository.PreviewLiveRecordingRetentionPolicy(ctx, policy, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), preview.ImmediateDeleteCount)
	require.NotNil(t, preview.OldestCompletedAt)
	require.WithinDuration(t, dueAt, *preview.OldestCompletedAt, time.Microsecond)

	stats, err := repository.GetOpsStats(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Retention.EnabledOrganizations)
	require.Equal(t, int64(1), stats.Retention.DueNow)
	require.NotNil(t, stats.Retention.OldestDueAgeSeconds)
	require.Equal(t, int64((7*24*time.Hour).Seconds()), *stats.Retention.OldestDueAgeSeconds)
}

func seedAdminRetentionRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, now, dueAt time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `UPDATE app_recording_retention_policy SET enabled = TRUE, retention_days = 7, effective_at = NULL, apply_to_existing = TRUE WHERE id = TRUE`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO organizations (id, name, slug, encryption_key_id) VALUES ('00000000-0000-0000-0000-0000000000c1', 'Retention Org', 'retention-org', 'transit/retention-org')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO media (id, organization_id, filename, content_type, size, status, storage_key, audio_deleted_at) VALUES
  ('00000000-0000-0000-0000-00000000c101', '00000000-0000-0000-0000-0000000000c1', 'due.wav', 'audio/wav', 1, 'ready', 'orgs/retention/media/due', NULL),
  ('00000000-0000-0000-0000-00000000c102', '00000000-0000-0000-0000-0000000000c1', 'young.wav', 'audio/wav', 1, 'ready', 'orgs/retention/media/young', NULL),
  ('00000000-0000-0000-0000-00000000c103', '00000000-0000-0000-0000-0000000000c1', 'deleted.wav', 'audio/wav', 1, 'deleted', 'orgs/retention/media/deleted', NULL),
  ('00000000-0000-0000-0000-00000000c104', '00000000-0000-0000-0000-0000000000c1', 'missing.wav', 'audio/wav', 1, 'ready', NULL, NULL),
  ('00000000-0000-0000-0000-00000000c105', '00000000-0000-0000-0000-0000000000c1', 'removed.wav', 'audio/wav', 1, 'ready', 'orgs/retention/media/removed', $1)`, now.Add(-24*time.Hour))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO recording_sessions (id, organization_id, user_id, status, media_id, completed_at) VALUES
  ('00000000-0000-0000-0000-00000000c201', '00000000-0000-0000-0000-0000000000c1', 'retention-due', 'completed', '00000000-0000-0000-0000-00000000c101', $1),
  ('00000000-0000-0000-0000-00000000c202', '00000000-0000-0000-0000-0000000000c1', 'retention-young', 'completed', '00000000-0000-0000-0000-00000000c102', $2),
  ('00000000-0000-0000-0000-00000000c203', '00000000-0000-0000-0000-0000000000c1', 'retention-deleted', 'completed', '00000000-0000-0000-0000-00000000c103', $3),
  ('00000000-0000-0000-0000-00000000c204', '00000000-0000-0000-0000-0000000000c1', 'retention-missing', 'completed', '00000000-0000-0000-0000-00000000c104', $3),
  ('00000000-0000-0000-0000-00000000c205', '00000000-0000-0000-0000-0000000000c1', 'retention-removed', 'completed', '00000000-0000-0000-0000-00000000c105', $3)`, dueAt, now.Add(-6*24*time.Hour), now.Add(-8*24*time.Hour))
	require.NoError(t, err)
}

func assertSpeechmaticsCleanupSelectionUsesFreshSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `UPDATE transcriptions SET status = 'pending' WHERE id = $1`, ossSearchOldest)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE transcriptions SET status = 'deleted' WHERE id = $1`, ossSearchForeign)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO transcription_segments (
    transcription_id, segment_index, start_offset_sec, end_offset_sec,
    speechmatics_job_id, status, speechmatics_deleted_at
) VALUES
    ('00000000-0000-0000-0000-00000000a101', 0, 0, 1, 'sm-parent-terminal', 'pending', NULL),
    ('00000000-0000-0000-0000-00000000a102', 0, 0, 1, 'sm-segment-terminal', 'completed', NULL),
    ('00000000-0000-0000-0000-00000000b101', 0, 0, 1, 'sm-parent-deleted', 'pending', NULL),
    ('00000000-0000-0000-0000-00000000a102', 1, 1, 2, 'sm-already-deleted', 'completed', NOW()),
    ('00000000-0000-0000-0000-00000000a102', 2, 2, 3, 'sm-active', 'pending', NULL)`)
	require.NoError(t, err)

	segments, err := NewTranscriptionRepository(pool).ListUndeletedSpeechmaticsSegments(ctx)
	require.NoError(t, err)
	require.Len(t, segments, 3)
	require.ElementsMatch(t, []string{"sm-parent-terminal", "sm-segment-terminal", "sm-parent-deleted"}, []string{
		segments[0].SpeechmaticsJobID,
		segments[1].SpeechmaticsJobID,
		segments[2].SpeechmaticsJobID,
	})
}

func assertSummaryUpdateNormalizesMissingMetadata(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	repository := NewSummaryRepository(pool)
	summary, err := domain.NewSummary(ossSearchOrgA, ossSearchNewest, domain.SummaryTypeGeneral)
	require.NoError(t, err)
	require.NoError(t, repository.Create(ctx, summary))

	summary.Status = domain.SummaryStatusCompleted
	summary.ContentEncrypted = []byte("content")
	summary.ContentNonce = []byte("nonce")
	summary.WrappedDEK = []byte("wrapped-dek")
	summary.WrappingNonce = []byte("wrapping-nonce")
	require.NoError(t, repository.Update(ctx, summary))

	fetched, err := repository.GetByID(ctx, summary.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(fetched.ModelMetadata))
}

func assertFilteredRepositoriesUseFreshSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	summaries := NewSummaryRepository(pool)
	summary, err := domain.NewSummary(ossSearchOrgA, ossSearchOldest, domain.SummaryTypeGeneral)
	require.NoError(t, err)
	require.NoError(t, summaries.Create(ctx, summary))
	summary.Status = domain.SummaryStatusCompleted
	summary.ContentEncrypted = []byte("content")
	summary.ContentNonce = []byte("nonce")
	summary.WrappedDEK = []byte("wrapped-dek")
	summary.WrappingNonce = []byte("wrapping-nonce")
	summary.ModelMetadata = []byte(`{"model":"synthetic"}`)
	require.NoError(t, summaries.Update(ctx, summary))

	listed, err := summaries.ListByOrganizationFiltered(ctx, ossSearchOrgA, port.SummaryListFilter{
		Search: "a-old",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, summary.ID, listed[0].ID)
	require.JSONEq(t, `{"model":"synthetic"}`, string(listed[0].ModelMetadata))

	otherOrg, err := summaries.ListByOrganizationFiltered(ctx, ossSearchOrgB, port.SummaryListFilter{
		Search: "a-old",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Empty(t, otherOrg)

	transcriptions := NewTranscriptionRepository(pool)
	filtered, err := transcriptions.ListByOrganizationFiltered(ctx, ossSearchOrgA, port.TranscriptionListFilter{
		Search: "a-old",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, ossSearchOldest, filtered[0].ID)
}

const (
	ossSearchOrgA    = "00000000-0000-0000-0000-00000000000a"
	ossSearchOrgB    = "00000000-0000-0000-0000-00000000000b"
	ossSearchNewest  = "00000000-0000-0000-0000-00000000a101"
	ossSearchOldest  = "00000000-0000-0000-0000-00000000a102"
	ossSearchForeign = "00000000-0000-0000-0000-00000000b101"
)

func openFreshOSSIntegrationDatabase(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	var existing *string
	err = pool.QueryRow(ctx, "SELECT to_regclass('public.organizations')").Scan(&existing)
	require.NoError(t, err)
	require.Nil(t, existing, "%s must name an empty database", ossSearchIntegrationDatabaseURL)
	return pool
}

func applyOSSInitialSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	schema, err := migrations.Files.ReadFile("000001_initial.up.sql")
	require.NoError(t, err)
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Conn().PgConn().Exec(ctx, string(schema)).ReadAll()
	require.NoError(t, err)
}

func seedTranscriptSearchRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO organizations (id, name, slug, encryption_key_id) VALUES
  ('00000000-0000-0000-0000-00000000000a', 'Org A', 'org-a', 'transit/org-a'),
  ('00000000-0000-0000-0000-00000000000b', 'Org B', 'org-b', 'transit/org-b')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO media (id, organization_id, filename, content_type, size, status) VALUES
  ('00000000-0000-0000-0000-000000001001', '00000000-0000-0000-0000-00000000000a', 'a-new.wav', 'audio/wav', 1, 'ready'),
  ('00000000-0000-0000-0000-000000001002', '00000000-0000-0000-0000-00000000000a', 'a-old.wav', 'audio/wav', 1, 'ready'),
  ('00000000-0000-0000-0000-000000002001', '00000000-0000-0000-0000-00000000000b', 'b-middle.wav', 'audio/wav', 1, 'ready')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO transcriptions (id, organization_id, media_id, status, created_at) VALUES
  ('00000000-0000-0000-0000-00000000a101', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0000-000000001001', 'completed', '2026-01-02T00:00:00Z'),
  ('00000000-0000-0000-0000-00000000a102', '00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0000-000000001002', 'completed', '2026-01-01T00:00:00Z'),
  ('00000000-0000-0000-0000-00000000b101', '00000000-0000-0000-0000-00000000000b', '00000000-0000-0000-0000-000000002001', 'completed', '2026-01-01T12:00:00Z')`)
	require.NoError(t, err)
}
