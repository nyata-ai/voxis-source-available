//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/db/migrations"
	"github.com/voxis/backend/internal/domain"
)

const (
	ptOrg        = "00000000-0000-0000-0000-0000000000d1"
	ptUser       = "purge-subject"
	ptMedia      = "00000000-0000-0000-0000-00000000d101"
	ptKeptMedia  = "00000000-0000-0000-0000-00000000d102"
	ptTrans      = "00000000-0000-0000-0000-00000000d201"
	ptKeptTrans  = "00000000-0000-0000-0000-00000000d202"
	ptSegment    = "00000000-0000-0000-0000-00000000d301"
	ptSession    = "00000000-0000-0000-0000-00000000d401"
	ptCollection = "00000000-0000-0000-0000-00000000d501"
)

// TestMediaPurgeRemovesCiphertextAndMetadata proves a media delete leaves no
// ciphertext, name, hash, or key reference behind, keeps the provider job id
// for the Speechmatics cleaner, blocks late segment writes, and leaves other
// media untouched.
func TestMediaPurgeRemovesCiphertextAndMetadata(t *testing.T) {
	ctx, pool := purgeTestDatabase(t, "000001_initial.up.sql", "000002_purge_deleted_content.up.sql")
	seedPurgeRows(t, ctx, pool)

	require.NoError(t, NewMediaRepository(pool).CascadeDelete(ctx, ptMedia))

	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcriptions WHERE media_id = $1
 AND (content_encrypted IS NOT NULL OR content_nonce IS NOT NULL OR wrapped_dek IS NOT NULL OR wrapping_nonce IS NOT NULL OR status <> 'deleted')`, ptMedia)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcription_segments WHERE transcription_id = $1
 AND (content_encrypted IS NOT NULL OR wrapped_dek IS NOT NULL OR status = 'submitted')`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM summaries WHERE transcription_id = $1 AND (
 content_encrypted IS NOT NULL OR wrapped_dek IS NOT NULL OR review_findings_encrypted IS NOT NULL OR review_wrapped_dek IS NOT NULL
 OR extraction_encrypted IS NOT NULL OR extraction_wrapped_dek IS NOT NULL OR structured_content_ciphertext IS NOT NULL
 OR structured_content_wrapped_dek IS NOT NULL OR source_hash IS NOT NULL OR status <> 'deleted')`, ptTrans)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM media WHERE id = $1 AND status = 'deleted' AND filename = ''
 AND title IS NULL AND description IS NULL AND file_hash IS NULL AND audio_metadata IS NULL
 AND storage_key IS NULL AND wrapped_dek IS NULL AND wrapping_nonce IS NULL`, ptMedia)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM mcp_collection_items WHERE transcription_id = $1`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM recording_chunks WHERE session_id = $1`, ptSession)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM recording_sessions WHERE id = $1 AND microphone_label IS NULL`, ptSession)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM transcriptions WHERE id = $1 AND speechmatics_job_id = 'sm-purged'`, ptTrans)

	// Untouched neighbour.
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM media WHERE id = $1 AND filename = 'kept.wav' AND wrapped_dek IS NOT NULL`, ptKeptMedia)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM transcriptions WHERE id = $1 AND content_encrypted IS NOT NULL`, ptKeptTrans)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM mcp_collection_items WHERE transcription_id = $1`, ptKeptTrans)

	// A late provider result cannot put ciphertext back.
	repo := NewTranscriptionRepository(pool)
	late := &domain.TranscriptionSegment{ID: ptSegment, Status: domain.TranscriptionSegmentStatusCompleted,
		ContentEncrypted: []byte("late"), ContentNonce: []byte("n"), WrappedDEK: []byte("k"), WrappingNonce: []byte("w")}
	require.ErrorIs(t, repo.UpdateSegment(ctx, late), domain.ErrNotFound)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcription_segments WHERE id = $1 AND content_encrypted IS NOT NULL`, ptSegment)

	// Deleting again is idempotent.
	require.NoError(t, NewMediaRepository(pool).CascadeDelete(ctx, ptMedia))
	require.ErrorIs(t, NewMediaRepository(pool).CascadeDelete(ctx, "00000000-0000-0000-0000-0000000000ff"), domain.ErrNotFound)
}

