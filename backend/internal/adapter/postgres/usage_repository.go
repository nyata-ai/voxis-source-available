package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that UsageRepository implements port.UsageRepository.
var _ port.UsageRepository = (*UsageRepository)(nil)

// UsageRepository implements port.UsageRepository using PostgreSQL.
type UsageRepository struct {
	queries *sqlcdb.Queries
}

// NewUsageRepository creates a new PostgreSQL usage repository.
func NewUsageRepository(pool *pgxpool.Pool) *UsageRepository {
	return &UsageRepository{
		queries: sqlcdb.New(pool),
	}
}

// GetStats returns aggregated usage statistics for an organization.
func (r *UsageRepository) GetStats(ctx context.Context, orgID string) (*port.UsageStats, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetUsageStats(ctx, orgUUID)
	if err != nil {
		return nil, err
	}

	return &port.UsageStats{
		TotalDurationSeconds:          row.TotalDurationSeconds,
		TotalPromptTokens:             row.TotalPromptTokens,
		TotalCompletionTokens:         row.TotalCompletionTokens,
		TotalThinkingTokens:           row.TotalThinkingTokens,
		StorageUsedBytes:              row.StorageUsedBytes,
		LiveRecordingRetentionEnabled: row.LiveRecordingRetentionEnabled,
		LiveRecordingRetentionDays:    row.LiveRecordingRetentionDays,
	}, nil
}

// GetURLTranscriptionCounts returns zero without querying omitted URL tables because
// URL transcription is not delivered in OSS.
func (r *UsageRepository) GetURLTranscriptionCounts(context.Context, string, string) (port.URLTranscriptionCounts, error) {
	return port.URLTranscriptionCounts{}, nil
}
