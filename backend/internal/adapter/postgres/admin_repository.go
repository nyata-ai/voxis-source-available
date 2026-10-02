package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// AdminRepository stores the operational settings and statistics retained by
// Voxis-OSS.
type AdminRepository struct{ pool *pgxpool.Pool }

// NewAdminRepository creates an operations repository backed by pool.
func NewAdminRepository(pool *pgxpool.Pool) *AdminRepository {
	if pool == nil {
		panic("postgres: admin repository pool must not be nil")
	}
	return &AdminRepository{pool: pool}
}

// GetOpsStats returns a consistent operations snapshot at now.
func (r *AdminRepository) GetOpsStats(ctx context.Context, now time.Time) (*port.AdminOpsStats, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin ops stats: %w", err)
	}
	defer rollbackAfterTransaction(ctx, tx, "load OSS operations statistics")
	stats := &port.AdminOpsStats{GeneratedAt: now.UTC(), Media: newEntityStats(), Scan: newEntityStats(), Transcriptions: port.AdminOpsTranscriptionStats{ByStatus: map[string]int64{}}, Summaries: port.AdminOpsSummaryStats{ByStatus: map[string]int64{}}, Recordings: newEntityStats()}
	if err := loadEntityStats(ctx, tx, &stats.Media, "SELECT status, COUNT(*)::bigint FROM media GROUP BY status"); err != nil {
		return nil, err
	}
	if err := loadEntityStats(ctx, tx, &stats.Scan, "SELECT scan_status, COUNT(*)::bigint FROM media GROUP BY scan_status"); err != nil {
		return nil, err
	}
	if err := loadTranscriptionStats(ctx, tx, &stats.Transcriptions, now); err != nil {
		return nil, err
	}
	if err := loadSummaryStats(ctx, tx, &stats.Summaries, now); err != nil {
		return nil, err
	}
	if err := loadEntityStats(ctx, tx, &stats.Recordings, "SELECT status, COUNT(*)::bigint FROM recording_sessions GROUP BY status"); err != nil {
		return nil, err
	}
	if err := loadProviderStats(ctx, tx, &stats.Providers, now); err != nil {
		return nil, err
	}
	if err := loadRetentionStats(ctx, tx, &stats.Retention, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit ops stats: %w", err)
	}
	return stats, nil
}

func rollbackAfterTransaction(ctx context.Context, tx pgx.Tx, operation string) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("rollback PostgreSQL transaction", "operation", operation, "error", err)
	}
}

func newEntityStats() port.AdminOpsEntityStats {
	return port.AdminOpsEntityStats{ByStatus: map[string]int64{}}
}

type adminQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadEntityStats(ctx context.Context, q adminQuerier, out *port.AdminOpsEntityStats, sql string) error {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		out.ByStatus[status] = count
		out.Total += count
	}
	return rows.Err()
}

func loadTranscriptionStats(ctx context.Context, q adminQuerier, out *port.AdminOpsTranscriptionStats, now time.Time) error {
	rows, err := q.Query(ctx, "SELECT status, COUNT(*)::bigint FROM transcriptions GROUP BY status")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		out.ByStatus[status] = count
		out.Total += count
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return q.QueryRow(ctx, "SELECT COUNT(*)::bigint FROM transcriptions WHERE status = 'submitted' AND updated_at < $1", now.UTC().Add(-2*time.Hour)).Scan(&out.StaleSubmittedOver2Hrs)
}

func loadSummaryStats(ctx context.Context, q adminQuerier, out *port.AdminOpsSummaryStats, now time.Time) error {
	rows, err := q.Query(ctx, "SELECT status, COUNT(*)::bigint FROM summaries GROUP BY status")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		out.ByStatus[status] = count
		out.Total += count
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return q.QueryRow(ctx, "SELECT COUNT(*)::bigint FROM summaries WHERE status = 'pending' AND updated_at < $1", now.UTC().Add(-30*time.Minute)).Scan(&out.StalePendingOver30Min)
}

