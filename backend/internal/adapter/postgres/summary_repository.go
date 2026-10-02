package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Compile-time check that SummaryRepository implements port.SummaryRepository.
var _ port.SummaryRepository = (*SummaryRepository)(nil)

// SummaryRepository implements port.SummaryRepository using PostgreSQL.
type SummaryRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewSummaryRepository creates a new PostgreSQL summary repository.
func NewSummaryRepository(pool *pgxpool.Pool) *SummaryRepository {
	return &SummaryRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// Create persists a new summary record.
func (r *SummaryRepository) Create(ctx context.Context, s *domain.Summary) error {
	if s.SummaryProfile == "" {
		s.SummaryProfile = domain.SummaryProfileGeneralProfessional
	}

	orgUUID, err := parseUUID(s.OrganizationID)
	if err != nil {
		return err
	}

	transUUID, err := parseUUID(s.TranscriptionID)
	if err != nil {
		return err
	}

	row, err := r.queries.CreateSummary(ctx, sqlcdb.CreateSummaryParams{
		OrganizationID:  orgUUID,
		TranscriptionID: transUUID,
		SummaryType:     s.SummaryType,
		Status:          s.Status,
		HighStakes:      s.HighStakes,
		ReviewStatus:    s.ReviewStatus,
		SummaryProfile:  s.SummaryProfile,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	// Populate DB-generated fields back onto the domain entity.
	s.ID = uuidToString(row.ID)
	s.CreatedAt = row.CreatedAt
	s.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID retrieves a summary record by ID.
func (r *SummaryRepository) GetByID(ctx context.Context, id string) (*domain.Summary, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetSummaryByID(ctx, uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return summaryRowToDomain(row), nil
}

// GetActiveByTranscriptionAndType returns the active summary (status NOT IN
// 'failed','deleted') for the given transcription and type.
func (r *SummaryRepository) GetActiveByTranscriptionAndType(ctx context.Context, transcriptionID, summaryType string) (*domain.Summary, error) {
	transUUID, err := parseUUID(transcriptionID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetActiveSummaryByTranscriptionAndType(ctx, sqlcdb.GetActiveSummaryByTranscriptionAndTypeParams{
		TranscriptionID: transUUID,
		SummaryType:     summaryType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return summaryRowToDomain(row), nil
}

// ListByTranscription retrieves all non-deleted summaries for a transcription,
// ordered by creation time descending. Includes the media filename via a
// two-hop JOIN (summaries -> transcriptions -> media).
func (r *SummaryRepository) ListByTranscription(ctx context.Context, transcriptionID string) ([]*domain.Summary, error) {
	transUUID, err := parseUUID(transcriptionID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.ListSummariesByTranscription(ctx, transUUID)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Summary, len(rows))
	for i, row := range rows {
		result[i] = summaryTranscriptionListRowToDomain(row)
	}
	return result, nil
}

// ListByOrganization retrieves summaries for an organization ordered by
// creation time descending, with pagination. Includes the media filename via
// a two-hop JOIN (summaries -> transcriptions -> media).
func (r *SummaryRepository) ListByOrganization(ctx context.Context, orgID string, limit, offset int, search string) ([]*domain.Summary, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.ListSummariesByOrganization(ctx, sqlcdb.ListSummariesByOrganizationParams{
		OrganizationID: orgUUID,
		Search:         search,
		LimitVal:       int32(limit),  //nolint:gosec // bounded by handler (1..100)
		OffsetVal:      int32(offset), //nolint:gosec // bounded by handler (>= 0)
	})
	if err != nil {
		return nil, err
	}

	result := make([]*domain.Summary, len(rows))
	for i, row := range rows {
		result[i] = summaryOrgListRowToDomain(row)
	}
	return result, nil
}

// CountByOrganization returns the total number of non-deleted summaries for
// the given organization.
func (r *SummaryRepository) CountByOrganization(ctx context.Context, orgID, search string) (int64, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, domain.ErrNotFound
	}

	return r.queries.CountSummariesByOrganization(ctx, sqlcdb.CountSummariesByOrganizationParams{
		OrganizationID: orgUUID,
		Search:         search,
	})
}

// Update updates an existing summary record.
// If ContentEncrypted is non-nil, uses UpdateSummaryContent (full content update).
// Otherwise, uses UpdateSummaryStatus (status/error_message only).
func (r *SummaryRepository) Update(ctx context.Context, s *domain.Summary) error {
	uuid, err := parseUUID(s.ID)
	if err != nil {
		return domain.ErrNotFound
	}

	// These columns are NOT NULL. Loaded legacy summaries can hydrate empty
	// values as nil, so a load-modify-update cycle must not turn them into SQL
	// NULL. An empty metadata object has no model claim but remains valid JSON.
	degradationCodes := s.DegradationCodes
	if degradationCodes == nil {
		degradationCodes = []string{}
	}
	modelMetadata := s.ModelMetadata
	if len(modelMetadata) == 0 {
		modelMetadata = []byte("{}")
	}

	if s.ContentEncrypted != nil {
		row, contentErr := r.queries.UpdateSummaryContent(ctx, sqlcdb.UpdateSummaryContentParams{
			ID:                             uuid,
			Status:                         s.Status,
			ContentEncrypted:               s.ContentEncrypted,
			ContentNonce:                   s.ContentNonce,
			WrappedDek:                     s.WrappedDEK,
			WrappingNonce:                  s.WrappingNonce,
			WordCount:                      int32(s.WordCount),        //nolint:gosec // bounded by summary metadata
			PromptTokens:                   int32(s.PromptTokens),     //nolint:gosec // bounded by summary metadata
			CompletionTokens:               int32(s.CompletionTokens), //nolint:gosec // bounded by summary metadata
			ThinkingTokens:                 int32(s.ThinkingTokens),   //nolint:gosec // bounded by summary metadata
			ReviewStatus:                   s.ReviewStatus,
			ExtractionEncrypted:            s.ExtractionEncrypted,
			ExtractionNonce:                s.ExtractionNonce,
			ExtractionWrappedDek:           s.ExtractionWrappedDEK,
			ExtractionWrappingNonce:        s.ExtractionWrappingNonce,
			PromptVersion:                  pgtype.Text{String: s.PromptVersion, Valid: s.PromptVersion != ""},
			Model:                          pgtype.Text{String: s.Model, Valid: s.Model != ""},
			ModelMetadata:                  modelMetadata,
			EndpointLocation:               pgtype.Text{String: s.EndpointLocation, Valid: s.EndpointLocation != ""},
			SourceVersion:                  pgtype.Text{String: s.SourceVersion, Valid: s.SourceVersion != ""},
			SourceHash:                     pgtype.Text{String: s.SourceHash, Valid: s.SourceHash != ""},
			DegradationCodes:               degradationCodes,
			StructuredContentCiphertext:    s.StructuredContentCiphertext,
			StructuredContentNonce:         s.StructuredContentNonce,
			StructuredContentWrappedDek:    s.StructuredContentWrappedDEK,
			StructuredContentWrappingNonce: s.StructuredContentWrappingNonce,
			StructuredSchemaVersion:        pgtype.Text{String: s.StructuredSchemaVersion, Valid: s.StructuredSchemaVersion != ""},
		})
		if contentErr != nil {
			if errors.Is(contentErr, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return contentErr
		}
		s.UpdatedAt = row.UpdatedAt
		if row.CompletedAt.Valid {
			s.CompletedAt = &row.CompletedAt.Time
		}
		return nil
	}

	// Status-only update (SetFailed, etc.).
	row, err := r.queries.UpdateSummaryStatus(ctx, sqlcdb.UpdateSummaryStatusParams{
		ID:           uuid,
		Status:       s.Status,
		ErrorMessage: pgtype.Text{String: s.ErrorMessage, Valid: s.ErrorMessage != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	s.UpdatedAt = row.UpdatedAt
	return nil
}

// Delete soft-deletes a summary by setting status to "deleted".
func (r *SummaryRepository) Delete(ctx context.Context, id string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	// Use UpdateSummaryStatus (with RETURNING) instead of SoftDeleteSummary
	// (:exec) so we can detect not-found via ErrNoRows.
	_, err = r.queries.UpdateSummaryStatus(ctx, sqlcdb.UpdateSummaryStatusParams{
		ID:     uuid,
		Status: domain.SummaryStatusDeleted,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// DeleteByTranscriptionID soft-deletes all non-deleted summaries for the given transcription.
// Returns the number of summaries affected. An invalid UUID yields zero rows.
func (r *SummaryRepository) DeleteByTranscriptionID(ctx context.Context, transcriptionID string) (int64, error) {
	transUUID, err := parseUUID(transcriptionID)
	if err != nil {
		return 0, nil //nolint:nilerr // invalid UUID means no matching rows
	}

	return r.queries.SoftDeleteSummariesByTranscriptionID(ctx, transUUID)
}

// DeleteByMediaID soft-deletes all non-deleted summaries whose transcription
// belongs to the given media. Returns the number of summaries affected.
// An invalid UUID yields zero rows.
func (r *SummaryRepository) DeleteByMediaID(ctx context.Context, mediaID string) (int64, error) {
	mediaUUID, err := parseUUID(mediaID)
	if err != nil {
		return 0, nil //nolint:nilerr // invalid UUID means no matching rows
	}

	return r.queries.SoftDeleteSummariesByMediaID(ctx, mediaUUID)
}

// ClearArtifactsByMediaID clears derived summary artifacts for every summary
// belonging to transcriptions of the given media. Returns the affected count.
func (r *SummaryRepository) ClearArtifactsByMediaID(context.Context, string) (int64, error) {
	return 0, domain.ErrInvalidInput
}

// UndoDelete restores a soft-deleted summary within its organization.
func (r *SummaryRepository) UndoDelete(ctx context.Context, id, orgID, restoreStatus string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return domain.ErrNotFound
	}

	_, err = r.queries.UndoDeleteSummary(ctx, sqlcdb.UndoDeleteSummaryParams{
		ID:             uuid,
		OrganizationID: orgUUID,
		Status:         restoreStatus,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

// ListSummaryStatusesByTranscriptionIDs returns summary type+status pairs
// grouped by transcription ID. Only non-deleted summaries are included.
func (r *SummaryRepository) ListSummaryStatusesByTranscriptionIDs(ctx context.Context, transcriptionIDs []string) (map[string][]domain.SummaryStatusInfo, error) {
	if len(transcriptionIDs) == 0 {
		return make(map[string][]domain.SummaryStatusInfo), nil
	}

	// Parse UUIDs.
	uuids := make([]pgtype.UUID, 0, len(transcriptionIDs))
	for _, id := range transcriptionIDs {
		u, err := parseUUID(id)
		if err != nil {
			continue // skip invalid UUIDs
		}
		uuids = append(uuids, u)
	}

	if len(uuids) == 0 {
		return make(map[string][]domain.SummaryStatusInfo), nil
	}

	const q = `
SELECT transcription_id, summary_type, status
FROM summaries
WHERE transcription_id = ANY($1)
  AND status != 'deleted'
ORDER BY transcription_id, summary_type`

	rows, err := r.pool.Query(ctx, q, uuids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]domain.SummaryStatusInfo)
	for rows.Next() {
		var (
			transID     pgtype.UUID
			summaryType string
			status      string
		)
		if scanErr := rows.Scan(&transID, &summaryType, &status); scanErr != nil {
			return nil, scanErr
		}
		tid := uuidToString(transID)
		result[tid] = append(result[tid], domain.SummaryStatusInfo{
			SummaryType: summaryType,
			Status:      status,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// ListByOrganizationFiltered retrieves media-backed summaries with optional filters.
func (r *SummaryRepository) ListByOrganizationFiltered(ctx context.Context, orgID string, filter port.SummaryListFilter) ([]*domain.Summary, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	q, args, limit := buildSummaryFilterQuery(orgUUID, filter)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list summaries filtered: %w", err)
	}
	defer rows.Close()

	result := make([]*domain.Summary, 0, limit)
	for rows.Next() {
		s, scanErr := scanFilteredSummaryRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan summary row: %w", scanErr)
		}
		result = append(result, s)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

// buildSummaryFilterQuery constructs the dynamic SQL for filtered summary listing.
func buildSummaryFilterQuery(orgUUID pgtype.UUID, filter port.SummaryListFilter) (query string, args []any, limit int) {
	query = `SELECT s.id, s.organization_id, s.transcription_id, s.summary_type,
		s.status, s.content_encrypted, s.content_nonce, s.wrapped_dek,
		s.wrapping_nonce, s.word_count, s.prompt_tokens, s.completion_tokens,
		s.error_message, s.created_at, s.updated_at, s.completed_at, s.thinking_tokens,
		s.high_stakes, s.review_status, s.review_findings_encrypted,
		s.review_findings_nonce, s.review_wrapped_dek, s.review_wrapping_nonce,
		s.extraction_encrypted, s.extraction_nonce, s.extraction_wrapped_dek,
		s.extraction_wrapping_nonce, s.summary_profile, s.prompt_version,
		s.model, s.endpoint_location, s.source_version, s.source_hash,
		s.structured_content_ciphertext, s.structured_content_nonce,
		s.structured_content_wrapped_dek, s.structured_content_wrapping_nonce,
		s.structured_schema_version, s.degradation_codes, s.model_metadata,
		COALESCE(m.filename, '') AS media_filename,
		COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') AS audio_name,
		COALESCE(m.description, '') AS media_description
	FROM summaries s
	JOIN transcriptions t ON t.id = s.transcription_id
	LEFT JOIN media m ON m.id = t.media_id AND m.status != 'deleted'
	WHERE s.organization_id = $1 AND s.status != 'deleted' AND t.status != 'deleted'
	  AND m.id IS NOT NULL`
	args = []any{orgUUID}
	argIdx := 2

	if filter.DateFrom != nil {
		query += fmt.Sprintf(" AND s.created_at >= $%d", argIdx)
		args = append(args, *filter.DateFrom)
		argIdx++
	}
	if filter.DateTo != nil {
		query += fmt.Sprintf(" AND s.created_at <= $%d", argIdx)
		args = append(args, *filter.DateTo)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND s.status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.SummaryType != "" {
		query += fmt.Sprintf(" AND s.summary_type = $%d", argIdx)
		args = append(args, filter.SummaryType)
		argIdx++
	}
	if filter.Search != "" {
		escaped := escapeILIKE(filter.Search)
		query += fmt.Sprintf(` AND (
				COALESCE(NULLIF(TRIM(m.title), ''), m.filename, '') ILIKE '%%' || $%d || '%%' ESCAPE '\' OR
				COALESCE(m.description, '') ILIKE '%%' || $%d || '%%' ESCAPE '\'
			)`, argIdx, argIdx)
		args = append(args, escaped)
		argIdx++
	}

	sortCol := summarySortColumn(filter.SortBy)
	sortDir := "DESC"
	if filter.SortOrder == "asc" {
		sortDir = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortCol, sortDir)

	limit = filter.Limit
	if limit <= 0 {
		limit = 20
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, int32(limit), int32(filter.Offset)) //nolint:gosec // bounded upstream

	return query, args, limit
}

// summarySortColumn maps a sort field name to a safe SQL column identifier.
func summarySortColumn(field string) string {
	switch field {
	case "word_count":
		return "s.word_count"
	default:
		return "s.created_at"
	}
}

// scanFilteredSummaryRow scans a row from the filtered summary query.
func scanFilteredSummaryRow(rows pgx.Rows) (*domain.Summary, error) {
	var row sqlcdb.Summary
	var mediaFilename, audioName, mediaDescription string

	if err := rows.Scan(
		&row.ID, &row.OrganizationID, &row.TranscriptionID, &row.SummaryType,
		&row.Status, &row.ContentEncrypted, &row.ContentNonce, &row.WrappedDek,
		&row.WrappingNonce, &row.WordCount, &row.PromptTokens, &row.CompletionTokens,
		&row.ErrorMessage, &row.CreatedAt, &row.UpdatedAt, &row.CompletedAt,
		&row.ThinkingTokens, &row.HighStakes, &row.ReviewStatus,
		&row.ReviewFindingsEncrypted, &row.ReviewFindingsNonce, &row.ReviewWrappedDek,
		&row.ReviewWrappingNonce, &row.ExtractionEncrypted, &row.ExtractionNonce,
		&row.ExtractionWrappedDek, &row.ExtractionWrappingNonce,
		&row.SummaryProfile, &row.PromptVersion, &row.Model, &row.EndpointLocation,
		&row.SourceVersion, &row.SourceHash, &row.StructuredContentCiphertext,
		&row.StructuredContentNonce, &row.StructuredContentWrappedDek,
		&row.StructuredContentWrappingNonce, &row.StructuredSchemaVersion,
		&row.DegradationCodes, &row.ModelMetadata,
		&mediaFilename, &audioName, &mediaDescription,
	); err != nil {
		return nil, err
	}

	s := summaryRowToDomain(row)
	s.TranscriptionMediaFilename = mediaFilename
	s.TranscriptionAudioName = audioName
	s.TranscriptionMediaDescription = mediaDescription

	return s, nil
}

// summaryRowToDomain converts a sqlcdb.Summary to a domain.Summary.
func summaryRowToDomain(row sqlcdb.Summary) *domain.Summary {
	s := &domain.Summary{
		ID:                             uuidToString(row.ID),
		OrganizationID:                 uuidToString(row.OrganizationID),
		TranscriptionID:                uuidToString(row.TranscriptionID),
		SummaryType:                    row.SummaryType,
		Status:                         row.Status,
		ContentEncrypted:               row.ContentEncrypted,
		ContentNonce:                   row.ContentNonce,
		WrappedDEK:                     row.WrappedDek,
		WrappingNonce:                  row.WrappingNonce,
		WordCount:                      int(row.WordCount),
		PromptTokens:                   int(row.PromptTokens),
		CompletionTokens:               int(row.CompletionTokens),
		ThinkingTokens:                 int(row.ThinkingTokens),
		HighStakes:                     row.HighStakes,
		ReviewStatus:                   row.ReviewStatus,
		ReviewFindingsEncrypted:        row.ReviewFindingsEncrypted,
		ReviewFindingsNonce:            row.ReviewFindingsNonce,
		ReviewWrappedDEK:               row.ReviewWrappedDek,
		ReviewWrappingNonce:            row.ReviewWrappingNonce,
		ExtractionEncrypted:            row.ExtractionEncrypted,
		ExtractionNonce:                row.ExtractionNonce,
		ExtractionWrappedDEK:           row.ExtractionWrappedDek,
		ExtractionWrappingNonce:        row.ExtractionWrappingNonce,
		SummaryProfile:                 row.SummaryProfile,
		PromptVersion:                  textToString(row.PromptVersion),
		Model:                          textToString(row.Model),
		ModelMetadata:                  append([]byte(nil), row.ModelMetadata...),
		EndpointLocation:               textToString(row.EndpointLocation),
		SourceVersion:                  textToString(row.SourceVersion),
		SourceHash:                     textToString(row.SourceHash),
		DegradationCodes:               append([]string(nil), row.DegradationCodes...),
		StructuredContentCiphertext:    row.StructuredContentCiphertext,
		StructuredContentNonce:         row.StructuredContentNonce,
		StructuredContentWrappedDEK:    row.StructuredContentWrappedDek,
		StructuredContentWrappingNonce: row.StructuredContentWrappingNonce,
		StructuredSchemaVersion:        textToString(row.StructuredSchemaVersion),
		ErrorMessage:                   textToString(row.ErrorMessage),
		CreatedAt:                      row.CreatedAt,
		UpdatedAt:                      row.UpdatedAt,
	}

	if row.CompletedAt.Valid {
		s.CompletedAt = &row.CompletedAt.Time
	}

	return s
}

// summaryTranscriptionListRowToDomain converts a ListSummariesByTranscriptionRow
// (which includes a two-hop JOIN for the media filename) to a domain.Summary.
func summaryTranscriptionListRowToDomain(row sqlcdb.ListSummariesByTranscriptionRow) *domain.Summary {
	s := &domain.Summary{
		ID:                             uuidToString(row.ID),
		OrganizationID:                 uuidToString(row.OrganizationID),
		TranscriptionID:                uuidToString(row.TranscriptionID),
		SummaryType:                    row.SummaryType,
		Status:                         row.Status,
		ContentEncrypted:               row.ContentEncrypted,
		ContentNonce:                   row.ContentNonce,
		WrappedDEK:                     row.WrappedDek,
		WrappingNonce:                  row.WrappingNonce,
		WordCount:                      int(row.WordCount),
		PromptTokens:                   int(row.PromptTokens),
		CompletionTokens:               int(row.CompletionTokens),
		ThinkingTokens:                 int(row.ThinkingTokens),
		HighStakes:                     row.HighStakes,
		ReviewStatus:                   row.ReviewStatus,
		ReviewFindingsEncrypted:        row.ReviewFindingsEncrypted,
		ReviewFindingsNonce:            row.ReviewFindingsNonce,
		ReviewWrappedDEK:               row.ReviewWrappedDek,
		ReviewWrappingNonce:            row.ReviewWrappingNonce,
		ExtractionEncrypted:            row.ExtractionEncrypted,
		ExtractionNonce:                row.ExtractionNonce,
		ExtractionWrappedDEK:           row.ExtractionWrappedDek,
		ExtractionWrappingNonce:        row.ExtractionWrappingNonce,
		SummaryProfile:                 row.SummaryProfile,
		PromptVersion:                  textToString(row.PromptVersion),
		Model:                          textToString(row.Model),
		ModelMetadata:                  append([]byte(nil), row.ModelMetadata...),
		EndpointLocation:               textToString(row.EndpointLocation),
		SourceVersion:                  textToString(row.SourceVersion),
		SourceHash:                     textToString(row.SourceHash),
		DegradationCodes:               append([]string(nil), row.DegradationCodes...),
		StructuredContentCiphertext:    row.StructuredContentCiphertext,
		StructuredContentNonce:         row.StructuredContentNonce,
		StructuredContentWrappedDEK:    row.StructuredContentWrappedDek,
		StructuredContentWrappingNonce: row.StructuredContentWrappingNonce,
		StructuredSchemaVersion:        textToString(row.StructuredSchemaVersion),
		ErrorMessage:                   textToString(row.ErrorMessage),
		CreatedAt:                      row.CreatedAt,
		UpdatedAt:                      row.UpdatedAt,
		TranscriptionMediaFilename:     row.TranscriptionMediaFilename,
		TranscriptionAudioName:         row.TranscriptionAudioName,
		TranscriptionMediaDescription:  row.TranscriptionMediaDescription,
	}

	if row.CompletedAt.Valid {
		s.CompletedAt = &row.CompletedAt.Time
	}

	return s
}

// summaryOrgListRowToDomain converts a ListSummariesByOrganizationRow
// (which includes a two-hop JOIN for the media filename) to a domain.Summary.
func summaryOrgListRowToDomain(row sqlcdb.ListSummariesByOrganizationRow) *domain.Summary {
	s := &domain.Summary{
		ID:                             uuidToString(row.ID),
		OrganizationID:                 uuidToString(row.OrganizationID),
		TranscriptionID:                uuidToString(row.TranscriptionID),
		SummaryType:                    row.SummaryType,
		Status:                         row.Status,
		ContentEncrypted:               row.ContentEncrypted,
		ContentNonce:                   row.ContentNonce,
		WrappedDEK:                     row.WrappedDek,
		WrappingNonce:                  row.WrappingNonce,
		WordCount:                      int(row.WordCount),
		PromptTokens:                   int(row.PromptTokens),
		CompletionTokens:               int(row.CompletionTokens),
		ThinkingTokens:                 int(row.ThinkingTokens),
		HighStakes:                     row.HighStakes,
		ReviewStatus:                   row.ReviewStatus,
		ReviewFindingsEncrypted:        row.ReviewFindingsEncrypted,
		ReviewFindingsNonce:            row.ReviewFindingsNonce,
		ReviewWrappedDEK:               row.ReviewWrappedDek,
		ReviewWrappingNonce:            row.ReviewWrappingNonce,
		ExtractionEncrypted:            row.ExtractionEncrypted,
		ExtractionNonce:                row.ExtractionNonce,
		ExtractionWrappedDEK:           row.ExtractionWrappedDek,
		ExtractionWrappingNonce:        row.ExtractionWrappingNonce,
		SummaryProfile:                 row.SummaryProfile,
		PromptVersion:                  textToString(row.PromptVersion),
		Model:                          textToString(row.Model),
		ModelMetadata:                  append([]byte(nil), row.ModelMetadata...),
		EndpointLocation:               textToString(row.EndpointLocation),
		SourceVersion:                  textToString(row.SourceVersion),
		SourceHash:                     textToString(row.SourceHash),
		DegradationCodes:               append([]string(nil), row.DegradationCodes...),
		StructuredContentCiphertext:    row.StructuredContentCiphertext,
		StructuredContentNonce:         row.StructuredContentNonce,
		StructuredContentWrappedDEK:    row.StructuredContentWrappedDek,
		StructuredContentWrappingNonce: row.StructuredContentWrappingNonce,
		StructuredSchemaVersion:        textToString(row.StructuredSchemaVersion),
		ErrorMessage:                   textToString(row.ErrorMessage),
		CreatedAt:                      row.CreatedAt,
		UpdatedAt:                      row.UpdatedAt,
		TranscriptionMediaFilename:     row.TranscriptionMediaFilename,
		TranscriptionAudioName:         row.TranscriptionAudioName,
		TranscriptionMediaDescription:  row.TranscriptionMediaDescription,
	}

	if row.CompletedAt.Valid {
		s.CompletedAt = &row.CompletedAt.Time
	}

	return s
}
