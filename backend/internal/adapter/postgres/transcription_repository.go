package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that TranscriptionRepository implements port.TranscriptionRepository.
var _ port.TranscriptionRepository = (*TranscriptionRepository)(nil)
var _ port.TranscriptionSegmentRepository = (*TranscriptionRepository)(nil)

// TranscriptionRepository implements port.TranscriptionRepository using PostgreSQL.
type TranscriptionRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewTranscriptionRepository creates a new PostgreSQL transcription repository.
func NewTranscriptionRepository(pool *pgxpool.Pool) *TranscriptionRepository {
	return &TranscriptionRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// expectedSpeakersToPg maps the domain's 0-means-auto int onto the nullable
// expected_speakers column. Out-of-range values are stored as NULL rather than
// letting the CHECK constraint turn a client bug into a 500.
//
// The clamp is deliberately silent, and this repository deliberately holds no
// logger. The real gates are upstream and both reject out-of-range values: the
// API boundary (resolveExpectedSpeakers, a 400) and the domain
// (SetExpectedSpeakers/ValidateExpectedSpeakers). A value can only reach this
// function out of range through a new code path that skipped both, i.e. a bug
// to be caught by the tests on those gates — not a runtime condition an
// operator could act on. Writing NULL keeps that bug from failing a submission.
func expectedSpeakersToPg(n int) pgtype.Int4 {
	if n < 1 || n > domain.MaxExpectedSpeakers {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

// expectedSpeakersFromPg maps a nullable expected_speakers column back to the
// domain's 0-means-auto int.
func expectedSpeakersFromPg(v pgtype.Int4) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int32)
}

func postgresInt(value int) (int32, error) {
	const maxPostgresInt = int(^uint32(0) >> 1)
	if value < 0 || value > maxPostgresInt {
		return 0, domain.ErrInvalidInput
	}
	return int32(value), nil
}

// Create persists a new transcription record.
func (r *TranscriptionRepository) Create(ctx context.Context, t *domain.Transcription) error {
	orgUUID, err := parseUUID(t.OrganizationID)
	if err != nil {
		return err
	}

	mediaUUID, err := parseUUID(t.MediaID)
	if err != nil {
		return err
	}

	row, err := r.queries.CreateTranscription(ctx, sqlcdb.CreateTranscriptionParams{
		OrganizationID:   orgUUID,
		MediaID:          mediaUUID,
		Languages:        t.Languages,
		Diarization:      t.Diarization,
		EnhanceAudio:     t.EnhanceAudio,
		Status:           t.Status,
		VocabularyPacks:  t.VocabularyPacks,
		ExpectedSpeakers: expectedSpeakersToPg(t.ExpectedSpeakers),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	// Populate DB-generated fields back onto the domain entity.
	t.ID = uuidToString(row.ID)
	t.CreatedAt = row.CreatedAt
	t.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID retrieves a transcription record by ID.
func (r *TranscriptionRepository) GetByID(ctx context.Context, id string) (*domain.Transcription, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetTranscriptionByID(ctx, uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return transcriptionRowToDomain(row), nil
}

// GetByMediaID retrieves the active transcription for mediaID.
func (r *TranscriptionRepository) GetByMediaID(ctx context.Context, mediaID string) (*domain.Transcription, error) {
	uuid, err := parseUUID(mediaID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetTranscriptionByMediaID(ctx, uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return transcriptionRowToDomain(row), nil
}

// ListByOrganization retrieves transcriptions for an organization with pagination.
// Uses a JOIN to include media filenames, titles, and descriptions in a single query.
// If search is non-empty, filters by media title, description, or filename.
func (r *TranscriptionRepository) ListByOrganization(ctx context.Context, orgID string, limit, offset int, search string) ([]*domain.Transcription, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.ListTranscriptionsByOrganization(ctx, sqlcdb.ListTranscriptionsByOrganizationParams{
		OrganizationID: orgUUID,
		Search:         search,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by handler (1..100)
		OffsetVal:      int32(offset), //nolint:gosec // bounded by handler (>= 0)
	})
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Transcription, len(rows))
	for i, row := range rows {
		result[i] = transcriptionListRowToDomain(row)
	}
	return result, nil
}

// ListCompletedForTranscriptSearch returns one bounded keyset page of completed
// media transcriptions. The caller decrypts each candidate only after this
// organization-scoped query has selected it.
func (r *TranscriptionRepository) ListCompletedForTranscriptSearch(
	ctx context.Context,
	orgID string,
	cursor *port.TranscriptSearchCursor,
	limit int,
) ([]*domain.Transcription, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	if limit < 1 || limit > 26 {
		return nil, domain.ErrInvalidInput
	}

	params := sqlcdb.ListCompletedTranscriptionsForTranscriptSearchParams{
		OrganizationID: orgUUID,
		LimitVal:       int32(limit),
	}
	if cursor != nil {
		cursorID, cursorErr := parseUUID(cursor.ID)
		if cursorErr != nil || cursor.CreatedAt.IsZero() {
			return nil, domain.ErrInvalidInput
		}
		params.HasCursor = true
		params.AfterCreatedAt = cursor.CreatedAt
		params.AfterID = cursorID
	}

	rows, err := r.queries.ListCompletedTranscriptionsForTranscriptSearch(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list completed transcript search candidates: %w", err)
	}
	result := make([]*domain.Transcription, len(rows))
	for i, row := range rows {
		result[i] = transcriptionSearchRowToDomain(row)
	}
	return result, nil
}

// CountByOrganization returns the total number of non-deleted transcriptions for an organization.
// If search is non-empty, counts only matching items.
func (r *TranscriptionRepository) CountByOrganization(ctx context.Context, orgID, search string) (int64, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, domain.ErrNotFound
	}

	return r.queries.CountTranscriptionsByOrganization(ctx, sqlcdb.CountTranscriptionsByOrganizationParams{
		OrganizationID: orgUUID,
		Search:         search,
	})
}

// Update persists transcription state and encrypted provider output.
func (r *TranscriptionRepository) Update(ctx context.Context, t *domain.Transcription) error {
	uuid, err := parseUUID(t.ID)
	if err != nil {
		return domain.ErrNotFound
	}

	if t.ContentEncrypted != nil {
		row, contentErr := r.queries.UpdateTranscriptionContent(ctx, sqlcdb.UpdateTranscriptionContentParams{
			ID:               uuid,
			Status:           t.Status,
			ContentEncrypted: t.ContentEncrypted,
			ContentNonce:     t.ContentNonce,
			WrappedDek:       t.WrappedDEK,
			WrappingNonce:    t.WrappingNonce,
			SpeakerCount:     int32(t.SpeakerCount), //nolint:gosec // bounded by transcript metadata
			WordCount:        int32(t.WordCount),    //nolint:gosec // bounded by transcript metadata
			DurationSeconds:  t.DurationSeconds,
		})
		if contentErr != nil {
			if errors.Is(contentErr, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return contentErr
		}
		t.UpdatedAt = row.UpdatedAt
		if row.CompletedAt.Valid {
			t.CompletedAt = &row.CompletedAt.Time
		}
		return nil
	}

	// Status-only update (SetSubmitted, SetFailed).
	row, err := r.queries.UpdateTranscriptionStatus(ctx, sqlcdb.UpdateTranscriptionStatusParams{
		ID:     uuid,
		Status: t.Status, ErrorMessage: pgtype.Text{String: t.ErrorMessage, Valid: t.ErrorMessage != ""},
		PreprocessorUsed: pgtype.Text{String: t.PreprocessorUsed, Valid: t.PreprocessorUsed != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	t.UpdatedAt = row.UpdatedAt
	return nil
}

// UpdateContentCAS performs an optimistic compare-and-swap of the encrypted
// content envelope columns, guarded by expectedNonce. Returns true iff exactly
// one row was updated (swap committed), false on a nonce mismatch (0 rows).
func (r *TranscriptionRepository) UpdateContentCAS(ctx context.Context, t *domain.Transcription, expectedNonce []byte) (bool, error) {
	uuid, err := parseUUID(t.ID)
	if err != nil {
		return false, domain.ErrNotFound
	}

	rows, err := r.queries.UpdateTranscriptionContentCAS(ctx, sqlcdb.UpdateTranscriptionContentCASParams{
		ID:                   uuid,
		ContentEncrypted:     t.ContentEncrypted,
		ContentNonce:         t.ContentNonce,
		WrappedDek:           t.WrappedDEK,
		WrappingNonce:        t.WrappingNonce,
		ExpectedContentNonce: expectedNonce,
	})
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// Delete soft-deletes a transcription by setting status to "deleted".
func (r *TranscriptionRepository) Delete(ctx context.Context, id string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	// SoftDeleteTranscription is :exec, so we use UpdateTranscriptionStatus
	// to detect not-found via ErrNoRows on the RETURNING clause.
	_, err = r.queries.UpdateTranscriptionStatus(ctx, sqlcdb.UpdateTranscriptionStatusParams{
		ID:               uuid,
		Status:           domain.TranscriptionStatusDeleted,
		PreprocessorUsed: pgtype.Text{}, // preserve existing value
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// CascadeDelete purges a transcription in one transaction: its transcript,
// segment and summary ciphertext and its collection memberships. A
// content-free tombstone remains with status "deleted".
func (r *TranscriptionRepository) CascadeDelete(ctx context.Context, id string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	return inTx(ctx, r.pool, func(q *sqlcdb.Queries) error {
		if _, lockErr := q.LockTranscriptionForPurge(ctx, uuid); lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("lock transcription: %w", lockErr)
		}
		return purgeTranscriptions(ctx, q, []pgtype.UUID{uuid})
	})
}

// DeleteByMediaID soft-deletes all non-deleted transcriptions for the given media.
// Returns the number of transcriptions affected. An invalid UUID yields zero rows.
func (r *TranscriptionRepository) DeleteByMediaID(ctx context.Context, mediaID string) (int64, error) {
	mediaUUID, err := parseUUID(mediaID)
	if err != nil {
		return 0, nil //nolint:nilerr // invalid UUID means no matching rows
	}

	return r.queries.SoftDeleteTranscriptionsByMediaID(ctx, mediaUUID)
}

// ListStaleSubmitted returns submitted transcriptions older than olderThan.
func (r *TranscriptionRepository) ListStaleSubmitted(ctx context.Context, olderThan time.Time) ([]*domain.Transcription, error) {
	return r.ListStaleSubmittedSpeechmatics(ctx, olderThan)
}

// ListByOrganizationFiltered returns organization transcriptions matching filter.
func (r *TranscriptionRepository) ListByOrganizationFiltered(ctx context.Context, orgID string, filter port.TranscriptionListFilter) ([]*domain.Transcription, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	q, args, limit := buildTranscriptionFilterQuery(orgUUID, filter)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list transcriptions filtered: %w", err)
	}
	defer rows.Close()

	result := make([]*domain.Transcription, 0, limit)
	for rows.Next() {
		t, scanErr := scanFilteredTranscriptionRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan transcription row: %w", scanErr)
		}
		result = append(result, t)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

// buildTranscriptionFilterQuery constrains every search and sort operand to
// retained media-backed rows.
func buildTranscriptionFilterQuery(orgUUID pgtype.UUID, filter port.TranscriptionListFilter) (resultQuery string, resultArgs []any, resultLimit int) {
	query := `SELECT t.id, t.organization_id, t.media_id, t.provider, t.speechmatics_job_id,
		t.speechmatics_deleted_at, t.status, t.languages, t.diarization, t.expected_speakers,
		t.vocabulary_packs, t.enhance_audio, t.preprocessor_used, t.content_encrypted,
		t.content_nonce, t.wrapped_dek, t.wrapping_nonce, t.speaker_count, t.word_count,
		t.duration_seconds, t.error_message, t.created_at, t.updated_at, t.completed_at,
		m.filename, COALESCE(m.title, ''), COALESCE(m.description, ''), m.status
	FROM transcriptions t JOIN media m ON m.id = t.media_id
	WHERE t.organization_id = $1 AND t.status != 'deleted' AND m.status != 'deleted'`
	args := []any{orgUUID}
	arg := 2
	if filter.DateFrom != nil {
		query += fmt.Sprintf(" AND t.created_at >= $%d", arg)
		args = append(args, *filter.DateFrom)
		arg++
	}
	if filter.DateTo != nil {
		query += fmt.Sprintf(" AND t.created_at <= $%d", arg)
		args = append(args, *filter.DateTo)
		arg++
	}
	if filter.MinDuration != nil {
		query += fmt.Sprintf(" AND t.duration_seconds >= $%d", arg)
		args = append(args, *filter.MinDuration)
		arg++
	}
	if filter.MaxDuration != nil {
		query += fmt.Sprintf(" AND t.duration_seconds <= $%d", arg)
		args = append(args, *filter.MaxDuration)
		arg++
	}
	if len(filter.Languages) > 0 {
		query += fmt.Sprintf(" AND t.languages && $%d", arg)
		args = append(args, filter.Languages)
		arg++
	}
	if filter.MinSpeakers != nil {
		query += fmt.Sprintf(" AND t.speaker_count >= $%d", arg)
		args = append(args, *filter.MinSpeakers)
		arg++
	}
	if filter.MaxSpeakers != nil {
		query += fmt.Sprintf(" AND t.speaker_count <= $%d", arg)
		args = append(args, *filter.MaxSpeakers)
		arg++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND t.status = $%d", arg)
		args = append(args, filter.Status)
		arg++
	}
	if filter.Search != "" {
		query += fmt.Sprintf(` AND (m.filename ILIKE '%%' || $%d || '%%' ESCAPE '\' OR COALESCE(m.title, '') ILIKE '%%' || $%d || '%%' ESCAPE '\' OR COALESCE(m.description, '') ILIKE '%%' || $%d || '%%' ESCAPE '\')`, arg, arg, arg)
		args = append(args, escapeILIKE(filter.Search))
		arg++
	}
	direction := "DESC"
	if filter.SortOrder == "asc" {
		direction = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", transcriptionSortColumn(filter.SortBy), direction)
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", arg, arg+1)
	offset := max(filter.Offset, 0)
	args = append(args, limit, offset)
	return query, args, limit
}

func transcriptionSortColumn(field string) string {
	switch field {
	case "duration_seconds":
		return "t.duration_seconds"
	case "word_count":
		return "t.word_count"
	case "speaker_count":
		return "t.speaker_count"
	default:
		return "t.created_at"
	}
}

func scanFilteredTranscriptionRow(rows pgx.Rows) (*domain.Transcription, error) {
	var row sqlcdb.Transcription
	var filename, title, description, mediaStatus string
	err := rows.Scan(&row.ID, &row.OrganizationID, &row.MediaID, &row.Provider, &row.SpeechmaticsJobID,
		&row.SpeechmaticsDeletedAt, &row.Status, &row.Languages, &row.Diarization, &row.ExpectedSpeakers,
		&row.VocabularyPacks, &row.EnhanceAudio, &row.PreprocessorUsed, &row.ContentEncrypted,
		&row.ContentNonce, &row.WrappedDek, &row.WrappingNonce, &row.SpeakerCount, &row.WordCount,
		&row.DurationSeconds, &row.ErrorMessage, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt,
		&filename, &title, &description, &mediaStatus)
	if err != nil {
		return nil, err
	}
	t := transcriptionRowToDomain(row)
	t.MediaFilename, t.MediaTitle, t.MediaDescription, t.MediaStatus = filename, title, description, mediaStatus
	return t, nil
}

// CreateBatch persists bounded chunk metadata for a media transcription.
func (r *TranscriptionRepository) CreateBatch(ctx context.Context, segments []*domain.TranscriptionSegment) error {
	if len(segments) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackAfterTransaction(ctx, tx, "create transcription segments")
	const query = `INSERT INTO transcription_segments (transcription_id, segment_index, start_offset_sec, end_offset_sec, status) VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at, updated_at`
	for _, segment := range segments {
		if segment == nil {
			return domain.ErrInvalidInput
		}
		segmentIndex, indexErr := postgresInt(segment.SegmentIndex)
		if indexErr != nil {
			return indexErr
		}
		transcriptionID, err := parseUUID(segment.TranscriptionID)
		if err != nil {
			return domain.ErrInvalidInput
		}
		var id pgtype.UUID
		if err := tx.QueryRow(ctx, query, transcriptionID, segmentIndex, segment.StartOffsetSec, segment.EndOffsetSec, segment.Status).Scan(&id, &segment.CreatedAt, &segment.UpdatedAt); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return domain.ErrConflict
			}
			return err
		}
		segment.ID = uuidToString(id)
	}
	return tx.Commit(ctx)
}

const segmentColumns = `id, transcription_id, segment_index, start_offset_sec, end_offset_sec, speechmatics_job_id, status, content_encrypted, content_nonce, wrapped_dek, wrapping_nonce, speaker_count, word_count, duration_seconds, error_message, created_at, updated_at, completed_at, speechmatics_deleted_at`

// ListByTranscriptionID returns persisted Speechmatics segments in source order.
func (r *TranscriptionRepository) ListByTranscriptionID(ctx context.Context, transcriptionID string) ([]*domain.TranscriptionSegment, error) {
	id, err := parseUUID(transcriptionID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	return r.querySegments(ctx, `SELECT `+segmentColumns+` FROM transcription_segments WHERE transcription_id=$1 ORDER BY segment_index ASC`, id)
}

// UpdateSegment persists one encrypted transcription segment.
func (r *TranscriptionRepository) UpdateSegment(ctx context.Context, segment *domain.TranscriptionSegment) error {
	if segment == nil {
		return domain.ErrInvalidInput
	}
	id, err := parseUUID(segment.ID)
	if err != nil {
		return domain.ErrNotFound
	}
	var completed pgtype.Timestamptz
	if segment.CompletedAt != nil {
		completed = pgtype.Timestamptz{Time: *segment.CompletedAt, Valid: true}
	}
	speakerCount, speakerErr := postgresInt(segment.SpeakerCount)
	if speakerErr != nil {
		return speakerErr
	}
	wordCount, wordErr := postgresInt(segment.WordCount)
	if wordErr != nil {
		return wordErr
	}
	err = r.pool.QueryRow(ctx, updateSegmentQuery, id, segment.Status, segment.ContentEncrypted, segment.ContentNonce, segment.WrappedDEK, segment.WrappingNonce, speakerCount, wordCount, segment.DurationSeconds, pgtype.Text{String: segment.ErrorMessage, Valid: segment.ErrorMessage != ""}, completed).Scan(&segment.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// updateSegmentQuery share-locks the parent transcription and writes only
// while it is not deleted. A late provider result for a purged transcription
// therefore cannot put ciphertext back; the media purge takes its row locks
// before this lock can be granted and the recheck then sees "deleted".
const updateSegmentQuery = `UPDATE transcription_segments SET status=$2, content_encrypted=$3, content_nonce=$4, wrapped_dek=$5, wrapping_nonce=$6, speaker_count=$7, word_count=$8, duration_seconds=$9, error_message=$10, completed_at=$11
WHERE id=$1 AND transcription_id = (
    SELECT t.id FROM transcriptions t
    JOIN transcription_segments s ON s.transcription_id = t.id
    WHERE s.id = $1 AND t.status <> 'deleted'
    FOR SHARE OF t)
RETURNING updated_at`

// ListStaleSubmittedSegments returns submitted segment jobs older than olderThan.
func (r *TranscriptionRepository) ListStaleSubmittedSegments(ctx context.Context, olderThan time.Time) ([]*domain.TranscriptionSegment, error) {
	return r.ListStaleSubmittedSpeechmaticsSegments(ctx, olderThan)
}

type segmentScanner interface{ Scan(...any) error }

func scanTranscriptionSegmentRow(row segmentScanner) (*domain.TranscriptionSegment, error) {
	var id, transcriptionID pgtype.UUID
	var index int32
	var start, end, duration float64
	var jobID, message pgtype.Text
	var status string
	var encrypted, nonce, dek, wrapping []byte
	var speakers, words int32
	var created, updated time.Time
	var completed, deleted pgtype.Timestamptz
	if err := row.Scan(&id, &transcriptionID, &index, &start, &end, &jobID, &status, &encrypted, &nonce, &dek, &wrapping, &speakers, &words, &duration, &message, &created, &updated, &completed, &deleted); err != nil {
		return nil, err
	}
	segment := &domain.TranscriptionSegment{ID: uuidToString(id), TranscriptionID: uuidToString(transcriptionID), SegmentIndex: int(index), StartOffsetSec: start, EndOffsetSec: end, SpeechmaticsJobID: textToString(jobID), Status: status, ContentEncrypted: encrypted, ContentNonce: nonce, WrappedDEK: dek, WrappingNonce: wrapping, SpeakerCount: int(speakers), WordCount: int(words), DurationSeconds: duration, ErrorMessage: textToString(message), CreatedAt: created, UpdatedAt: updated}
	if completed.Valid {
		segment.CompletedAt = &completed.Time
	}
	if deleted.Valid {
		segment.SpeechmaticsDeletedAt = &deleted.Time
	}
	return segment, nil
}

func (r *TranscriptionRepository) querySegments(ctx context.Context, query string, args ...any) ([]*domain.TranscriptionSegment, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*domain.TranscriptionSegment, 0)
	for rows.Next() {
		segment, err := scanTranscriptionSegmentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, segment)
	}
	return out, rows.Err()
}

// ListRecentActivity returns the newest retained transcriptions for orgID.
func (r *TranscriptionRepository) ListRecentActivity(ctx context.Context, orgID string, limit int) ([]*domain.Transcription, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 25 {
		limit = 25
	}
	return r.ListByOrganization(ctx, orgID, limit, 0, "")
}

// ListActiveActivity returns in-progress organization work for the activity feed.
func (r *TranscriptionRepository) ListActiveActivity(ctx context.Context, orgID string, limit int) ([]*domain.Transcription, error) {
	id, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 25 {
		limit = 25
	}
	const query = `SELECT t.id,t.organization_id,t.media_id,t.provider,t.speechmatics_job_id,t.speechmatics_deleted_at,t.status,t.languages,t.diarization,t.expected_speakers,t.vocabulary_packs,t.enhance_audio,t.preprocessor_used,t.content_encrypted,t.content_nonce,t.wrapped_dek,t.wrapping_nonce,t.speaker_count,t.word_count,t.duration_seconds,t.error_message,t.created_at,t.updated_at,t.completed_at,m.filename,COALESCE(m.title,''),COALESCE(m.description,''),m.status FROM transcriptions t JOIN media m ON m.id=t.media_id AND m.status!='deleted' WHERE t.organization_id=$1 AND t.status!='deleted' AND (t.status IN ('pending','submitted') OR t.updated_at>NOW()-INTERVAL '5 minutes' OR EXISTS(SELECT 1 FROM summaries s WHERE s.transcription_id=t.id AND (s.status='pending' OR s.updated_at>NOW()-INTERVAL '5 minutes'))) ORDER BY t.updated_at DESC LIMIT $2`
	queryLimit, limitErr := postgresInt(limit)
	if limitErr != nil {
		return nil, limitErr
	}
	rows, err := r.pool.Query(ctx, query, id, queryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*domain.Transcription, 0, limit)
	for rows.Next() {
		value, err := scanFilteredTranscriptionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
func transcriptionListRowToDomain(row sqlcdb.ListTranscriptionsByOrganizationRow) *domain.Transcription {
	t := &domain.Transcription{ID: uuidToString(row.ID), OrganizationID: uuidToString(row.OrganizationID), MediaID: uuidToString(row.MediaID), Provider: row.Provider, SpeechmaticsJobID: textToString(row.SpeechmaticsJobID), Status: row.Status, Languages: row.Languages, Diarization: row.Diarization, EnhanceAudio: row.EnhanceAudio, PreprocessorUsed: textToString(row.PreprocessorUsed), ContentEncrypted: row.ContentEncrypted, ContentNonce: row.ContentNonce, WrappedDEK: row.WrappedDek, WrappingNonce: row.WrappingNonce, SpeakerCount: int(row.SpeakerCount), WordCount: int(row.WordCount), DurationSeconds: row.DurationSeconds, ErrorMessage: textToString(row.ErrorMessage), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, MediaFilename: row.MediaFilename, MediaTitle: row.MediaTitle, MediaDescription: row.MediaDescription, MediaStatus: row.MediaStatus, VocabularyPacks: row.VocabularyPacks, ExpectedSpeakers: expectedSpeakersFromPg(row.ExpectedSpeakers)}
	if row.CompletedAt.Valid {
		t.CompletedAt = &row.CompletedAt.Time
	}
	if row.SpeechmaticsDeletedAt.Valid {
		t.SpeechmaticsDeletedAt = &row.SpeechmaticsDeletedAt.Time
	}
	return t
}

func transcriptionSearchRowToDomain(row sqlcdb.ListCompletedTranscriptionsForTranscriptSearchRow) *domain.Transcription {
	t := &domain.Transcription{ID: uuidToString(row.ID), OrganizationID: uuidToString(row.OrganizationID), MediaID: uuidToString(row.MediaID), Provider: row.Provider, SpeechmaticsJobID: textToString(row.SpeechmaticsJobID), Status: row.Status, Languages: row.Languages, Diarization: row.Diarization, EnhanceAudio: row.EnhanceAudio, PreprocessorUsed: textToString(row.PreprocessorUsed), ContentEncrypted: row.ContentEncrypted, ContentNonce: row.ContentNonce, WrappedDEK: row.WrappedDek, WrappingNonce: row.WrappingNonce, SpeakerCount: int(row.SpeakerCount), WordCount: int(row.WordCount), DurationSeconds: row.DurationSeconds, ErrorMessage: textToString(row.ErrorMessage), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, MediaFilename: row.MediaFilename, MediaTitle: row.MediaTitle, MediaDescription: row.MediaDescription, MediaStatus: row.MediaStatus, VocabularyPacks: row.VocabularyPacks, ExpectedSpeakers: expectedSpeakersFromPg(row.ExpectedSpeakers)}
	if row.CompletedAt.Valid {
		t.CompletedAt = &row.CompletedAt.Time
	}
	if row.SpeechmaticsDeletedAt.Valid {
		t.SpeechmaticsDeletedAt = &row.SpeechmaticsDeletedAt.Time
	}
	return t
}
func transcriptionRowToDomain(row sqlcdb.Transcription) *domain.Transcription {
	t := &domain.Transcription{ID: uuidToString(row.ID), OrganizationID: uuidToString(row.OrganizationID), MediaID: uuidToString(row.MediaID), Provider: row.Provider, SpeechmaticsJobID: textToString(row.SpeechmaticsJobID), Status: row.Status, Languages: row.Languages, Diarization: row.Diarization, EnhanceAudio: row.EnhanceAudio, PreprocessorUsed: textToString(row.PreprocessorUsed), ContentEncrypted: row.ContentEncrypted, ContentNonce: row.ContentNonce, WrappedDEK: row.WrappedDek, WrappingNonce: row.WrappingNonce, SpeakerCount: int(row.SpeakerCount), WordCount: int(row.WordCount), DurationSeconds: row.DurationSeconds, ErrorMessage: textToString(row.ErrorMessage), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, VocabularyPacks: row.VocabularyPacks, ExpectedSpeakers: expectedSpeakersFromPg(row.ExpectedSpeakers)}
	if row.CompletedAt.Valid {
		t.CompletedAt = &row.CompletedAt.Time
	}
	if row.SpeechmaticsDeletedAt.Valid {
		t.SpeechmaticsDeletedAt = &row.SpeechmaticsDeletedAt.Time
	}
	return t
}
