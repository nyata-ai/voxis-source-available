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

// Compile-time interface checks.
var _ port.RecordingRepository = (*RecordingRepository)(nil)
var _ port.ChunkRepository = (*ChunkRepository)(nil)
var _ port.RecordingSessionLocker = (*RecordingRepository)(nil)
var _ port.RecordingSessionTryLocker = (*RecordingRepository)(nil)
var _ port.RecordingCompleter = (*RecordingRepository)(nil)
var _ port.RecordingChunkCleanupLister = (*RecordingRepository)(nil)
var _ port.RecordingChunkSizer = (*ChunkRepository)(nil)

// RecordingRepository implements port.RecordingRepository using PostgreSQL.
type RecordingRepository struct {
	pool      *pgxpool.Pool
	queries   *sqlcdb.Queries
	lockSlots chan struct{}
}

// NewRecordingRepository creates a new PostgreSQL recording repository.
func NewRecordingRepository(pool *pgxpool.Pool) *RecordingRepository {
	return &RecordingRepository{
		pool:      pool,
		queries:   sqlcdb.New(pool),
		lockSlots: make(chan struct{}, max(1, int(pool.Config().MaxConns)/4)),
	}
}

// Create persists a new recording session.
func (r *RecordingRepository) Create(ctx context.Context, session *domain.RecordingSession) error {
	orgUUID, err := parseUUID(session.OrganizationID)
	if err != nil {
		return err
	}

	row, err := r.queries.CreateRecordingSession(ctx, sqlcdb.CreateRecordingSessionParams{
		OrganizationID:  orgUUID,
		UserID:          session.UserID,
		Status:          session.Status,
		MimeType:        session.MimeType,
		MicrophoneLabel: pgtype.Text{String: session.MicrophoneLabel, Valid: session.MicrophoneLabel != ""},
		LastActivityAt:  time.Now(),
		CaptureSource:   string(session.CaptureSource),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	session.ID = uuidToString(row.ID)
	session.CreatedAt = row.CreatedAt
	session.UpdatedAt = row.UpdatedAt
	t := row.LastActivityAt
	session.LastActivityAt = &t
	return nil
}

// GetByID retrieves a recording session by ID.
func (r *RecordingRepository) GetByID(ctx context.Context, id string) (*domain.RecordingSession, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetRecordingSessionByID(ctx, uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return sessionRowToDomain(row), nil
}

// GetActiveByUserID retrieves the user's active session.
func (r *RecordingRepository) GetActiveByUserID(ctx context.Context, userID string) (*domain.RecordingSession, error) {
	row, err := r.queries.GetActiveRecordingByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return sessionRowToDomain(row), nil
}

// GetInterruptedByUserID retrieves all interrupted sessions for a user.
func (r *RecordingRepository) GetInterruptedByUserID(ctx context.Context, userID string) ([]domain.RecordingSession, error) {
	rows, err := r.queries.GetInterruptedRecordingsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]domain.RecordingSession, len(rows))
	for i, row := range rows {
		s := sessionRowToDomain(row)
		result[i] = *s
	}
	return result, nil
}

// CountInterruptedByUserID returns the number of interrupted sessions.
func (r *RecordingRepository) CountInterruptedByUserID(ctx context.Context, userID string) (int, error) {
	count, err := r.queries.CountInterruptedRecordingsByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// UpdateStatus atomically transitions a session status.
func (r *RecordingRepository) UpdateStatus(ctx context.Context, id, fromStatus, toStatus string) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	rows, err := r.queries.UpdateRecordingSessionStatus(ctx, sqlcdb.UpdateRecordingSessionStatusParams{
		ID:        uuid,
		OldStatus: fromStatus,
		NewStatus: toStatus,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrConflict
	}
	return nil
}

// WithSessionLock serializes all recording mutations for one session across
// API instances. The advisory lock belongs to the acquired PostgreSQL session,
// while fn may safely use the normal repository pool on another connection.
func (r *RecordingRepository) WithSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	return r.withSessionLock(ctx, sessionID, fn, false)
}

// TryWithSessionLock rejects contention without leaving HTTP requests queued
// behind a long stitch: ErrConflict when this session's lock is held elsewhere,
// ErrRateLimited when the process has no free lock slot (slots also bound
// locks on different sessions).
func (r *RecordingRepository) TryWithSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	return r.withSessionLock(ctx, sessionID, fn, true)
}