func loadProviderStats(ctx context.Context, q adminQuerier, out *port.AdminProviderUsageStats, now time.Time) error {
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE speechmatics_job_id IS NOT NULL AND speechmatics_job_id != '')::bigint, COUNT(*) FILTER (WHERE speechmatics_job_id IS NOT NULL AND speechmatics_job_id != '' AND status = 'completed')::bigint, COUNT(*) FILTER (WHERE speechmatics_job_id IS NOT NULL AND speechmatics_job_id != '' AND status = 'failed')::bigint, COUNT(*) FILTER (WHERE speechmatics_job_id IS NOT NULL AND speechmatics_job_id != '' AND status = 'submitted' AND updated_at < $1)::bigint, COALESCE(SUM(duration_seconds) FILTER (WHERE speechmatics_job_id IS NOT NULL AND speechmatics_job_id != '' AND status = 'completed'), 0)::double precision FROM (SELECT status, speechmatics_job_id, duration_seconds, updated_at FROM transcriptions UNION ALL SELECT status, speechmatics_job_id, duration_seconds, updated_at FROM transcription_segments) jobs`, now.UTC().Add(-2*time.Hour)).Scan(&out.Speechmatics.SubmittedJobs, &out.Speechmatics.CompletedJobs, &out.Speechmatics.FailedJobs, &out.Speechmatics.StaleSubmittedOver2Hrs, &out.Speechmatics.ProcessedAudioSeconds); err != nil {
		return err
	}
	if err := q.QueryRow(ctx, "SELECT COUNT(*) FILTER (WHERE status = 'completed')::bigint, COALESCE(bool_and(COALESCE((model_metadata->>'usage_available')::boolean, FALSE)) FILTER (WHERE status = 'completed'), FALSE), COALESCE(SUM(prompt_tokens) FILTER (WHERE status = 'completed'),0)::bigint, COALESCE(SUM(completion_tokens) FILTER (WHERE status = 'completed'),0)::bigint, COALESCE(SUM(thinking_tokens) FILTER (WHERE status = 'completed'),0)::bigint FROM summaries").Scan(&out.Gemma.CompletedSummaries, &out.Gemma.UsageAvailable, &out.Gemma.PromptTokens, &out.Gemma.CompletionTokens, &out.Gemma.ThinkingTokens); err != nil {
		return err
	}
	out.Gemma.TotalTokens = out.Gemma.PromptTokens + out.Gemma.CompletionTokens + out.Gemma.ThinkingTokens
	return nil
}

func loadRetentionStats(ctx context.Context, q adminQuerier, out *port.AdminRetentionStats, now time.Time) error {
	var oldest pgtype.Timestamptz
	err := q.QueryRow(ctx, `SELECT CASE WHEN (SELECT enabled FROM app_recording_retention_policy WHERE id = TRUE) THEN 1::bigint ELSE 0::bigint END, COUNT(*)::bigint, MIN(rs.completed_at)::timestamptz, (SELECT COUNT(*)::bigint FROM media WHERE audio_deleted_at IS NOT NULL) FROM recording_sessions rs JOIN media m ON m.id = rs.media_id JOIN app_recording_retention_policy p ON p.id = TRUE WHERE p.enabled AND rs.status = 'completed' AND rs.completed_at IS NOT NULL AND rs.completed_at <= $1::timestamptz - (p.retention_days::int * INTERVAL '1 day') AND (p.apply_to_existing OR (p.effective_at IS NOT NULL AND rs.completed_at >= p.effective_at)) AND m.status != 'deleted' AND m.storage_key IS NOT NULL AND m.audio_deleted_at IS NULL`, now.UTC()).Scan(&out.EnabledOrganizations, &out.DueNow, &oldest, &out.AudioDeleted)
	if err != nil {
		return err
	}
	if oldest.Valid {
		age := int64(now.UTC().Sub(oldest.Time).Seconds())
		if age < 0 {
			age = 0
		}
		out.OldestDueAgeSeconds = &age
	}
	return nil
}

// GetLiveRecordingRetentionPolicy returns the configured standard-recording retention policy.
func (r *AdminRepository) GetLiveRecordingRetentionPolicy(ctx context.Context) (domain.LiveRecordingRetentionPolicy, error) {
	var out domain.LiveRecordingRetentionPolicy
	var effective, updated pgtype.Timestamptz
	var by pgtype.Text
	err := r.pool.QueryRow(ctx, "SELECT enabled, retention_days, effective_at, apply_to_existing, updated_at, updated_by FROM app_recording_retention_policy WHERE id = TRUE").Scan(&out.Enabled, &out.Days, &effective, &out.ApplyToExisting, &updated, &by)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	out.EffectiveAt = timeFromPg(effective)
	out.UpdatedAt = timeFromPg(updated)
	if by.Valid {
		out.UpdatedBy = by.String
	}
	return out, nil
}

// PreviewLiveRecordingRetentionPolicy returns the recordings a policy would affect.
func (r *AdminRepository) PreviewLiveRecordingRetentionPolicy(ctx context.Context, policy domain.LiveRecordingRetentionPolicy, now time.Time) (domain.LiveRecordingRetentionPreview, error) {
	var out domain.LiveRecordingRetentionPreview
	var oldest pgtype.Timestamptz
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::bigint, MIN(rs.completed_at)::timestamptz FROM recording_sessions rs JOIN media m ON m.id = rs.media_id WHERE $1 AND rs.status = 'completed' AND rs.completed_at IS NOT NULL AND rs.completed_at <= $2::timestamptz - ($3::int * INTERVAL '1 day') AND ($4 OR ($5::timestamptz IS NOT NULL AND rs.completed_at >= $5)) AND m.status != 'deleted' AND m.storage_key IS NOT NULL AND m.audio_deleted_at IS NULL`, policy.Enabled, now.UTC(), policy.Days, policy.ApplyToExisting, timestamptzFromPtr(policy.EffectiveAt)).Scan(&out.ImmediateDeleteCount, &oldest)
	if err != nil {
		return out, err
	}
	out.OldestCompletedAt = timeFromPg(oldest)
	return out, nil
}

