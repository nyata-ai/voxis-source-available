package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that MediaRepository implements port.MediaRepository.
var _ port.MediaRepository = (*MediaRepository)(nil)

// MediaRepository implements port.MediaRepository using PostgreSQL.
type MediaRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewMediaRepository creates a new PostgreSQL media repository.
func NewMediaRepository(pool *pgxpool.Pool) *MediaRepository {
	return &MediaRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// Create persists a new media record.
func (r *MediaRepository) Create(ctx context.Context, media *domain.Media) error {
	orgUUID, err := parseUUID(media.OrganizationID)
	if err != nil {
		return err
	}

	if media.RecordingMode == domain.RecordingModePrivilege {
		return domain.ErrInvalidInput
	}

	row, err := r.queries.CreateMedia(ctx, sqlcdb.CreateMediaParams{
		OrganizationID: orgUUID,
		CreatedBy:      pgtype.Text{String: media.CreatedBy, Valid: media.CreatedBy != ""},
		Filename:       media.Filename,
		ContentType:    media.ContentType,
		Size:           media.Size,
		Duration:       media.Duration,
		Status:         media.Status,
		ScanStatus:     media.ScanStatus,
	})
	if err != nil {
		return err
	}

	// Populate DB-generated fields back onto the domain entity.
	media.ID = uuidToString(row.ID)
	media.CreatedAt = row.CreatedAt
	media.UpdatedAt = row.UpdatedAt
	media.RecordingMode = domain.RecordingModeRegular
	return nil
}

// GetByID retrieves a media record by ID.
func (r *MediaRepository) GetByID(ctx context.Context, id string) (*domain.Media, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetMediaByID(ctx, uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return mediaRowToDomain(row), nil
}

// ListByOrganization retrieves media records for an organization with pagination.
func (r *MediaRepository) ListByOrganization(ctx context.Context, orgID string, limit, offset int) ([]*domain.Media, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.ListMediaByOrganization(ctx, sqlcdb.ListMediaByOrganizationParams{
		OrganizationID: orgUUID,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by handler (1..100)
		OffsetVal:      int32(offset), //nolint:gosec // bounded by handler (>= 0)
	})
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Media, len(rows))
	for i, row := range rows {
		result[i] = mediaRowToDomain(row)
	}
	return result, nil
}

// Update updates an existing media record's encryption metadata and status.
func (r *MediaRepository) Update(ctx context.Context, media *domain.Media) error {
	uuid, err := parseUUID(media.ID)
	if err != nil {
		return domain.ErrNotFound
	}

	row, err := r.queries.UpdateMediaEncryption(ctx, sqlcdb.UpdateMediaEncryptionParams{
		ID:             uuid,
		StorageKey:     pgtype.Text{String: media.StorageKey, Valid: media.StorageKey != ""},
		WrappedDek:     media.WrappedDEK,
		WrappingNonce:  media.WrappingNonce,
		EncryptionAlgo: pgtype.Text{String: media.EncryptionAlgo, Valid: media.EncryptionAlgo != ""},
		ChunkSize:      int32(media.ChunkSize),  //nolint:gosec // bounded by encryption chunk size constant
		ChunkCount:     int32(media.ChunkCount), //nolint:gosec // bounded by file size / chunk size
		Status:         media.Status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}

	media.UpdatedAt = row.UpdatedAt
	return nil
}

// Delete performs a soft delete by setting status to "deleted".
func (r *MediaRepository) Delete(ctx context.Context, id string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	// Use SoftDeleteMedia which is :exec — it doesn't return ErrNoRows.
	// To detect not-found, we check the rows affected via UpdateMediaStatus instead.
	_, err = r.queries.UpdateMediaStatus(ctx, sqlcdb.UpdateMediaStatusParams{
		ID:     uuid,
		Status: domain.MediaStatusDeleted,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// CascadeDelete purges a media record and everything derived from it in one
// transaction: transcript, segment and summary ciphertext, collection
// memberships, recording chunk rows, and the media names, hash, and key
// references. Content-free tombstones remain with status "deleted".
func (r *MediaRepository) CascadeDelete(ctx context.Context, id string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	return inTx(ctx, r.pool, func(q *sqlcdb.Queries) error { return purgeMedia(ctx, q, uuid) })
}

// CountByOrganization returns the total number of non-deleted media records for an organization.
func (r *MediaRepository) CountByOrganization(ctx context.Context, orgID string) (int64, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, domain.ErrNotFound
	}

	return r.queries.CountMediaByOrganization(ctx, orgUUID)
}

// SearchByOrganization searches media with optional filename and status filters.
func (r *MediaRepository) SearchByOrganization(ctx context.Context, orgID, search, status string, limit, offset int) ([]*domain.Media, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.SearchMediaByOrganization(ctx, sqlcdb.SearchMediaByOrganizationParams{
		OrganizationID: orgUUID,
		SearchQuery:    search,
		StatusFilter:   status,
		LimitVal:       int32(limit),  //nolint:gosec // capped by handler
		OffsetVal:      int32(offset), //nolint:gosec // capped by handler
	})
	if err != nil {
		return nil, fmt.Errorf("search media: %w", err)
	}

	result := make([]*domain.Media, 0, len(rows))
	for _, row := range rows {
		result = append(result, mediaRowToDomain(row))
	}
	return result, nil
}

// CountSearchByOrganization counts media matching search/status filters.
func (r *MediaRepository) CountSearchByOrganization(ctx context.Context, orgID, search, status string) (int64, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, domain.ErrNotFound
	}

	count, err := r.queries.CountSearchMediaByOrganization(ctx, sqlcdb.CountSearchMediaByOrganizationParams{
		OrganizationID: orgUUID,
		SearchQuery:    search,
		StatusFilter:   status,
	})
	if err != nil {
		return 0, fmt.Errorf("count search media: %w", err)
	}
	return count, nil
}

// UpdateDuration updates duration on a media record.
func (r *MediaRepository) UpdateDuration(ctx context.Context, mediaID string, duration float64) error {
	uuid, err := parseUUID(mediaID)
	if err != nil {
		return domain.ErrNotFound
	}
	_, err = r.queries.UpdateMediaDuration(ctx, sqlcdb.UpdateMediaDurationParams{
		ID:       uuid,
		Duration: duration,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// UpdateMetadata updates title and/or description on a media record.
func (r *MediaRepository) UpdateMetadata(ctx context.Context, id string, title, description *string) (*domain.Media, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	setTitle := title != nil
	setDesc := description != nil
	titleVal := ""
	descVal := ""
	if setTitle {
		titleVal = *title
	}
	if setDesc {
		descVal = *description
	}

	row, err := r.queries.UpdateMediaMetadata(ctx, sqlcdb.UpdateMediaMetadataParams{
		ID:             uuid,
		SetTitle:       setTitle,
		Title:          pgtype.Text{String: titleVal, Valid: setTitle},
		SetDescription: setDesc,
		Description:    pgtype.Text{String: descVal, Valid: setDesc},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return mediaRowToDomain(row), nil
}

// UpdateForensics stores the file hash and encrypted audio metadata.
func (r *MediaRepository) UpdateForensics(ctx context.Context, id, fileHash string, audioMetadata []byte) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	_, err = r.queries.UpdateMediaForensics(ctx, sqlcdb.UpdateMediaForensicsParams{
		ID:            uuid,
		FileHash:      pgtype.Text{String: fileHash, Valid: fileHash != ""},
		AudioMetadata: audioMetadata,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// UpdateScanStatus updates the malware scan status for a media record.
func (r *MediaRepository) UpdateScanStatus(ctx context.Context, id, status string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	return r.queries.UpdateMediaScanStatus(ctx, sqlcdb.UpdateMediaScanStatusParams{
		ID:         uuid,
		ScanStatus: status,
	})
}

// MarkAudioDeleted clears audio storage metadata and records the audio deletion timestamp.
func (r *MediaRepository) MarkAudioDeleted(ctx context.Context, mediaID, storageKey string, deletedAt time.Time) (*domain.Media, error) {
	uuid, err := parseUUID(mediaID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	row, err := r.queries.MarkMediaAudioDeleted(ctx, sqlcdb.MarkMediaAudioDeletedParams{
		ID:         uuid,
		StorageKey: pgtype.Text{String: storageKey, Valid: storageKey != ""},
		DeletedAt:  pgtype.Timestamptz{Time: deletedAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrConflict
		}
		return nil, err
	}
	return mediaRowToDomain(row), nil
}

// ListByOrganizationFiltered retrieves media records with optional filters.
// Uses dynamic SQL because sqlc does not support dynamic ORDER BY or optional
// WHERE clauses with nil-semantics. Follows the same raw-SQL pattern as
// TranscriptionRepository.ListByOrgURL.
func (r *MediaRepository) ListByOrganizationFiltered(ctx context.Context, orgID string, filter port.MediaListFilter) ([]*domain.Media, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	q, args, limit := buildMediaFilterQuery(orgUUID, filter)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list media filtered: %w", err)
	}
	defer rows.Close()

	result := make([]*domain.Media, 0, limit)
	for rows.Next() {
		var row sqlcdb.Medium
		if scanErr := rows.Scan(
			&row.ID, &row.OrganizationID, &row.CreatedBy, &row.Filename, &row.ContentType,
			&row.Size, &row.Duration, &row.Status, &row.StorageKey,
			&row.WrappedDek, &row.WrappingNonce, &row.EncryptionAlgo,
			&row.ChunkSize, &row.ChunkCount, &row.CreatedAt, &row.UpdatedAt,
			&row.Title, &row.Description, &row.FileHash, &row.AudioMetadata,
			&row.ScanStatus, &row.AudioDeletedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan media row: %w", scanErr)
		}
		result = append(result, mediaRowToDomain(row))
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

// buildMediaFilterQuery constructs the dynamic SQL query for filtered media listing.
// Returns the query string, arguments slice, and effective limit.
func buildMediaFilterQuery(orgUUID pgtype.UUID, filter port.MediaListFilter) (query string, args []any, limit int) {
	query = `SELECT id, organization_id, created_by, filename, content_type, size, duration,
		status, storage_key, wrapped_dek, wrapping_nonce, encryption_algo,
		chunk_size, chunk_count, created_at, updated_at, title, description,
		file_hash, audio_metadata, scan_status, audio_deleted_at
	FROM media WHERE organization_id = $1 AND status != 'deleted'`
	args = []any{orgUUID}
	argIdx := 2

	if filter.DateFrom != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *filter.DateFrom)
		argIdx++
	}
	if filter.DateTo != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *filter.DateTo)
		argIdx++
	}
	if filter.MinDuration != nil {
		query += fmt.Sprintf(" AND duration >= $%d", argIdx)
		args = append(args, *filter.MinDuration)
		argIdx++
	}
	if filter.MaxDuration != nil {
		query += fmt.Sprintf(" AND duration <= $%d", argIdx)
		args = append(args, *filter.MaxDuration)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.Search != "" {
		escaped := escapeILIKE(filter.Search)
		query += fmt.Sprintf(` AND (
				filename ILIKE '%%' || $%d || '%%' ESCAPE '\' OR
				COALESCE(title, '') ILIKE '%%' || $%d || '%%' ESCAPE '\' OR
				COALESCE(description, '') ILIKE '%%' || $%d || '%%' ESCAPE '\'
			)`, argIdx, argIdx, argIdx)
		args = append(args, escaped)
		argIdx++
	}

	// Sort — whitelist validated upstream; default to created_at DESC.
	sortCol := mediaSortColumn(filter.SortBy)
	sortDir := "DESC"
	if filter.SortOrder == "asc" {
		sortDir = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortCol, sortDir)

	// Pagination
	limit = filter.Limit
	if limit <= 0 {
		limit = 20
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, int32(limit), int32(filter.Offset)) //nolint:gosec // bounded upstream

	return query, args, limit
}

// mediaSortColumn maps a sort field name to a safe SQL column identifier.
func mediaSortColumn(field string) string {
	switch field {
	case "duration":
		return "duration"
	case "size":
		return "size"
	default:
		return "created_at"
	}
}

// escapeILIKE escapes special ILIKE pattern characters in the input string.
func escapeILIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// SetRecordingMode rejects the only excluded recording mode. Standard sessions
// have no mutable mode in the OSS schema.
func (r *MediaRepository) SetRecordingMode(ctx context.Context, mediaID string, mode domain.RecordingMode) error {
	if _, err := parseUUID(mediaID); err != nil {
		return domain.ErrNotFound
	}
	if mode == domain.RecordingModePrivilege {
		return domain.ErrInvalidInput
	}
	return nil
}

// mediaRowToDomain converts a sqlcdb.Medium to a domain.Media.
func mediaRowToDomain(row sqlcdb.Medium) *domain.Media {
	return &domain.Media{
		ID:             uuidToString(row.ID),
		OrganizationID: uuidToString(row.OrganizationID),
		CreatedBy:      textToString(row.CreatedBy),
		Filename:       row.Filename,
		Title:          textToString(row.Title),
		Description:    textToString(row.Description),
		ContentType:    row.ContentType,
		Size:           row.Size,
		Duration:       row.Duration,
		Status:         row.Status,
		StorageKey:     textToString(row.StorageKey),
		WrappedDEK:     row.WrappedDek,
		WrappingNonce:  row.WrappingNonce,
		EncryptionAlgo: textToString(row.EncryptionAlgo),
		ChunkSize:      int(row.ChunkSize),
		ChunkCount:     int(row.ChunkCount),
		FileHash:       textToString(row.FileHash),
		AudioMetadata:  row.AudioMetadata,
		ScanStatus:     row.ScanStatus,
		RecordingMode:  domain.RecordingModeRegular,
		AudioDeletedAt: timestamptzToTimePtr(row.AudioDeletedAt),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