func (r *RecordingRepository) withSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error, try bool) error {
	if _, parseErr := parseUUID(sessionID); parseErr != nil {
		return domain.ErrNotFound
	}
	if fn == nil {
		return fmt.Errorf("session lock callback is required")
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if try {
		select {
		case r.lockSlots <- struct{}{}:
		default:
			// Capacity, not contention: nothing is wrong with this session, the
			// process is simply out of lock slots. Callers surface it as a
			// retryable 429, never as the permanent 409 a busy session gets.
			return fmt.Errorf("recording lock capacity exhausted: %w", domain.ErrRateLimited)
		}
	} else {
		select {
		case r.lockSlots <- struct{}{}:
		case <-waitCtx.Done():
			return waitCtx.Err()
		}
	}
	defer func() { <-r.lockSlots }()
	conn, err := r.pool.Acquire(waitCtx)
	if err != nil {
		return fmt.Errorf("acquire recording lock connection: %w", err)
	}
	defer conn.Release()

	// Reset session settings and release every lock before returning the connection.
	// A failed cleanup discards it, never returning a locked session to the pool.
	defer releaseRecordingLock(conn)
	if _, err = conn.Exec(waitCtx, "SET lock_timeout = '5s'"); err != nil {
		return err
	}
	if try {
		var locked bool
		if scanErr := conn.QueryRow(waitCtx, "SELECT pg_try_advisory_lock(hashtextextended($1, 0))", sessionID).Scan(&locked); scanErr != nil {
			return scanErr
		}
		if !locked {
			return domain.ErrConflict
		}
	} else if _, err = conn.Exec(waitCtx, "SELECT pg_advisory_lock(hashtextextended($1, 0))", sessionID); err != nil {
		return fmt.Errorf("acquire recording session lock: %w", err)
	}
	return fn(ctx)
}

func releaseRecordingLock(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock_all(); RESET lock_timeout"); err != nil {
		_ = conn.Conn().Close(ctx) //nolint:errcheck // Close marks the session closed even if graceful termination fails
	}
}

// SetMediaID stores the resulting media ID on a session.
func (r *RecordingRepository) SetMediaID(ctx context.Context, id, mediaID string) error {
	sessionUUID, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	mediaUUID, err := parseUUID(mediaID)
	if err != nil {
		return domain.ErrNotFound
	}

	return r.queries.SetRecordingMediaID(ctx, sqlcdb.SetRecordingMediaIDParams{
		ID:      sessionUUID,
		MediaID: mediaUUID,
	})
}

// UpdateLastChunkAt updates the last chunk timestamp.
func (r *RecordingRepository) UpdateLastChunkAt(ctx context.Context, id string, t time.Time) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	return r.queries.UpdateRecordingLastChunkAt(ctx, sqlcdb.UpdateRecordingLastChunkAtParams{
		ID:          uuid,
		LastChunkAt: pgtype.Timestamptz{Time: t, Valid: true},
	})
}

// UpdateLastActivityAt updates the last activity timestamp.
func (r *RecordingRepository) UpdateLastActivityAt(ctx context.Context, id string, t time.Time) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	return r.queries.UpdateRecordingLastActivityAt(ctx, sqlcdb.UpdateRecordingLastActivityAtParams{
		ID:             uuid,
		LastActivityAt: t,
	})
}

// FindStaleRecording finds sessions in an active status ('recording' or
// 'paused') past the threshold.
func (r *RecordingRepository) FindStaleRecording(ctx context.Context, threshold time.Duration) ([]domain.RecordingSession, error) {
	thresholdTime := time.Now().Add(-threshold)

	rows, err := r.queries.FindStaleRecordingSessions(ctx, thresholdTime)
	if err != nil {
		return nil, err
	}

	result := make([]domain.RecordingSession, len(rows))
	for i, row := range rows {
		s := sessionRowToDomain(row)
		result[i] = *s
	}
	return result, nil
}