func TestTranscriptionPurgeRemovesCiphertext(t *testing.T) {
	ctx, pool := purgeTestDatabase(t, "000001_initial.up.sql", "000002_purge_deleted_content.up.sql")
	seedPurgeRows(t, ctx, pool)

	require.NoError(t, NewTranscriptionRepository(pool).CascadeDelete(ctx, ptTrans))

	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM transcriptions WHERE id = $1 AND status = 'deleted' AND content_encrypted IS NULL AND wrapped_dek IS NULL`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM summaries WHERE transcription_id = $1 AND (content_encrypted IS NOT NULL OR structured_content_ciphertext IS NOT NULL)`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcription_segments WHERE transcription_id = $1 AND content_encrypted IS NOT NULL`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM mcp_collection_items WHERE transcription_id = $1`, ptTrans)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM transcriptions WHERE id = $1 AND content_encrypted IS NOT NULL`, ptKeptTrans)
}

// TestMigration000002PurgesRowsDeletedBeforeUpgrade seeds soft-deleted rows
// the way older releases left them, then applies 000002.
func TestMigration000002PurgesRowsDeletedBeforeUpgrade(t *testing.T) {
	ctx, pool := purgeTestDatabase(t, "000001_initial.up.sql")
	seedPurgeRows(t, ctx, pool)
	_, err := pool.Exec(ctx, `UPDATE media SET status = 'deleted' WHERE id = $1;`, ptMedia)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE transcriptions SET status = 'deleted' WHERE id = $1`, ptTrans)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE summaries SET endpoint_location = 'http://llama:8080/v1/chat/completions'`)
	require.NoError(t, err)

	applyMigrationFiles(t, ctx, pool, "000002_purge_deleted_content.up.sql")

	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcriptions WHERE id = $1 AND content_encrypted IS NOT NULL`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM summaries WHERE transcription_id = $1 AND content_encrypted IS NOT NULL`, ptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcription_segments WHERE transcription_id = $1 AND content_encrypted IS NOT NULL`, ptTrans)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM media WHERE id = $1 AND filename = '' AND title IS NULL AND file_hash IS NULL`, ptMedia)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM transcriptions WHERE id = $1 AND content_encrypted IS NOT NULL`, ptKeptTrans)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM summaries WHERE endpoint_location <> 'local-gemma'`)
}

func TestDataPurgeRepositoryDeletesOrganizationRows(t *testing.T) {
	ctx, pool := purgeTestDatabase(t, "000001_initial.up.sql", "000002_purge_deleted_content.up.sql")
	seedPurgeRows(t, ctx, pool)
	repo := NewDataPurgeRepository(pool)

	org, err := repo.GetUserOrganization(ctx, ptUser)
	require.NoError(t, err)
	require.Equal(t, ptOrg, org.OrganizationID)
	require.Equal(t, int64(1), org.MemberCount)
	counts, err := repo.CountOrganization(ctx, ptOrg)
	require.NoError(t, err)
	require.Equal(t, int64(2), counts.Media)
	require.Equal(t, int64(1), counts.APIKeys)
	require.Equal(t, int64(2), counts.PendingProviderDeletions)
	firstPage, err := repo.ListMediaIDsAfter(ctx, ptOrg, "", 1)
	require.NoError(t, err)
	require.Equal(t, []string{ptMedia}, firstPage)
	secondPage, err := repo.ListMediaIDsAfter(ctx, ptOrg, ptMedia, 1)
	require.NoError(t, err)
	require.Equal(t, []string{ptKeptMedia}, secondPage)

	require.NoError(t, repo.DeleteAPIKeys(ctx, ptOrg))
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM api_keys WHERE organization_id = $1`, ptOrg)
	requireCount(t, ctx, pool, 2, `SELECT COUNT(*) FROM media WHERE organization_id = $1`, ptOrg)

	require.NoError(t, repo.DeleteOrganizationData(ctx, ptOrg, false))
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM api_keys WHERE organization_id = $1`, ptOrg)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM mcp_collections WHERE organization_id = $1`, ptOrg)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM recording_sessions WHERE organization_id = $1`, ptOrg)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM users WHERE id = $1 AND email = '' AND name = ''`, ptUser)
	requireCount(t, ctx, pool, 1, `SELECT COUNT(*) FROM organizations WHERE id = $1 AND name = 'Purged workspace'`, ptOrg)

	require.NoError(t, repo.DeleteOrganizationData(ctx, ptOrg, true))
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM organizations WHERE id = $1`, ptOrg)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM users WHERE id = $1`, ptUser)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM media WHERE organization_id = $1`, ptOrg)
	requireCount(t, ctx, pool, 0, `SELECT COUNT(*) FROM transcriptions WHERE organization_id = $1`, ptOrg)
}

func requireCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64, query string, args ...any) {
	t.Helper()
	var got int64
	require.NoError(t, pool.QueryRow(ctx, query, args...).Scan(&got))
	require.Equal(t, want, got, query)
}

// purgeTestDatabase creates a private database next to OSS_TEST_DATABASE_URL
// (which must allow CREATE DATABASE) and applies the named migrations.
func purgeTestDatabase(t *testing.T, files ...string) (context.Context, *pgxpool.Pool) {
	t.Helper()
	baseURL := os.Getenv(ossSearchIntegrationDatabaseURL)
	if baseURL == "" {
		t.Skipf("%s is not configured", ossSearchIntegrationDatabaseURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, baseURL)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	name := fmt.Sprintf("purge_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	parsed, err := url.Parse(baseURL)
	require.NoError(t, err)
	parsed.Path = "/" + name
	pool, err := pgxpool.New(ctx, parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name)
	})
	applyMigrationFiles(t, ctx, pool, files...)
	return ctx, pool
}

func applyMigrationFiles(t *testing.T, ctx context.Context, pool *pgxpool.Pool, files ...string) {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	for _, file := range files {
		sql, readErr := migrations.Files.ReadFile(file)
		require.NoError(t, readErr)
		_, err = conn.Conn().PgConn().Exec(ctx, string(sql)).ReadAll()
		require.NoError(t, err, file)
	}
}

func seedPurgeRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`INSERT INTO organizations (id, name, slug) VALUES ('` + ptOrg + `', 'ada''s Workspace', 'ada-s-workspace')`,
		`INSERT INTO users (id, organization_id, email, name) VALUES ('` + ptUser + `', '` + ptOrg + `', 'ada@example.org', 'Ada')`,
		`INSERT INTO api_keys (organization_id, created_by, name, key_prefix, key_hash) VALUES ('` + ptOrg + `', '` + ptUser + `', 'k', 'vxs_live_p', 'h')`,
		`INSERT INTO media (id, organization_id, filename, content_type, size, status, title, description, file_hash,
  audio_metadata, storage_key, wrapped_dek, wrapping_nonce, encryption_algo) VALUES
  ('` + ptMedia + `', '` + ptOrg + `', 'counselling-session-ada.wav', 'audio/wav', 2048, 'ready', 'Ada counselling', 'private notes',
   'abc123', 'meta', 'orgs/d1/media/d101/encrypted.bin', 'dek', 'nonce', 'aes-256-gcm-chunked'),
  ('` + ptKeptMedia + `', '` + ptOrg + `', 'kept.wav', 'audio/wav', 2048, 'ready', NULL, NULL, NULL, NULL,
   'orgs/d1/media/d102/encrypted.bin', 'dek', 'nonce', 'aes-256-gcm-chunked')`,
		`INSERT INTO transcriptions (id, organization_id, media_id, status, speechmatics_job_id, content_encrypted, content_nonce, wrapped_dek, wrapping_nonce) VALUES
  ('` + ptTrans + `', '` + ptOrg + `', '` + ptMedia + `', 'completed', 'sm-purged', 'ct', 'n', 'k', 'w'),
  ('` + ptKeptTrans + `', '` + ptOrg + `', '` + ptKeptMedia + `', 'completed', 'sm-kept', 'ct', 'n', 'k', 'w')`,
		`INSERT INTO transcription_segments (id, transcription_id, segment_index, start_offset_sec, end_offset_sec, status, content_encrypted, content_nonce, wrapped_dek, wrapping_nonce) VALUES
  ('` + ptSegment + `', '` + ptTrans + `', 0, 0, 1, 'submitted', 'ct', 'n', 'k', 'w')`,
		`INSERT INTO summaries (organization_id, transcription_id, summary_type, status, content_encrypted, content_nonce, wrapped_dek, wrapping_nonce,
  review_findings_encrypted, review_wrapped_dek, extraction_encrypted, extraction_wrapped_dek,
  structured_content_ciphertext, structured_content_wrapped_dek, source_hash, endpoint_location) VALUES
  ('` + ptOrg + `', '` + ptTrans + `', 'general', 'completed', 'ct', 'n', 'k', 'w', 'r', 'rk', 'e', 'ek', 's', 'sk', 'hash', 'local-gemma'),
  ('` + ptOrg + `', '` + ptKeptTrans + `', 'general', 'completed', 'ct', 'n', 'k', 'w', NULL, NULL, NULL, NULL, NULL, NULL, 'hash', 'local-gemma')`,
		`INSERT INTO mcp_collections (id, organization_id, name) VALUES ('` + ptCollection + `', '` + ptOrg + `', 'Case files')`,
		`INSERT INTO mcp_collection_items (collection_id, transcription_id) VALUES ('` + ptCollection + `', '` + ptTrans + `'), ('` + ptCollection + `', '` + ptKeptTrans + `')`,
		`INSERT INTO recording_sessions (id, organization_id, user_id, status, media_id, microphone_label) VALUES
  ('` + ptSession + `', '` + ptOrg + `', '` + ptUser + `', 'completed', '` + ptMedia + `', 'Ada''s headset')`,
		`INSERT INTO recording_chunks (session_id, seq, storage_key, wrapped_dek, wrapping_nonce, chunk_size, chunk_count, plaintext_size, checksum) VALUES
  ('` + ptSession + `', 0, 'orgs/d1/recordings/d401/chunk-000.bin.enc', 'dek', 'nonce', 1, 1, 1, 'sum')`,
	}
	for _, statement := range statements {
		_, err := pool.Exec(ctx, statement)
		require.NoError(t, err, statement)
	}
}
