package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// maxPurgePageSize bounds one listing page of the operator purge.
const maxPurgePageSize = 500

var _ port.DataPurgeRepository = (*DataPurgeRepository)(nil)
var _ port.MediaPurgeChunkLister = (*MediaRepository)(nil)

// inTx runs fn in one transaction and commits only when fn succeeds.
func inTx(ctx context.Context, pool *pgxpool.Pool, fn func(*sqlcdb.Queries) error) error {
	if pool == nil {
		return fmt.Errorf("postgres pool is required: %w", domain.ErrInternal)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	if err := fn(sqlcdb.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// purgeTranscriptions removes every ciphertext column derived from the given
// transcriptions (summaries, segments, the transcript itself) and their
// collection memberships. Callers must already hold the transcription locks.
func purgeTranscriptions(ctx context.Context, q *sqlcdb.Queries, ids []pgtype.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if err := q.PurgeSummariesByTranscriptionIDs(ctx, ids); err != nil {
		return fmt.Errorf("purge summaries: %w", err)
	}
	if err := q.PurgeSegmentsByTranscriptionIDs(ctx, ids); err != nil {
		return fmt.Errorf("purge transcription segments: %w", err)
	}
	if err := q.PurgeTranscriptionsByIDs(ctx, ids); err != nil {
		return fmt.Errorf("purge transcriptions: %w", err)
	}
	if err := q.DeleteCollectionItemsByTranscriptionIDs(ctx, ids); err != nil {
		return fmt.Errorf("delete collection items: %w", err)
	}
	return nil
}

// purgeMedia turns one media row and everything derived from it into
// content-free tombstones inside the caller's transaction. It locks the media
// row, then its transcriptions, before writing anything.
func purgeMedia(ctx context.Context, q *sqlcdb.Queries, mediaID pgtype.UUID) error {
	if _, err := q.LockMediaForPurge(ctx, mediaID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("lock media: %w", err)
	}
	transcriptionIDs, err := q.LockMediaTranscriptionsForPurge(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("lock media transcriptions: %w", err)
	}
	if err := purgeTranscriptions(ctx, q, transcriptionIDs); err != nil {
		return err
	}
	if err := q.DeleteMediaRecordingChunks(ctx, mediaID); err != nil {
		return fmt.Errorf("delete recording chunk rows: %w", err)
	}
	if err := q.ScrubMediaRecordingSessions(ctx, mediaID); err != nil {
		return fmt.Errorf("scrub recording sessions: %w", err)
	}
	if err := q.PurgeMediaRow(ctx, mediaID); err != nil {
		return fmt.Errorf("purge media row: %w", err)
	}
	return nil
}

// ListRecordingChunkKeysForPurge returns the storage keys of chunk rows still
// recorded for the session a media row was stitched from.
func (r *MediaRepository) ListRecordingChunkKeysForPurge(ctx context.Context, mediaID string) ([]string, error) {
	id, err := parseUUID(mediaID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	return r.queries.ListMediaRecordingChunkKeysForPurge(ctx, id)
}

// DataPurgeRepository implements port.DataPurgeRepository using PostgreSQL.
type DataPurgeRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewDataPurgeRepository creates the repository behind the purge-user command.
func NewDataPurgeRepository(pool *pgxpool.Pool) *DataPurgeRepository {
	return &DataPurgeRepository{pool: pool, queries: sqlcdb.New(pool)}
}

// GetUserOrganization returns the person's organization and its member count.
func (r *DataPurgeRepository) GetUserOrganization(ctx context.Context, userID string) (port.PurgeUserOrganization, error) {
	if userID == "" {
		return port.PurgeUserOrganization{}, domain.ErrInvalidInput
	}
	row, err := r.queries.GetUserOrganizationForPurge(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.PurgeUserOrganization{}, domain.ErrNotFound
	}
	if err != nil {
		return port.PurgeUserOrganization{}, err
	}
	return port.PurgeUserOrganization{
		UserID: row.ID, OrganizationID: uuidToString(row.OrganizationID), MemberCount: row.MemberCount,
	}, nil
}

// CountOrganization returns the dry-run inventory of the organization's data.
func (r *DataPurgeRepository) CountOrganization(ctx context.Context, orgID string) (port.PurgeCounts, error) {
	id, err := parseUUID(orgID)
	if err != nil {
		return port.PurgeCounts{}, domain.ErrNotFound
	}
	row, err := r.queries.CountOrganizationPurgeTargets(ctx, id)
	if err != nil {
		return port.PurgeCounts{}, err
	}
	return port.PurgeCounts{
		Media: row.Media, Transcriptions: row.Transcriptions, Summaries: row.Summaries,
		RecordingSessions: row.RecordingSessions, RecordingChunks: row.RecordingChunks,
		Collections: row.Collections, APIKeys: row.ApiKeys, PendingProviderDeletions: row.PendingProviderDeletions,
	}, nil
}

// ListMediaIDsAfter pages every media row of the organization, deleted or not.
func (r *DataPurgeRepository) ListMediaIDsAfter(ctx context.Context, orgID, afterID string, limit int) ([]string, error) {
	params, err := purgePageParams(orgID, afterID, limit)
	if err != nil {
		return nil, err
	}
	ids, err := r.queries.ListOrganizationMediaIDsAfter(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, uuidToString(id))
	}
	return out, nil
}

// DeleteAPIKeys deletes every API key of the organization. The purge calls it
// first, so a key cannot add content while the rest of the purge runs.
func (r *DataPurgeRepository) DeleteAPIKeys(ctx context.Context, orgID string) error {
	id, err := parseUUID(orgID)
	if err != nil {
		return domain.ErrNotFound
	}
	return r.queries.DeleteOrganizationAPIKeys(ctx, id)
}

// DeleteOrganizationData removes the organization's non-media rows in one
// transaction and, when asked, the organization itself.
func (r *DataPurgeRepository) DeleteOrganizationData(ctx context.Context, orgID string, deleteOrganization bool) error {
	id, err := parseUUID(orgID)
	if err != nil {
		return domain.ErrNotFound
	}
	return inTx(ctx, r.pool, func(q *sqlcdb.Queries) error {
		if err := deleteOrganizationSideRows(ctx, q, id); err != nil {
			return err
		}
		if !deleteOrganization {
			return scrubOrganizationIdentity(ctx, q, id)
		}
		deleted, err := q.DeleteOrganizationForPurge(ctx, id)
		if err != nil {
			return fmt.Errorf("delete organization: %w", err)
		}
		if deleted != 1 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// deleteOrganizationSideRows removes the rows that hold no media content but
// still describe the person's work: recordings (and chunk rows), collections,
// API keys, and storage allocations.
func deleteOrganizationSideRows(ctx context.Context, q *sqlcdb.Queries, id pgtype.UUID) error {
	if err := q.DeleteOrganizationRecordingSessions(ctx, id); err != nil {
		return fmt.Errorf("delete recording sessions: %w", err)
	}
	if err := q.DeleteOrganizationCollections(ctx, id); err != nil {
		return fmt.Errorf("delete collections: %w", err)
	}
	if err := q.DeleteOrganizationAPIKeys(ctx, id); err != nil {
		return fmt.Errorf("delete API keys: %w", err)
	}
	if err := q.DeleteOrganizationStorageAllocations(ctx, id); err != nil {
		return fmt.Errorf("delete storage allocations: %w", err)
	}
	return nil
}

// scrubOrganizationIdentity removes names, email addresses, and preferences
// from an organization that must stay until provider deletion is confirmed.
func scrubOrganizationIdentity(ctx context.Context, q *sqlcdb.Queries, id pgtype.UUID) error {
	if err := q.ScrubOrganizationIdentityForPurge(ctx, id); err != nil {
		return fmt.Errorf("scrub organization identity: %w", err)
	}
	if err := q.ScrubOrganizationUsersForPurge(ctx, id); err != nil {
		return fmt.Errorf("scrub organization users: %w", err)
	}
	return nil
}

func purgePageParams(orgID, afterID string, limit int) (sqlcdb.ListOrganizationMediaIDsAfterParams, error) {
	var params sqlcdb.ListOrganizationMediaIDsAfterParams
	org, err := parseUUID(orgID)
	if err != nil {
		return params, domain.ErrNotFound
	}
	if limit <= 0 || limit > maxPurgePageSize {
		return params, fmt.Errorf("purge page size %d outside 1..%d: %w", limit, maxPurgePageSize, domain.ErrInvalidInput)
	}
	after := pgtype.UUID{Valid: true} // the zero UUID sorts before every generated ID
	if afterID != "" {
		if after, err = parseUUID(afterID); err != nil {
			return params, domain.ErrInvalidInput
		}
	}
	params.OrgID, params.AfterID, params.LimitVal = org, after, int32(limit) //nolint:gosec // bounded by maxPurgePageSize above
	return params, nil
}