// ListByStatusOlderThan returns sessions in the given status older than threshold.
func (r *RecordingRepository) ListByStatusOlderThan(ctx context.Context, status string, threshold time.Time) ([]domain.RecordingSession, error) {
	rows, err := r.queries.ListRecordingSessionsByStatusOlderThan(ctx, sqlcdb.ListRecordingSessionsByStatusOlderThanParams{
		Status:        status,
		ThresholdTime: threshold,
	})
	if err != nil {
		return nil, err
	}

	result := make([]domain.RecordingSession, len(rows))
	for i, row := range rows {
		s := sessionRowToDomain(row)
		result[i] = *s
	}
	return result, nil
}

// ListWithChunksByStatusOlderThan returns sessions still holding a chunk
// manifest, so cleanup retries never discard the only decryption metadata.
func (r *RecordingRepository) ListWithChunksByStatusOlderThan(ctx context.Context, status string, threshold time.Time, limit int) ([]domain.RecordingSession, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.queries.ListRecordingSessionsWithChunksByStatusOlderThan(ctx, sqlcdb.ListRecordingSessionsWithChunksByStatusOlderThanParams{
		Status:        status,
		ThresholdTime: threshold,
		LimitVal:      int32(limit), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}

	result := make([]domain.RecordingSession, len(rows))
	for i, row := range rows {
		result[i] = *sessionRowToDomain(row)
	}
	return result, nil
}

// SetCompletedAt sets completed_at and total_duration on a session.
func (r *RecordingRepository) SetCompletedAt(ctx context.Context, id string, completedAt time.Time, totalDuration float64) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}

	return r.queries.SetRecordingCompletedAt(ctx, sqlcdb.SetRecordingCompletedAtParams{
		ID:            uuid,
		CompletedAt:   pgtype.Timestamptz{Time: completedAt, Valid: true},
		TotalDuration: totalDuration,
	})
}

