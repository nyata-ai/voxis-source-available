package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/adapter/postgres/sqlcdb"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Two hours safely exceeds the bounded 20-minute upload deadline plus the
// 15-minute sealing guard, while still releasing genuinely abandoned writes.
const storageReservationTTL = 2 * time.Hour

var _ port.StorageQuotaRepository = (*StorageQuotaRepository)(nil)
var _ port.UserStorageUsageRepository = (*StorageQuotaRepository)(nil)

// StorageQuotaRepository persists logical storage allocations. It deliberately
// uses one small ledger rather than a cache: the users row lock serializes all
// checks and mutations for one user across API instances.
type StorageQuotaRepository struct{ pool *pgxpool.Pool }

// NewStorageQuotaRepository builds a repository backed by the given pool.
func NewStorageQuotaRepository(pool *pgxpool.Pool) *StorageQuotaRepository {
	return &StorageQuotaRepository{pool: pool}
}

// GetStorageQuotaPolicy returns the single global storage quota policy row.
func (r *StorageQuotaRepository) GetStorageQuotaPolicy(ctx context.Context) (domain.StorageQuotaPolicy, error) {
	return getStorageQuotaPolicy(ctx, r.pool)
}

func getStorageQuotaPolicy(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (domain.StorageQuotaPolicy, error) {
	const query = `SELECT enabled, default_limit_bytes, warning_threshold_percent, updated_at, updated_by
FROM app_storage_quota_policy WHERE id = TRUE`
	var policy domain.StorageQuotaPolicy
	var updatedAt pgtype.Timestamptz
	var updatedBy pgtype.Text
	if err := db.QueryRow(ctx, query).Scan(
		&policy.Enabled,
		&policy.DefaultLimitBytes,
		&policy.WarningThresholdPercent,
		&updatedAt,
		&updatedBy,
	); err != nil {
		return domain.StorageQuotaPolicy{}, err
	}
	if updatedAt.Valid {
		t := updatedAt.Time
		policy.UpdatedAt = &t
	}
	if updatedBy.Valid {
		policy.UpdatedBy = updatedBy.String
	}
	return policy, nil
}

// UpdateStorageQuotaPolicy validates and persists the global policy, recording
// who changed it, and returns the stored row.
func (r *StorageQuotaRepository) UpdateStorageQuotaPolicy(
	ctx context.Context,
	policy domain.StorageQuotaPolicy,
	updatedBy string,
) (domain.StorageQuotaPolicy, error) {
	if policy.DefaultLimitBytes <= 0 {
		return domain.StorageQuotaPolicy{}, fmt.Errorf("default limit must be positive: %w", domain.ErrInvalidInput)
	}
	if policy.WarningThresholdPercent != domain.StorageQuotaWarningThresholdPercent {
		return domain.StorageQuotaPolicy{}, fmt.Errorf("warning threshold is fixed at %d: %w", domain.StorageQuotaWarningThresholdPercent, domain.ErrInvalidInput)
	}
	const query = `UPDATE app_storage_quota_policy
SET enabled = $1, default_limit_bytes = $2, updated_at = NOW(), updated_by = $3
WHERE id = TRUE
RETURNING enabled, default_limit_bytes, warning_threshold_percent, updated_at, updated_by`
	var updated domain.StorageQuotaPolicy
	var updatedAt pgtype.Timestamptz
	var updatedByValue pgtype.Text
	err := r.pool.QueryRow(ctx, query, policy.Enabled, policy.DefaultLimitBytes, updatedBy).Scan(
		&updated.Enabled,
		&updated.DefaultLimitBytes,
		&updated.WarningThresholdPercent,
		&updatedAt,
		&updatedByValue,
	)
	if err != nil {
		return domain.StorageQuotaPolicy{}, err
	}
	if updatedAt.Valid {
		t := updatedAt.Time
		updated.UpdatedAt = &t
	}
	if updatedByValue.Valid {
		updated.UpdatedBy = updatedByValue.String
	}
	return updated, nil
}

// CreateMediaReservation atomically checks the quota, records a reserved
// allocation, and inserts the pending media row in one transaction.
func (r *StorageQuotaRepository) CreateMediaReservation(ctx context.Context, media *domain.Media, replacingSessionID string) error {
	if media == nil || media.ID == "" || media.CreatedBy == "" {
		return fmt.Errorf("media id and owner are required: %w", domain.ErrInvalidInput)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin media reservation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	orgID, err := parseUUID(media.OrganizationID)
	if err != nil {
		return domain.ErrNotFound
	}
	mediaID, err := parseUUID(media.ID)
	if err != nil {
		return domain.ErrInvalidInput
	}
	var replacingID pgtype.UUID
	if replacingSessionID != "" {
		replacingID, err = parseUUID(replacingSessionID)
		if err != nil {
			return domain.ErrInvalidInput
		}
	}
	bytes, err := r.reserveMediaTx(ctx, tx, orgID, media.CreatedBy, mediaID, replacingID, media.Size)
	if err != nil {
		return err
	}
	if err := insertPendingMedia(ctx, tx, media, orgID, mediaID); err != nil {
		return err
	}
	media.Size = bytes
	return tx.Commit(ctx)
}

func insertPendingMedia(ctx context.Context, tx pgx.Tx, media *domain.Media, orgID, mediaID pgtype.UUID) error {
	const query = `INSERT INTO media (
 id, organization_id, created_by, filename, content_type, size, duration, status, scan_status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING created_at, updated_at`
	var createdAt, updatedAt time.Time
	err := tx.QueryRow(ctx, query,
		mediaID, orgID, media.CreatedBy, media.Filename, media.ContentType,
		media.Size, media.Duration, media.Status, media.ScanStatus,
	).Scan(&createdAt, &updatedAt)
	if err != nil {
		return fmt.Errorf("insert pending media: %w", err)
	}
	media.CreatedAt = createdAt
	media.UpdatedAt = updatedAt
	media.RecordingMode = domain.RecordingModeRegular
	return nil
}

func (r *StorageQuotaRepository) reserveMediaTx(
	ctx context.Context,
	tx pgx.Tx,
	orgID pgtype.UUID,
	userID string,
	mediaID, replacingSessionID pgtype.UUID,
	requested int64,
) (int64, error) {
	if requested < 0 {
		return 0, fmt.Errorf("negative storage reservation: %w", domain.ErrInvalidInput)
	}
	if err := lockStorageUser(ctx, tx, orgID, userID); err != nil {
		return 0, err
	}
	charge := requested
	if replacingSessionID.Valid {
		var existingMediaID pgtype.UUID
		err := tx.QueryRow(ctx, `SELECT media_id FROM user_storage_allocations
WHERE replaces_session_id = $1 AND kind = 'media' AND state != 'released'`, replacingSessionID).Scan(&existingMediaID)
		if err == nil {
			return 0, domain.ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		chunkBytes, err := activeSessionChunkBytes(ctx, tx, replacingSessionID)
		if err != nil {
			return 0, err
		}
		if chunkBytes >= requested {
			charge = 0
		} else {
			charge = requested - chunkBytes
		}
	}
	if err := enforceStorageLimit(ctx, tx, userID, charge, requested); err != nil {
		return 0, err
	}
	const query = `INSERT INTO user_storage_allocations (
 id, organization_id, user_id, kind, media_id, replaces_session_id, bytes, state, expires_at
) VALUES ($1, $2, $3, 'media', $4, $5, $6, 'reserved', NOW() + ($7::bigint * INTERVAL '1 second'))`
	_, err := tx.Exec(ctx, query, mediaAllocationID(mediaID), orgID, userID, mediaID, replacingSessionID, charge, int64(storageReservationTTL/time.Second))
	if err != nil {
		return 0, fmt.Errorf("insert media reservation: %w", err)
	}
	return requested, nil
}

// FinalizeMediaReservation commits a reserved media allocation to its sealed
// size and stores the media encryption metadata in the same transaction.
func (r *StorageQuotaRepository) FinalizeMediaReservation(ctx context.Context, media *domain.Media) error {
	if media == nil || media.ID == "" {
		return fmt.Errorf("media id is required: %w", domain.ErrInvalidInput)
	}
	mediaID, err := parseUUID(media.ID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin media finalization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit

	userID, err := mediaAllocationUser(ctx, tx, mediaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if lockErr := lockStorageUserByID(ctx, tx, userID); lockErr != nil {
		return lockErr
	}
	// user_id is immutable on allocations, so the locked row's owner matches
	// the userID read above; only the state is needed here.
	_, _, state, err := lockMediaAllocation(ctx, tx, mediaID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if state == "released" {
		return domain.ErrConflict
	}
	if err := updateMediaEncryption(ctx, tx, mediaID, media); err != nil {
		return err
	}
	if state == "reserved" {
		const commit = `UPDATE user_storage_allocations
SET bytes = $1, state = 'committed', expires_at = NULL, updated_at = NOW()
WHERE media_id = $2 AND state = 'reserved'`
		if _, err := tx.Exec(ctx, commit, media.Size, mediaID); err != nil {
			return fmt.Errorf("commit media reservation: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func mediaAllocationUser(ctx context.Context, tx pgx.Tx, mediaID pgtype.UUID) (string, error) {
	var userID string
	err := tx.QueryRow(ctx, `SELECT user_id FROM user_storage_allocations WHERE media_id = $1`, mediaID).Scan(&userID)
	return userID, err
}

func lockMediaAllocation(
	ctx context.Context, tx pgx.Tx, mediaID pgtype.UUID,
) (userID string, replacingSessionID pgtype.UUID, state string, err error) {
	const query = `SELECT user_id, replaces_session_id, state
FROM user_storage_allocations WHERE media_id = $1 FOR UPDATE`
	err = tx.QueryRow(ctx, query, mediaID).Scan(&userID, &replacingSessionID, &state)
	return userID, replacingSessionID, state, err
}

func updateMediaEncryption(ctx context.Context, tx pgx.Tx, mediaID pgtype.UUID, media *domain.Media) error {
	const query = `UPDATE media SET
 storage_key = $1, wrapped_dek = $2, wrapping_nonce = $3, encryption_algo = $4,
 chunk_size = $5, chunk_count = $6, status = $7
WHERE id = $8 RETURNING updated_at`
	chunkSize, err := int32FromInt(media.ChunkSize)
	if err != nil {
		return fmt.Errorf("chunk size: %w", err)
	}
	chunkCount, err := int32FromInt(media.ChunkCount)
	if err != nil {
		return fmt.Errorf("chunk count: %w", err)
	}
	var updatedAt time.Time
	err = tx.QueryRow(ctx, query,
		pgtype.Text{String: media.StorageKey, Valid: media.StorageKey != ""},
		media.WrappedDEK, media.WrappingNonce,
		pgtype.Text{String: media.EncryptionAlgo, Valid: media.EncryptionAlgo != ""},
		chunkSize, chunkCount, media.Status, mediaID,
	).Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update reserved media: %w", err)
	}
	media.UpdatedAt = updatedAt
	return nil
}

// ReleaseMediaReservation marks the media allocation released, refunding its
// bytes to the owner's quota.
func (r *StorageQuotaRepository) ReleaseMediaReservation(ctx context.Context, mediaID string) error {
	id, err := parseUUID(mediaID)
	if err != nil {
		return domain.ErrNotFound
	}
	return r.releaseAllocation(ctx,
		`SELECT user_id FROM user_storage_allocations WHERE media_id = $1`,
		id, mediaAllocationID(id))
}

// DeleteMediaAndRelease atomically soft-deletes regular media and releases its
// allocation after the caller has confirmed that the bucket object is gone.
// A transaction failure leaves both the visible media row and charge intact so
// a later delete can safely retry the cleanup.
func (r *StorageQuotaRepository) DeleteMediaAndRelease(ctx context.Context, mediaID, storageKey string) error {
	id, err := parseUUID(mediaID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit

	userID, err := mediaAllocationUser(ctx, tx, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if userID != "" {
		if lockErr := lockStorageUserByID(ctx, tx, userID); lockErr != nil {
			return lockErr
		}
	}

	currentKey, err := lockMediaStorageKey(ctx, tx, id)
	if err != nil {
		return err
	}
	if currentKey.Valid && currentKey.String != "" && currentKey.String != storageKey {
		return domain.ErrConflict
	}
	if err := softDeleteRegularMedia(ctx, tx, id); err != nil {
		return err
	}
	if userID != "" {
		if err := markAllocationReleased(ctx, tx, `UPDATE user_storage_allocations SET state = 'released', released_at = COALESCE(released_at, NOW()), updated_at = NOW() WHERE media_id = $1 AND state != 'released'`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func lockMediaStorageKey(ctx context.Context, tx pgx.Tx, mediaID pgtype.UUID) (pgtype.Text, error) {
	var storageKey pgtype.Text
	err := tx.QueryRow(ctx, `SELECT storage_key FROM media WHERE id = $1 FOR UPDATE`, mediaID).Scan(&storageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Text{}, domain.ErrNotFound
	}
	return storageKey, err
}

// softDeleteRegularMedia applies the same content purge as
// MediaRepository.CascadeDelete inside the quota transaction.
func softDeleteRegularMedia(ctx context.Context, tx pgx.Tx, mediaID pgtype.UUID) error {
	err := purgeMedia(ctx, sqlcdb.New(tx), mediaID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrConflict
	}
	return err
}

// FindCommittedStitchedMedia returns the media ID whose committed allocation
// replaces the given recording session, if one exists.
func (r *StorageQuotaRepository) FindCommittedStitchedMedia(ctx context.Context, sessionID string) (mediaID string, found bool, err error) {
	id, err := parseUUID(sessionID)
	if err != nil {
		return "", false, domain.ErrNotFound
	}
	var mediaUUID pgtype.UUID
	err = r.pool.QueryRow(ctx, `SELECT media_id FROM user_storage_allocations
WHERE replaces_session_id = $1 AND kind = 'media' AND state = 'committed'`, id).Scan(&mediaUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uuidToString(mediaUUID), true, nil
}

// MarkMediaAudioDeletedAndRelease clears the encryption metadata of regular
// media whose bucket object is gone and releases its allocation atomically.
func (r *StorageQuotaRepository) MarkMediaAudioDeletedAndRelease(ctx context.Context, mediaID, storageKey string) error {
	id, err := parseUUID(mediaID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM user_storage_allocations WHERE media_id = $1`, id).Scan(&userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if userID != "" {
		if lockErr := lockStorageUserByID(ctx, tx, userID); lockErr != nil {
			return lockErr
		}
	}
	const clearQuery = `UPDATE media SET storage_key = NULL, wrapped_dek = NULL, wrapping_nonce = NULL,
 encryption_algo = NULL, chunk_size = 0, chunk_count = 0, file_hash = NULL, audio_metadata = NULL,
 audio_deleted_at = COALESCE(audio_deleted_at, NOW()), updated_at = NOW()
WHERE id = $1 AND storage_key = $2`
	result, err := tx.Exec(ctx, clearQuery, id, storageKey)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	if userID != "" {
		if err := markAllocationReleased(ctx, tx, `UPDATE user_storage_allocations SET state = 'released', released_at = COALESCE(released_at, NOW()), updated_at = NOW() WHERE media_id = $1 AND state != 'released'`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ReserveRecordingChunk reserves quota for one recording chunk upload;
// retries of the same seq extend the existing reservation idempotently.
func (r *StorageQuotaRepository) ReserveRecordingChunk(ctx context.Context, session *domain.RecordingSession, seq int, bytes int64) error {
	if session == nil || session.ID == "" || session.UserID == "" || bytes < 0 {
		return fmt.Errorf("recording chunk reservation is invalid: %w", domain.ErrInvalidInput)
	}
	orgID, err := parseUUID(session.OrganizationID)
	if err != nil {
		return domain.ErrNotFound
	}
	sessionID, err := parseUUID(session.ID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	if lockErr := lockStorageUser(ctx, tx, orgID, session.UserID); lockErr != nil {
		return lockErr
	}
	additionalBytes, err := chunkReservationAdditionalBytes(ctx, tx, sessionID, seq, bytes)
	if err != nil {
		return err
	}
	if err := enforceStorageLimit(ctx, tx, session.UserID, additionalBytes, bytes); err != nil {
		return err
	}
	if err := insertChunkReservation(ctx, tx, orgID, session.UserID, sessionID, seq, bytes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// chunkReservationAdditionalBytes makes retries idempotent. A retry extends
// an existing reservation instead of treating its already-reserved bytes as a
// second request; a committed manifest cannot be overwritten by a new chunk.
func chunkReservationAdditionalBytes(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID, seq int, bytes int64) (int64, error) {
	var state string
	var existingBytes int64
	err := tx.QueryRow(ctx, `SELECT state, bytes FROM user_storage_allocations
WHERE id = $1 FOR UPDATE`, chunkAllocationID(sessionID, seq)).Scan(&state, &existingBytes)
	if errors.Is(err, pgx.ErrNoRows) || state == "released" {
		return bytes, nil
	}
	if err != nil {
		return 0, err
	}
	if state == "committed" {
		return 0, domain.ErrConflict
	}
	if bytes <= existingBytes {
		return 0, nil
	}
	return bytes - existingBytes, nil
}

func insertChunkReservation(ctx context.Context, tx pgx.Tx, orgID pgtype.UUID, userID string, sessionID pgtype.UUID, seq int, bytes int64) error {
	const query = `INSERT INTO user_storage_allocations (
 id, organization_id, user_id, kind, recording_session_id, recording_chunk_seq, bytes, state, expires_at
) VALUES ($1, $2, $3, 'recording_chunk', $4, $5, $6, 'reserved', NOW() + ($7::bigint * INTERVAL '1 second'))
ON CONFLICT (id) DO UPDATE SET
 bytes = EXCLUDED.bytes, state = 'reserved', expires_at = EXCLUDED.expires_at,
 released_at = NULL, updated_at = NOW()
WHERE user_storage_allocations.state IN ('reserved', 'released')`
	_, err := tx.Exec(ctx, query, chunkAllocationID(sessionID, seq), orgID, userID, sessionID, seq, bytes, int64(storageReservationTTL/time.Second))
	return err
}

// FinalizeRecordingChunk records the chunk manifest and commits its reserved
// allocation in one transaction.
func (r *StorageQuotaRepository) FinalizeRecordingChunk(ctx context.Context, chunk *domain.RecordingChunk) error {
	if chunk == nil {
		return domain.ErrInvalidInput
	}
	sessionID, err := parseUUID(chunk.SessionID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM user_storage_allocations WHERE id = $1`, chunkAllocationID(sessionID, chunk.Seq)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if lockErr := lockStorageUserByID(ctx, tx, userID); lockErr != nil {
		return lockErr
	}
	var state string
	err = tx.QueryRow(ctx, `SELECT state FROM user_storage_allocations WHERE id = $1 FOR UPDATE`, chunkAllocationID(sessionID, chunk.Seq)).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if state == "released" {
		return domain.ErrConflict
	}
	if err := insertChunkManifest(ctx, tx, chunk, sessionID); err != nil {
		return err
	}
	if state == "reserved" {
		if _, err := tx.Exec(ctx, `UPDATE user_storage_allocations SET state = 'committed', expires_at = NULL, updated_at = NOW() WHERE id = $1 AND state = 'reserved'`, chunkAllocationID(sessionID, chunk.Seq)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func insertChunkManifest(ctx context.Context, tx pgx.Tx, chunk *domain.RecordingChunk, sessionID pgtype.UUID) error {
	const insert = `INSERT INTO recording_chunks (
 session_id, seq, storage_key, wrapped_dek, wrapping_nonce, encryption_algo, chunk_size, chunk_count, plaintext_size, checksum
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT ON CONSTRAINT uq_recording_chunks_session_seq DO NOTHING
RETURNING id, uploaded_at`
	var id pgtype.UUID
	var uploadedAt time.Time
	err := tx.QueryRow(ctx, insert, sessionID, chunk.Seq, chunk.StorageKey, chunk.WrappedDEK, chunk.WrappingNonce,
		chunk.EncryptionAlgo, chunk.ChunkSize, chunk.ChunkCount, chunk.PlaintextSize, chunk.Checksum).Scan(&id, &uploadedAt)
	if err == nil {
		chunk.ID = uuidToString(id)
		chunk.UploadedAt = uploadedAt
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var checksum string
	var plaintextSize int64
	err = tx.QueryRow(ctx, `SELECT checksum, plaintext_size FROM recording_chunks WHERE session_id = $1 AND seq = $2`, sessionID, chunk.Seq).Scan(&checksum, &plaintextSize)
	if err != nil {
		return err
	}
	if checksum != chunk.Checksum || plaintextSize != chunk.PlaintextSize {
		return domain.ErrConflict
	}
	return nil
}

// ReleaseRecordingChunk releases the allocation of a single chunk that never
// reached durable storage.
func (r *StorageQuotaRepository) ReleaseRecordingChunk(ctx context.Context, sessionID string, seq int) error {
	id, err := parseUUID(sessionID)
	if err != nil {
		return domain.ErrNotFound
	}
	allocationID := chunkAllocationID(id, seq)
	return r.releaseAllocation(ctx, `SELECT user_id FROM user_storage_allocations WHERE id = $1`, allocationID, allocationID)
}

// ReleaseRecordingChunks releases every unreleased chunk allocation of a
// recording session, e.g. after cleanup deletes the chunk objects.
func (r *StorageQuotaRepository) ReleaseRecordingChunks(ctx context.Context, sessionID string) error {
	id, err := parseUUID(sessionID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM user_storage_allocations WHERE recording_session_id = $1 AND state != 'released' LIMIT 1`, id).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if lockErr := lockStorageUserByID(ctx, tx, userID); lockErr != nil {
		return lockErr
	}
	if err := releaseSessionChunksTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *StorageQuotaRepository) releaseAllocation(ctx context.Context, selectUser string, selectArg, allocationID any) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	var userID string
	err = tx.QueryRow(ctx, selectUser, selectArg).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if lockErr := lockStorageUserByID(ctx, tx, userID); lockErr != nil {
		return lockErr
	}
	if err := markAllocationReleased(ctx, tx, `UPDATE user_storage_allocations SET state = 'released', released_at = COALESCE(released_at, NOW()), updated_at = NOW() WHERE user_id = $1 AND id = $2 AND state != 'released'`, userID, allocationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func markAllocationReleased(ctx context.Context, tx pgx.Tx, query string, args ...any) error {
	_, err := tx.Exec(ctx, query, args...)
	return err
}

func releaseSessionChunksTx(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE user_storage_allocations SET state = 'released', released_at = COALESCE(released_at, NOW()), updated_at = NOW()
WHERE recording_session_id = $1 AND kind = 'recording_chunk' AND state != 'released'`, sessionID)
	return err
}

// GetUserStorageUsedBytes reports the user's active (unreleased) allocation
// total, serialized against concurrent writers via the users row lock.
func (r *StorageQuotaRepository) GetUserStorageUsedBytes(ctx context.Context, orgID, userID string) (int64, error) {
	orgUUID, err := parseUUID(orgID)
	if err != nil {
		return 0, domain.ErrNotFound
	}
	if userID == "" {
		return 0, domain.ErrUnauthorized
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // no-op after a successful commit
	if lockErr := lockStorageUser(ctx, tx, orgUUID, userID); lockErr != nil {
		if !errors.Is(lockErr, domain.ErrForbidden) {
			return 0, lockErr
		}
		// A deleted legacy subject cannot issue new writes, so there is no
		// concurrent allocator to serialize. Keep its remaining ledger rows
		// visible for conservative reporting and cleanup.
	}
	var used int64
	err = tx.QueryRow(ctx, activeStorageUsageQuery+` AND a.organization_id = $1 AND a.user_id = $2`, orgID, userID).Scan(&used)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return used, nil
}

func lockStorageUser(ctx context.Context, tx pgx.Tx, orgID pgtype.UUID, userID string) error {
	var ignored string
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 AND organization_id = $2 FOR UPDATE`, userID, orgID).Scan(&ignored)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrForbidden
	}
	return err
}

func lockStorageUserByID(ctx context.Context, tx pgx.Tx, userID string) error {
	var ignored string
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&ignored)
	if errors.Is(err, pgx.ErrNoRows) {
		// A legacy recording can retain a stale Keycloak subject. New writes
		// always use lockStorageUser above, but cleanup must still be able to
		// release this durable allocation after deleting its object.
		return nil
	}
	return err
}

func activeSessionChunkBytes(ctx context.Context, tx pgx.Tx, sessionID pgtype.UUID) (int64, error) {
	var total int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(bytes), 0) FROM user_storage_allocations
WHERE recording_session_id = $1 AND kind = 'recording_chunk' AND state != 'released'`, sessionID).Scan(&total)
	return total, err
}

func enforceStorageLimit(ctx context.Context, tx pgx.Tx, userID string, additionalBytes, requestedBytes int64) error {
	policy, err := getStorageQuotaPolicy(ctx, tx)
	if err != nil {
		return err
	}
	var used int64
	err = tx.QueryRow(ctx, activeStorageUsageQuery+` AND a.user_id = $1`, userID).Scan(&used)
	if err != nil {
		return err
	}
	if !policy.Enabled || additionalBytes <= 0 || used <= policy.DefaultLimitBytes-additionalBytes {
		return nil
	}
	remaining := max(policy.DefaultLimitBytes-used, 0)
	return &domain.StorageQuotaExceededError{
		RequestedBytes: requestedBytes,
		UsedBytes:      used,
		LimitBytes:     policy.DefaultLimitBytes,
		RemainingBytes: remaining,
	}
}

// int32FromInt rejects values that would silently wrap in an int32 column.
func int32FromInt(v int) (int32, error) {
	if v < 0 || v > math.MaxInt32 {
		return 0, fmt.Errorf("value %d out of int32 range: %w", v, domain.ErrInvalidInput)
	}
	return int32(v), nil
}

func mediaAllocationID(mediaID pgtype.UUID) string { return "media:" + uuidToString(mediaID) }

func chunkAllocationID(sessionID pgtype.UUID, seq int) string {
	return fmt.Sprintf("chunk:%s:%d", uuidToString(sessionID), seq)
}

// A committed stitched-media allocation logically replaces its session's
// chunks. The manifest rows remain allocated until storage cleanup succeeds;
// excluding them here prevents double charging while restoring the chunk charge
// automatically if a later compensation releases the final media allocation.
const activeStorageUsageQuery = `SELECT COALESCE(SUM(a.bytes), 0)
FROM user_storage_allocations a
WHERE a.state != 'released'
  AND (
    a.kind = 'media'
    OR NOT EXISTS (
      SELECT 1 FROM user_storage_allocations replacement
      WHERE replacement.kind = 'media'
        AND replacement.state = 'committed'
        AND replacement.replaces_session_id = a.recording_session_id
    )
  )`
