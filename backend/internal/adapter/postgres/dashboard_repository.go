package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that DashboardRepository implements port.DashboardRepository.
var _ port.DashboardRepository = (*DashboardRepository)(nil)

// DashboardRepository implements port.DashboardRepository using PostgreSQL.
type DashboardRepository struct {
	queries *sqlcdb.Queries
}

// NewDashboardRepository creates a new PostgreSQL dashboard repository.
func NewDashboardRepository(pool *pgxpool.Pool) *DashboardRepository {
	return &DashboardRepository{
		queries: sqlcdb.New(pool),
	}
}

// GetStats returns aggregated dashboard statistics for an organization.
func (r *DashboardRepository) GetStats(ctx context.Context, orgID string) (*port.DashboardStats, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetDashboardStats(ctx, orgUUID)
	if err != nil {
		return nil, err
	}

	stats := &port.DashboardStats{
		TotalMedia:              row.TotalMedia,
		ProcessingCount:         row.ProcessingCount,
		EncryptingCount:         row.EncryptingCount,
		TranscribingCount:       row.TranscribingCount,
		CompletedTranscriptions: row.CompletedTranscriptions,
		CompletedThisMonth:      row.CompletedThisMonth,
		CompletedLastMonth:      row.CompletedLastMonth,
		SecondsThisMonth:        row.SecondsThisMonth,
	}

	// The SQL COALESCEs MAX(completed_at) to 1970-01-01 as a sentinel for "no
	// completions yet". Anything before 2000 is treated as absent.
	if row.LastCompletedAt.Year() >= 2000 {
		t := row.LastCompletedAt
		stats.LastCompletedAt = &t
	}

	return stats, nil
}