// Complete atomically records final metadata and transitions completing -> completed.
func (r *RecordingRepository) Complete(ctx context.Context, id string, completedAt time.Time, totalDuration float64) error {
	uuid, err := parseUUID(id)
	if err != nil {
		return domain.ErrNotFound
	}
	rows, err := r.queries.CompleteRecordingSession(ctx, sqlcdb.CompleteRecordingSessionParams{
		ID:            uuid,
		CompletedAt:   pgtype.Timestamptz{Time: completedAt, Valid: true},
		TotalDuration: totalDuration,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrConflict
	}
	return nil
}

// ListLiveRecordingAudioRetentionCandidates returns completed live recordings due for audio deletion.
func (r *RecordingRepository) ListLiveRecordingAudioRetentionCandidates(ctx context.Context, now time.Time, limit int) ([]port.LiveRecordingAudioRetentionCandidate, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.queries.ListLiveRecordingAudioRetentionCandidates(ctx, sqlcdb.ListLiveRecordingAudioRetentionCandidatesParams{
		NowTime:  now,
		LimitVal: int32(limit), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	candidates := make([]port.LiveRecordingAudioRetentionCandidate, 0, len(rows))
	for _, row := range rows {
		candidate := port.LiveRecordingAudioRetentionCandidate{
			SessionID:      uuidToString(row.SessionID),
			MediaID:        uuidToString(row.MediaID),
			OrganizationID: uuidToString(row.OrganizationID),
			StorageKey:     textToString(row.StorageKey),
		}
		if row.CompletedAt.Valid {
			candidate.CompletedAt = row.CompletedAt.Time
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

// ListActiveActivity returns recording sessions active in the activity feed.
func (r *RecordingRepository) ListActiveActivity(ctx context.Context, userID string) ([]domain.RecordingActivityRow, error) {
	const q = `
SELECT rs.id, rs.status, COALESCE(rs.media_id::text, ''),
       COALESCE(rs.microphone_label, ''),
       COALESCE(m.status, ''), COALESCE(m.scan_status, ''),
       COALESCE(m.title, ''), COALESCE(m.filename, ''),
       rs.created_at, rs.updated_at
FROM recording_sessions rs
LEFT JOIN media m ON rs.media_id = m.id
WHERE rs.user_id = $1
  AND (
    -- Failed sessions stay visible for the full 48h failed-chunk retention
    -- window so the user can still see (and act on) a lost recording before its
    -- chunks are crypto-deleted.
    (rs.status = 'failed' AND rs.updated_at > NOW() - INTERVAL '48 hours')
    OR (
      rs.updated_at > NOW() - INTERVAL '2 hours'
      AND (
        rs.status = 'completing'
        OR (rs.status = 'completed' AND rs.updated_at > NOW() - INTERVAL '5 minutes')
        OR (rs.status = 'completed' AND m.scan_status = 'scan_pending')
      )
    )
  )
ORDER BY rs.updated_at DESC
LIMIT 20`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.RecordingActivityRow, 0, 8)
	for rows.Next() {
		var row domain.RecordingActivityRow
		var sessID pgtype.UUID
		var created, updated pgtype.Timestamptz
		if scanErr := rows.Scan(&sessID, &row.Status, &row.MediaID, &row.MicrophoneLabel,
			&row.MediaStatus, &row.MediaScanStatus, &row.MediaTitle, &row.MediaFilename,
			&created, &updated); scanErr != nil {
			return nil, scanErr
		}
		row.SessionID = uuidToString(sessID)
		row.StartedAt = created.Time
		row.UpdatedAt = updated.Time
		out = append(out, row)
	}
	return out, rows.Err()
}

// sessionRowToDomain converts a sqlcdb.RecordingSession to a domain entity.
func sessionRowToDomain(row sqlcdb.RecordingSession) *domain.RecordingSession {
	s := &domain.RecordingSession{
		ID:              uuidToString(row.ID),
		OrganizationID:  uuidToString(row.OrganizationID),
		UserID:          row.UserID,
		Status:          row.Status,
		MimeType:        row.MimeType,
		MicrophoneLabel: textToString(row.MicrophoneLabel),
		MediaID:         uuidToString(row.MediaID),
		TotalDuration:   row.TotalDuration,
		RecordingMode:   domain.RecordingModeRegular,
		CaptureSource:   domain.RecordingCaptureSource(row.CaptureSource),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if row.LastChunkAt.Valid {
		t := row.LastChunkAt.Time
		s.LastChunkAt = &t
	}
	t := row.LastActivityAt
	s.LastActivityAt = &t
	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time
		s.CompletedAt = &t
	}
	return s
}

// ChunkRepository implements port.ChunkRepository using PostgreSQL.
type ChunkRepository struct {
	pool    *pgxpool.Pool
	queries *sqlcdb.Queries
}

// NewChunkRepository creates a new PostgreSQL chunk repository.
func NewChunkRepository(pool *pgxpool.Pool) *ChunkRepository {
	return &ChunkRepository{
		pool:    pool,
		queries: sqlcdb.New(pool),
	}
}

// Create persists a new chunk manifest row.
func (r *ChunkRepository) Create(ctx context.Context, chunk *domain.RecordingChunk) error {
	sessionUUID, err := parseUUID(chunk.SessionID)
	if err != nil {
		return err
	}

	row, err := r.queries.CreateRecordingChunk(ctx, sqlcdb.CreateRecordingChunkParams{
		SessionID:      sessionUUID,
		Seq:            int32(chunk.Seq), //nolint:gosec // bounded by MaxChunksPerSession (500)
		StorageKey:     chunk.StorageKey,
		WrappedDek:     chunk.WrappedDEK,
		WrappingNonce:  chunk.WrappingNonce,
		EncryptionAlgo: chunk.EncryptionAlgo,
		ChunkSize:      int32(chunk.ChunkSize),  //nolint:gosec // bounded by encryption chunk size
		ChunkCount:     int32(chunk.ChunkCount), //nolint:gosec // bounded by file/chunk ratio
		PlaintextSize:  chunk.PlaintextSize,
		Checksum:       chunk.Checksum,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	chunk.ID = uuidToString(row.ID)
	chunk.UploadedAt = row.UploadedAt
	return nil
}

// Upsert creates or replaces a chunk manifest row (idempotent retry).
func (r *ChunkRepository) Upsert(ctx context.Context, chunk *domain.RecordingChunk) error {
	sessionUUID, err := parseUUID(chunk.SessionID)
	if err != nil {
		return err
	}

	row, err := r.queries.UpsertRecordingChunk(ctx, sqlcdb.UpsertRecordingChunkParams{
		SessionID:      sessionUUID,
		Seq:            int32(chunk.Seq), //nolint:gosec // bounded by MaxChunksPerSession (500)
		StorageKey:     chunk.StorageKey,
		WrappedDek:     chunk.WrappedDEK,
		WrappingNonce:  chunk.WrappingNonce,
		EncryptionAlgo: chunk.EncryptionAlgo,
		ChunkSize:      int32(chunk.ChunkSize),  //nolint:gosec // bounded by encryption chunk size
		ChunkCount:     int32(chunk.ChunkCount), //nolint:gosec // bounded by file/chunk ratio
		PlaintextSize:  chunk.PlaintextSize,
		Checksum:       chunk.Checksum,
	})
	if err != nil {
		return err
	}

	chunk.ID = uuidToString(row.ID)
	chunk.UploadedAt = row.UploadedAt
	return nil
}

// GetBySessionAndSeq retrieves a chunk by session ID and sequence number.
func (r *ChunkRepository) GetBySessionAndSeq(ctx context.Context, sessionID string, seq int) (*domain.RecordingChunk, error) {
	uuid, err := parseUUID(sessionID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	row, err := r.queries.GetRecordingChunkBySessionAndSeq(ctx, sqlcdb.GetRecordingChunkBySessionAndSeqParams{
		SessionID: uuid,
		Seq:       int32(seq), //nolint:gosec // bounded by MaxChunksPerSession (500)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return chunkRowToDomain(row), nil
}

// ListBySession retrieves all chunks for a session ordered by seq ASC.
func (r *ChunkRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.RecordingChunk, error) {
	uuid, err := parseUUID(sessionID)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	rows, err := r.queries.ListRecordingChunksBySession(ctx, uuid)
	if err != nil {
		return nil, err
	}

	result := make([]domain.RecordingChunk, len(rows))
	for i, row := range rows {
		c := chunkRowToDomain(row)
		result[i] = *c
	}
	return result, nil
}

// CountBySession returns the number of chunks for a session.
func (r *ChunkRepository) CountBySession(ctx context.Context, sessionID string) (int, error) {
	uuid, err := parseUUID(sessionID)
	if err != nil {
		return 0, domain.ErrNotFound
	}

	count, err := r.queries.CountRecordingChunksBySession(ctx, uuid)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// MaxSeqBySession returns the maximum sequence number for a session.
func (r *ChunkRepository) MaxSeqBySession(ctx context.Context, sessionID string) (int, error) {
	uuid, err := parseUUID(sessionID)
	if err != nil {
		return -1, domain.ErrNotFound
	}

	maxSeq, err := r.queries.MaxSeqRecordingChunksBySession(ctx, uuid)
	if err != nil {
		return -1, err
	}
	return int(maxSeq), nil
}

// TotalPlaintextSizeBySession returns the aggregate encoded source bytes.
func (r *ChunkRepository) TotalPlaintextSizeBySession(ctx context.Context, sessionID string) (int64, error) {
	uuid, err := parseUUID(sessionID)
	if err != nil {
		return 0, domain.ErrNotFound
	}
	return r.queries.TotalRecordingChunkPlaintextSizeBySession(ctx, uuid)
}

// DeleteBySession removes all chunk manifest rows for a session.
func (r *ChunkRepository) DeleteBySession(ctx context.Context, sessionID string) error {
	uuid, err := parseUUID(sessionID)
	if err != nil {
		return domain.ErrNotFound
	}

	return r.queries.DeleteRecordingChunksBySession(ctx, uuid)
}

// chunkRowToDomain converts a sqlcdb.RecordingChunk to a domain entity.
func chunkRowToDomain(row sqlcdb.RecordingChunk) *domain.RecordingChunk {
	return &domain.RecordingChunk{
		ID:             uuidToString(row.ID),
		SessionID:      uuidToString(row.SessionID),
		Seq:            int(row.Seq),
		StorageKey:     row.StorageKey,
		WrappedDEK:     row.WrappedDek,
		WrappingNonce:  row.WrappingNonce,
		EncryptionAlgo: row.EncryptionAlgo,
		ChunkSize:      int(row.ChunkSize),
		ChunkCount:     int(row.ChunkCount),
		PlaintextSize:  row.PlaintextSize,
		Checksum:       row.Checksum,
		UploadedAt:     row.UploadedAt,
	}
}