// UpdateLiveRecordingRetentionPolicy persists an authorized policy change.
func (r *AdminRepository) UpdateLiveRecordingRetentionPolicy(ctx context.Context, policy domain.LiveRecordingRetentionPolicy, updatedBy string) (domain.LiveRecordingRetentionPolicy, error) {
	var out domain.LiveRecordingRetentionPolicy
	var effective, updated pgtype.Timestamptz
	var by pgtype.Text
	err := r.pool.QueryRow(ctx, "UPDATE app_recording_retention_policy SET enabled=$1,retention_days=$2,effective_at=$3,apply_to_existing=$4,updated_at=$5,updated_by=$6 WHERE id=TRUE RETURNING enabled,retention_days,effective_at,apply_to_existing,updated_at,updated_by", policy.Enabled, policy.Days, timestamptzFromPtr(policy.EffectiveAt), policy.ApplyToExisting, timestamptzFromPtr(policy.UpdatedAt), updatedBy).Scan(&out.Enabled, &out.Days, &effective, &out.ApplyToExisting, &updated, &by)
	if err != nil {
		return out, err
	}
	out.EffectiveAt = timeFromPg(effective)
	out.UpdatedAt = timeFromPg(updated)
	if by.Valid {
		out.UpdatedBy = by.String
	}
	return out, nil
}

func timestamptzFromPtr(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: v.UTC(), Valid: true}
}
func timeFromPg(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

var _ port.AdminRepository = (*AdminRepository)(nil)
var _ port.RecordingRetentionPolicyRepository = (*AdminRepository)(nil)
