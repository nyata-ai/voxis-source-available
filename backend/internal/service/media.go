package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// MediaService orchestrates media upload, encryption, and storage.
type MediaService struct {
	mediaRepo    port.MediaRepository
	transRepo    port.TranscriptionRepository
	summaryRepo  port.SummaryRepository
	storage      port.StorageClient
	envelope     *crypto.EnvelopeService
	scanInserter port.ScanJobInserter
	quota        port.StorageQuotaRepository
	quotaApplies bool
	maxDuration  time.Duration
	logger       *slog.Logger
}

// MediaServiceOption configures optional upload integrations.
type MediaServiceOption func(*MediaService)

// WithStorageQuota enables durable quota allocations for bucket-like storage.
// localfs and memory storage remain unlimited by design.
func WithStorageQuota(repo port.StorageQuotaRepository, backend string) MediaServiceOption {
	return func(s *MediaService) {
		s.quota = repo
		s.quotaApplies = repo != nil && domain.IsBucketStorageBackend(backend)
	}
}

// WithMaxMediaDuration sets the MEDIA_MAX_DURATION limit that CheckDuration
// enforces. Without it the domain default applies.
func WithMaxMediaDuration(limit time.Duration) MediaServiceOption {
	return func(s *MediaService) { s.maxDuration = limit }
}

// NewMediaService creates a new MediaService.
func NewMediaService(
	mediaRepo port.MediaRepository,
	transRepo port.TranscriptionRepository,
	summaryRepo port.SummaryRepository,
	storage port.StorageClient,
	envelope *crypto.EnvelopeService,
	scanInserter port.ScanJobInserter,
	logger *slog.Logger,
	opts ...MediaServiceOption,
) *MediaService {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &MediaService{
		mediaRepo:    mediaRepo,
		transRepo:    transRepo,
		summaryRepo:  summaryRepo,
		storage:      storage,
		envelope:     envelope,
		scanInserter: scanInserter,
		maxDuration:  domain.DefaultMaxMediaDuration,
		logger:       logger,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// SetScanInserter updates the scan job inserter used for background scan jobs.
// Called after the River client is initialized (chicken-and-egg with setupRouter).
func (s *MediaService) SetScanInserter(inserter port.ScanJobInserter) {
	s.scanInserter = inserter
}

// Upload validates, persists, encrypts, and stores an audio file.
func (s *MediaService) Upload(
	ctx context.Context,
	orgID, ownerID, filename, contentType string,
	size int64,
	src io.Reader,
) (*domain.Media, error) {
	return s.upload(ctx, orgID, ownerID, filename, contentType, size, src, "", domain.RecordingModeRegular)
}

// UploadStitchedRecording writes final recording media while atomically
// converting that session's chunk allocation to the final media allocation.
//
// mode is the session's recording mode. It selects the storage namespace (and
// therefore the bucket) for the encrypted object, so a privilege recording's
// audio lands in the shred bucket from the very first write. The media row's
// own recording_mode is still stamped later by
// RecordingService.ensureStitchedMediaMode; only the object placement is
// decided here.
func (s *MediaService) UploadStitchedRecording(
	ctx context.Context,
	orgID, ownerID, sessionID, filename, contentType string,
	size int64,
	src io.Reader,
	mode domain.RecordingMode,
) (*domain.Media, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("recording session id is required: %w", domain.ErrInvalidInput)
	}
	return s.upload(ctx, orgID, ownerID, filename, contentType, size, src, sessionID, mode)
}

// mediaUploadState threads per-upload values through the upload phase helpers.
type mediaUploadState struct {
	media        *domain.Media
	orgID        string
	contentType  string
	storageKey   string
	src          io.Reader
	size         int64
	encryptStart time.Time
}

// sealOutcome carries the SealStream goroutine result back to the uploader.
type sealOutcome struct {
	meta *crypto.SealedMeta
	err  error
}

func (s *MediaService) upload(
	ctx context.Context,
	orgID, ownerID, filename, contentType string,
	size int64,
	src io.Reader,
	replacingSessionID string,
	mode domain.RecordingMode,
) (*domain.Media, error) {
	if ownerID == "" {
		return nil, fmt.Errorf("upload owner is required: %w", domain.ErrUnauthorized)
	}
	// 1. Validate and create domain entity.
	media, err := domain.NewMedia(orgID, filename, contentType, size)
	if err != nil {
		return nil, fmt.Errorf("create media: %w", err)
	}
	media.CreatedBy = ownerID

	// 2. Persist with status=pending.
	if persistErr := s.persistPendingMedia(ctx, media, replacingSessionID); persistErr != nil {
		return nil, persistErr
	}

	// 3. Build storage key. Built after persist: the quota path assigns the
	// media ID. The mode namespaces privilege objects into the shred bucket.
	st := &mediaUploadState{
		media:       media,
		orgID:       orgID,
		contentType: contentType,
		storageKey:  domain.MediaStorageKey(orgID, media.ID, mode),
		src:         src,
		size:        size,
	}

	// 4–8. Encrypt-and-store; on failure mark the record failed and clean up.
	meta, err := s.encryptAndStore(ctx, st)
	if err != nil {
		s.cleanupFailedUpload(ctx, media, st.storageKey)
		return nil, err
	}

	// 9. Update media with encryption metadata.
	media.SetEncrypted(
		st.storageKey,
		meta.WrappedDEK,
		meta.WrappingNonce,
		meta.Algorithm,
		meta.ChunkSize,
		meta.ChunkCount,
	)

	if err := s.finalizeUploadRecord(ctx, media, st.storageKey); err != nil {
		return nil, err
	}

	if err := s.ensureUploadScan(ctx, media, orgID); err != nil {
		return nil, err
	}

	s.logger.Info("media persisted",
		"media_id", media.ID,
		"org_id", orgID,
		"size", size,
		"chunks", meta.ChunkCount,
		"total_duration_ms", time.Since(st.encryptStart).Milliseconds(),
	)

	return media, nil
}

// persistPendingMedia persists the pending record. Bucket-backed uploads
// reserve before any object write; PostgreSQL creates both records in one
// transaction.
func (s *MediaService) persistPendingMedia(ctx context.Context, media *domain.Media, replacingSessionID string) error {
	var err error
	if s.quotaApplies {
		media.ID = uuid.NewString()
		err = s.quota.CreateMediaReservation(ctx, media, replacingSessionID)
	} else {
		err = s.mediaRepo.Create(ctx, media)
	}
	if err != nil {
		return fmt.Errorf("persist media: %w", err)
	}
	return nil
}

// encryptAndStore streams the source through SealStream into storage. On
// failure it returns the wrapped encrypt/upload error (encryption failure
// takes precedence) and leaves DB/storage cleanup to the caller.
func (s *MediaService) encryptAndStore(ctx context.Context, st *mediaUploadState) (*crypto.SealedMeta, error) {
	// 4. Create pipe: encrypted data flows from SealStream → storage.Upload.
	pr, pw := io.Pipe()

	// 5. Launch encryption goroutine.
	sealCh := make(chan sealOutcome, 1)

	s.logger.Info("encryption starting", "media_id", st.media.ID, "size", st.size)
	st.encryptStart = time.Now()

	go s.sealToPipe(ctx, st.orgID, st.src, pw, sealCh)

	// 6. Upload encrypted stream to storage.
	metadata := map[string]string{
		"org-id":                st.orgID,
		"media-id":              st.media.ID,
		"original-content-type": st.contentType,
	}
	uploadErr := s.storage.Upload(ctx, st.storageKey, pr, "application/octet-stream", metadata)
	_ = pr.Close() //nolint:errcheck // unblock SealStream goroutine

	s.logger.Info("gcs upload done", "media_id", st.media.ID, "duration_ms", time.Since(st.encryptStart).Milliseconds())

	// 7. Wait for encryption result.
	result := waitForSeal(sealCh)

	// 8. Handle errors.
	if result.err != nil {
		return nil, fmt.Errorf("encrypt media: %w", result.err)
	}
	if uploadErr != nil {
		return nil, fmt.Errorf("upload media: %w", uploadErr)
	}
	return result.meta, nil
}

// sealToPipe runs SealStream, closing the pipe writer and reporting the
// outcome (including panics) through sealCh.
func (s *MediaService) sealToPipe(ctx context.Context, orgID string, src io.Reader, pw *io.PipeWriter, sealCh chan<- sealOutcome) {
	defer func() {
		if r := recover(); r != nil {
			pw.CloseWithError(fmt.Errorf("SealStream panic: %v", r))
			sealCh <- sealOutcome{err: fmt.Errorf("SealStream panic: %v", r)}
		}
	}()
	meta, sealErr := s.envelope.SealStream(ctx, orgID, src, pw)
	if sealErr != nil {
		pw.CloseWithError(sealErr)
		sealCh <- sealOutcome{err: sealErr}
		return
	}
	_ = pw.Close() //nolint:errcheck // pipe close always returns nil
	sealCh <- sealOutcome{meta: meta}
}

// waitForSeal waits for the encryption result (15-minute defense-in-depth timeout).
func waitForSeal(sealCh <-chan sealOutcome) sealOutcome {
	const sealTimeout = 15 * time.Minute
	sealTimer := time.NewTimer(sealTimeout)
	defer sealTimer.Stop()

	select {
	case result := <-sealCh:
		return result
	case <-sealTimer.C:
		return sealOutcome{err: fmt.Errorf("SealStream goroutine did not complete within %v", sealTimeout)}
	}
}

// deleteStorageAndReleaseQuota best-effort deletes the storage object and, for
// quota-tracked uploads, releases the reservation only after a confirmed
// delete. Log messages are caller-supplied to keep failure context precise.
// Returns whether the storage delete was confirmed.
func (s *MediaService) deleteStorageAndReleaseQuota(ctx context.Context, mediaID, storageKey, deleteMsg, releaseMsg string) bool {
	deleteConfirmed := true
	if delErr := s.storage.Delete(ctx, storageKey); delErr != nil && !errors.Is(delErr, domain.ErrNotFound) {
		deleteConfirmed = false
		s.logger.Error(deleteMsg, "storage_key", storageKey, "error", delErr)
	}
	if s.quotaApplies && deleteConfirmed {
		if cleanupErr := s.quota.DeleteMediaAndRelease(ctx, mediaID, storageKey); cleanupErr != nil {
			s.logger.Error(releaseMsg, "media_id", mediaID, "error", cleanupErr)
		}
	}
	return deleteConfirmed
}

// cleanupFailedUpload marks the record failed and removes the (possibly
// partial) storage object after an encrypt/upload failure.
func (s *MediaService) cleanupFailedUpload(ctx context.Context, media *domain.Media, storageKey string) {
	media.Status = domain.MediaStatusFailed
	if updateErr := s.mediaRepo.Update(ctx, media); updateErr != nil {
		s.logger.Error("failed to update media status to failed",
			"media_id", media.ID, "error", updateErr)
	}
	s.deleteStorageAndReleaseQuota(ctx, media.ID, storageKey,
		"failed to clean up storage after upload failure",
		"failed to finalize media cleanup after upload failure")
}

// finalizeUploadRecord persists the encrypted-media metadata (finalizing the
// quota reservation when one applies), cleaning up storage on failure.
func (s *MediaService) finalizeUploadRecord(ctx context.Context, media *domain.Media, storageKey string) error {
	var err error
	if s.quotaApplies {
		err = s.quota.FinalizeMediaReservation(ctx, media)
	} else {
		err = s.mediaRepo.Update(ctx, media)
	}
	if err == nil {
		return nil
	}
	s.logger.Error("failed to update media after upload, cleaning up storage",
		"media_id", media.ID, "storage_key", storageKey, "error", err)
	s.deleteStorageAndReleaseQuota(ctx, media.ID, storageKey,
		"failed to clean up orphaned storage object",
		"failed to finalize media cleanup after metadata failure")
	return fmt.Errorf("update media record: %w", err)
}

// ensureUploadScan enqueues the async malware scan — upload fails if enqueue
// fails to prevent zombie scan_pending media with no scan job. Since River
// jobs live in the same PostgreSQL database as the media record, enqueue
// failure implies PostgreSQL issues which would likely have failed the upload
// anyway.
func (s *MediaService) ensureUploadScan(ctx context.Context, media *domain.Media, orgID string) error {
	if s.scanInserter == nil {
		// No background scanner configured in this runtime (e.g., in-memory/dev).
		// Mark as skipped so downstream stream/transcribe gates don't stall.
		if updateErr := s.mediaRepo.UpdateScanStatus(ctx, media.ID, domain.ScanStatusSkipped); updateErr != nil {
			s.logger.Warn("failed to set scan status to skipped",
				"media_id", media.ID, "error", updateErr)
		} else {
			media.ScanStatus = domain.ScanStatusSkipped
		}
		return nil
	}

	insertErr := s.scanInserter.InsertScanJob(ctx, media.ID, orgID)
	if insertErr == nil {
		return nil
	}
	s.logger.Error("failed to enqueue scan job, rolling back upload",
		"media_id", media.ID, "error", insertErr)
	// Clean up: delete the storage object and media record to prevent orphans.
	s.deleteStorageAndReleaseQuota(ctx, media.ID, media.StorageKey,
		"failed to clean up storage after scan enqueue failure",
		"failed to finalize media cleanup after scan enqueue failure")
	if !s.quotaApplies {
		if delErr := s.mediaRepo.Delete(ctx, media.ID); delErr != nil {
			s.logger.Error("failed to delete media after scan enqueue failure",
				"media_id", media.ID, "error", delErr)
		}
	}
	return fmt.Errorf("enqueue scan job: %w", insertErr)
}

// GetByID retrieves a media record, enforcing organization-level isolation.
func (s *MediaService) GetByID(ctx context.Context, orgID, mediaID string) (*domain.Media, error) {
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}

	if media.OrganizationID != orgID || media.Status == domain.MediaStatusDeleted {
		return nil, domain.ErrNotFound
	}

	s.enrichWithTranscriptionID(ctx, media)
	return media, nil
}

// List retrieves media records for an organization with pagination.
func (s *MediaService) List(ctx context.Context, orgID string, limit, offset int) ([]*domain.Media, int64, error) {
	items, err := s.mediaRepo.ListByOrganization(ctx, orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list media: %w", err)
	}

	total, err := s.mediaRepo.CountByOrganization(ctx, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("count media: %w", err)
	}

	for _, m := range items {
		s.enrichWithTranscriptionID(ctx, m)
	}

	return items, total, nil
}

// Search retrieves media with optional filename and status filters.
func (s *MediaService) Search(ctx context.Context, orgID, search, status string, limit, offset int) ([]*domain.Media, int64, error) {
	items, err := s.mediaRepo.SearchByOrganization(ctx, orgID, search, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search media: %w", err)
	}

	total, err := s.mediaRepo.CountSearchByOrganization(ctx, orgID, search, status)
	if err != nil {
		return nil, 0, fmt.Errorf("count search media: %w", err)
	}

	for _, m := range items {
		s.enrichWithTranscriptionID(ctx, m)
	}

	return items, total, nil
}

// ListFiltered retrieves media with optional filters (date, duration, status, sort, search).
// Delegates to the repository's ListByOrganizationFiltered.
func (s *MediaService) ListFiltered(ctx context.Context, orgID string, filter port.MediaListFilter) ([]*domain.Media, error) {
	items, err := s.mediaRepo.ListByOrganizationFiltered(ctx, orgID, filter)
	if err != nil {
		return nil, fmt.Errorf("list media filtered: %w", err)
	}

	for _, m := range items {
		s.enrichWithTranscriptionID(ctx, m)
	}

	return items, nil
}

// enrichWithTranscriptionID looks up the latest completed transcription
// for a media item and sets LatestTranscriptionID if found.
func (s *MediaService) enrichWithTranscriptionID(ctx context.Context, media *domain.Media) {
	if s.transRepo == nil {
		return
	}
	trans, err := s.transRepo.GetByMediaID(ctx, media.ID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			s.logger.Warn("failed to look up transcription for media",
				"media_id", media.ID, "error", err)
		}
		return
	}
	if trans.Status != domain.TranscriptionStatusCompleted {
		return
	}
	media.LatestTranscriptionID = trans.ID
}

// StreamRange describes the plaintext byte range being served.
type StreamRange struct {
	Start   int64
	End     int64
	Length  int64
	Partial bool
}

// Stream returns a decrypted reader for the requested byte range.
// If rangeStart and rangeEnd are nil, it streams the full file.
func (s *MediaService) Stream(ctx context.Context, orgID, mediaID string, rangeStart, rangeEnd *int64) (*domain.Media, io.ReadCloser, *StreamRange, error) {
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return nil, nil, nil, err
	}

	if media.OrganizationID != orgID || media.Status == domain.MediaStatusDeleted {
		return nil, nil, nil, domain.ErrNotFound
	}
	if media.Status != domain.MediaStatusReady {
		s.logger.Warn("stream attempted on non-ready media", "media_id", media.ID, "status", media.Status)
		return nil, nil, nil, fmt.Errorf("media is not ready for streaming: %w", domain.ErrInvalidInput)
	}
	if media.AudioDeletedAt != nil || media.StorageKey == "" {
		return nil, nil, nil, domain.ErrAudioUnavailable
	}
	if media.ChunkSize <= 0 || media.ChunkCount <= 0 {
		return nil, nil, nil, fmt.Errorf("missing encryption metadata: %w", domain.ErrInvalidInput)
	}

	// Full stream path (no range)
	if rangeStart == nil || rangeEnd == nil {
		return s.streamFull(ctx, orgID, media)
	}

	return s.streamRange(ctx, orgID, media, *rangeStart, *rangeEnd)
}

// sealedMetaFromMedia builds a SealedMeta from the media domain object.
func sealedMetaFromMedia(media *domain.Media) *crypto.SealedMeta {
	return &crypto.SealedMeta{
		EncryptionMeta: crypto.EncryptionMeta{
			Algorithm:     media.EncryptionAlgo,
			ChunkSize:     media.ChunkSize,
			ChunkCount:    media.ChunkCount,
			PlaintextSize: media.Size,
		},
		WrappedDEK:    media.WrappedDEK,
		WrappingNonce: media.WrappingNonce,
	}
}

// streamFull decrypts and returns the entire media file.
func (s *MediaService) streamFull(ctx context.Context, orgID string, media *domain.Media) (*domain.Media, io.ReadCloser, *StreamRange, error) {
	encrypted, dlErr := s.storage.Download(ctx, media.StorageKey)
	if dlErr != nil {
		return nil, nil, nil, fmt.Errorf("download encrypted media: %w", dlErr)
	}

	meta := sealedMetaFromMedia(media)

	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = encrypted.Close() }() //nolint:errcheck // best-effort cleanup
		defer func() {
			if r := recover(); r != nil {
				pw.CloseWithError(fmt.Errorf("OpenStream panic: %v", r))
			}
		}()
		if openErr := s.envelope.OpenStream(ctx, orgID, encrypted, pw, meta); openErr != nil {
			pw.CloseWithError(fmt.Errorf("decrypt stream: %w", openErr))
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close always returns nil
	}()

	r := &StreamRange{Start: 0, End: media.Size - 1, Length: media.Size, Partial: false}
	return media, pr, r, nil
}

// streamRange decrypts and returns a byte range of the media file.
func (s *MediaService) streamRange(ctx context.Context, orgID string, media *domain.Media, start, end int64) (*domain.Media, io.ReadCloser, *StreamRange, error) {
	if start < 0 || end < start || end >= media.Size {
		return nil, nil, nil, domain.ErrInvalidInput
	}

	chunkSize := int64(media.ChunkSize)
	startChunk := int(start / chunkSize)
	endChunk := int(end / chunkSize)
	chunkCount := endChunk - startChunk + 1

	blockSize := int64(4 + media.ChunkSize + 16)
	encStart := int64(startChunk) * blockSize

	// Compute encrypted length to include endChunk (handle last chunk size)
	encLen := int64(chunkCount) * blockSize
	if endChunk == media.ChunkCount-1 {
		lastPlain := media.Size - int64(media.ChunkSize)*(int64(media.ChunkCount)-1)
		lastCipher := lastPlain + 16
		encLen = int64(chunkCount-1)*blockSize + int64(4) + lastCipher
	}

	encrypted, dlErr := s.storage.DownloadRange(ctx, media.StorageKey, encStart, encLen)
	if dlErr != nil {
		return nil, nil, nil, fmt.Errorf("download encrypted range: %w", dlErr)
	}

	meta := sealedMetaFromMedia(media)

	// Filter to exact plaintext range (skip leading bytes, limit total)
	skip := start - int64(startChunk)*chunkSize
	length := end - start + 1

	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = encrypted.Close() }() //nolint:errcheck // best-effort cleanup
		defer func() {
			if r := recover(); r != nil {
				pw.CloseWithError(fmt.Errorf("OpenStreamRange panic: %v", r))
			}
		}()
		writer := newRangeWriter(pw, skip, length)
		if openErr := s.envelope.OpenStreamRange(ctx, orgID, encrypted, writer, meta, startChunk, chunkCount); openErr != nil {
			pw.CloseWithError(fmt.Errorf("decrypt range: %w", openErr))
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close always returns nil
	}()

	r := &StreamRange{Start: start, End: end, Length: length, Partial: true}
	return media, pr, r, nil
}

type rangeWriter struct {
	w      io.Writer
	skip   int64
	remain int64
}

func newRangeWriter(w io.Writer, skip, remain int64) *rangeWriter {
	return &rangeWriter{w: w, skip: skip, remain: remain}
}

// Write implements io.Writer. rangeWriter always reports the full input as
// consumed (even when bytes are skipped or truncated) because the caller
// writes complete decrypted chunks and should never retry partial writes.
func (r *rangeWriter) Write(p []byte) (int, error) {
	consumed := len(p) // always report full input consumed
	if r.remain <= 0 {
		return consumed, nil
	}
	if r.skip > 0 {
		if int64(len(p)) <= r.skip {
			r.skip -= int64(len(p))
			return consumed, nil
		}
		p = p[r.skip:]
		r.skip = 0
	}
	if int64(len(p)) > r.remain {
		p = p[:r.remain]
	}

	n, err := r.w.Write(p)
	r.remain -= int64(n)
	return consumed, err
}

// UpdateDuration sets the audio duration on a media record.
func (s *MediaService) UpdateDuration(ctx context.Context, orgID, mediaID string, duration float64) error {
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if media.OrganizationID != orgID {
		return domain.ErrNotFound
	}
	return s.mediaRepo.UpdateDuration(ctx, mediaID, duration)
}

// MarkAudioDeleted clears active audio encryption metadata after retention
// deletion and records the deletion timestamp. The storage key acts as a
// compare-and-set guard: domain.ErrConflict is returned when no active media
// row matches (already processed by a concurrent sweep or the key changed).
func (s *MediaService) MarkAudioDeleted(ctx context.Context, mediaID, storageKey string, deletedAt time.Time) (*domain.Media, error) {
	if mediaID == "" || storageKey == "" {
		return nil, fmt.Errorf("media id and storage key are required: %w", domain.ErrInvalidInput)
	}
	if deletedAt.IsZero() {
		return nil, fmt.Errorf("deleted-at timestamp is required: %w", domain.ErrInvalidInput)
	}
	if s.quotaApplies {
		if err := s.quota.MarkMediaAudioDeletedAndRelease(ctx, mediaID, storageKey); err != nil {
			return nil, err
		}
		return s.mediaRepo.GetByID(ctx, mediaID)
	}
	return s.mediaRepo.MarkAudioDeleted(ctx, mediaID, storageKey, deletedAt)
}

// Delete permanently removes a media file and everything derived from it after
// verifying organization-level isolation. Stored objects (the encrypted audio
// and any leftover recording chunks) are removed first; the database purge
// then clears every transcript, segment and summary ciphertext, collection
// membership, and the media names, hash, and key references, leaving only
// content-free tombstones. A failure at either step leaves the media visible,
// so the caller can simply retry; a retry on already-deleted media re-runs the
// purge, which also scrubs rows soft-deleted by older releases.
//
// Privilege-mode media is rejected with ErrNotFound: the shred lifecycle
// (PrivilegeShredWorker) is the only sanctioned removal path because the
// regular delete would skip metadata clearing and Gladia-deletion verification.
// We deliberately return the same error as a missing/cross-tenant id — using a
// distinct ErrForbidden here would let a caller enumerate which UUIDs in their
// org are privilege-mode by comparing 404 vs 403 responses. The block is
// logged internally so operators can still spot abuse attempts.
func (s *MediaService) Delete(ctx context.Context, orgID, mediaID string) error {
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return err
	}

	if media.OrganizationID != orgID {
		return domain.ErrNotFound
	}
	if media.IsPrivilege() {
		s.logger.Warn("blocked regular delete of privilege media",
			"media_id", mediaID,
			"org_id", orgID,
		)
		return domain.ErrNotFound
	}
	if err := s.deleteStoredObjects(ctx, media); err != nil {
		return err
	}
	if s.quotaApplies {
		if err := s.quota.DeleteMediaAndRelease(ctx, media.ID, media.StorageKey); err != nil {
			return fmt.Errorf("finalize media deletion %s: %w", media.ID, err)
		}
		return nil
	}
	if err := s.mediaRepo.CascadeDelete(ctx, mediaID); err != nil {
		return fmt.Errorf("purge media %s: %w", mediaID, err)
	}
	s.logger.Info("media purged", "media_id", mediaID, "org_id", orgID)
	return nil
}

// deleteStoredObjects removes the encrypted audio object and the chunk objects
// of any recording session the media was stitched from. Missing objects count
// as already deleted.
func (s *MediaService) deleteStoredObjects(ctx context.Context, media *domain.Media) error {
	if media.StorageKey != "" {
		if err := s.storage.Delete(ctx, media.StorageKey); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("delete storage object for media %s: %w", media.ID, err)
		}
	}
	lister, ok := s.mediaRepo.(port.MediaPurgeChunkLister)
	if !ok {
		return nil
	}
	keys, err := lister.ListRecordingChunkKeysForPurge(ctx, media.ID)
	if err != nil {
		return fmt.Errorf("list recording chunks for media %s: %w", media.ID, err)
	}
	for _, key := range keys {
		if err := s.storage.Delete(ctx, key); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("delete recording chunk for media %s: %w", media.ID, err)
		}
	}
	return nil
}

// DeleteStorageByPrefix removes every stored object under prefix. Objects that
// disappear concurrently count as deleted.
func DeleteStorageByPrefix(ctx context.Context, storage port.StorageClient, prefix string) error {
	if storage == nil || prefix == "" {
		return fmt.Errorf("storage and prefix are required: %w", domain.ErrInvalidInput)
	}
	keys, err := storage.ListByPrefix(ctx, prefix)
	if err != nil {
		return fmt.Errorf("list objects: %w", err)
	}
	for _, key := range keys {
		if delErr := storage.Delete(ctx, key); delErr != nil && !errors.Is(delErr, domain.ErrNotFound) {
			return fmt.Errorf("delete object: %w", delErr)
		}
	}
	return nil
}

// CheckDuration rejects media longer than the configured maximum with
// domain.ErrMediaTooLong. A zero or unknown duration passes: the ffmpeg
// duration cap and the transcription worker still bound such files.
func (s *MediaService) CheckDuration(durationSeconds float64) error {
	return domain.CheckMediaDuration(durationSeconds, s.maxDuration)
}

// CheckStitchedDuration applies the media limit to a stitched browser
// recording, plus domain.MediaDurationGrace for the final chunk: the recording
// limit, which config keeps within the media limit, is enforced on wall-clock
// time as chunks arrive.
func (s *MediaService) CheckStitchedDuration(durationSeconds float64) error {
	if s.maxDuration <= 0 {
		return nil
	}
	return domain.CheckMediaDuration(durationSeconds, s.maxDuration+domain.MediaDurationGrace)
}

// UpdateMetadata updates the title and/or description on a media record.
// Pointer fields: nil = don't change, non-nil = set to this value (empty string = clear).
func (s *MediaService) UpdateMetadata(ctx context.Context, orgID, mediaID string, title, description *string) (*domain.Media, error) {
	// Validate before touching the database
	if title != nil {
		cleaned, err := domain.ValidateTitle(*title)
		if err != nil {
			return nil, fmt.Errorf("update metadata: %w", err)
		}
		title = &cleaned
	}
	if description != nil {
		cleaned, err := domain.ValidateDescription(*description)
		if err != nil {
			return nil, fmt.Errorf("update metadata: %w", err)
		}
		description = &cleaned
	}

	// Verify org isolation
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if media.OrganizationID != orgID || media.Status == domain.MediaStatusDeleted {
		return nil, domain.ErrNotFound
	}

	updated, err := s.mediaRepo.UpdateMetadata(ctx, mediaID, title, description)
	if err != nil {
		return nil, err
	}
	s.enrichWithTranscriptionID(ctx, updated)
	return updated, nil
}

// SetRecordingMode updates the recording_mode column on a media record.
// Used by RecordingService.Stitch to propagate the session's mode after
// MediaService.Upload has already created the row in regular mode.
//
// Caller must verify org isolation. Returns ErrNotFound if missing.
func (s *MediaService) SetRecordingMode(ctx context.Context, orgID, mediaID string, mode domain.RecordingMode) error {
	if !mode.IsValid() {
		return fmt.Errorf("invalid recording mode %q: %w", mode, domain.ErrInvalidInput)
	}
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if media.OrganizationID != orgID {
		return domain.ErrNotFound
	}
	media.RecordingMode = mode
	return s.mediaRepo.Update(ctx, media)
}

// StoreForensics persists the file hash and encrypted audio analysis.
func (s *MediaService) StoreForensics(ctx context.Context, orgID, mediaID, fileHash string, analysis *domain.AudioAnalysis) error {
	// Verify org isolation before encrypting
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if media.OrganizationID != orgID || media.Status == domain.MediaStatusDeleted {
		return fmt.Errorf("store forensics: org mismatch: %w", domain.ErrNotFound)
	}

	if analysis == nil {
		return s.mediaRepo.UpdateForensics(ctx, mediaID, fileHash, nil)
	}

	jsonBytes, err := json.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("marshal analysis: %w", err)
	}

	sealed, err := s.envelope.SealField(ctx, orgID, jsonBytes)
	if err != nil {
		return fmt.Errorf("encrypt analysis: %w", err)
	}

	metaBytes, err := json.Marshal(sealed)
	if err != nil {
		return fmt.Errorf("marshal sealed metadata: %w", err)
	}

	return s.mediaRepo.UpdateForensics(ctx, mediaID, fileHash, metaBytes)
}

// BuildAnalysis creates forensic analysis from probe results and file extension.
// This wraps the package-level BuildAnalysis function to keep the handler
// decoupled from the forensics implementation.
func (s *MediaService) BuildAnalysis(probe *port.AudioProbeResult, fileExt string) *domain.AudioAnalysis {
	return BuildAnalysis(probe, fileExt)
}

// GetAudioAnalysis decrypts and returns the audio analysis for a media record.
// Returns (nil, nil) if no analysis is stored.
func (s *MediaService) GetAudioAnalysis(ctx context.Context, orgID string, media *domain.Media) (*domain.AudioAnalysis, error) {
	if media.OrganizationID != orgID {
		return nil, fmt.Errorf("get audio analysis: org mismatch: %w", domain.ErrNotFound)
	}
	if len(media.AudioMetadata) == 0 {
		return nil, nil
	}

	var sealed crypto.SealedFieldMeta
	if err := json.Unmarshal(media.AudioMetadata, &sealed); err != nil {
		return nil, fmt.Errorf("unmarshal sealed metadata: %w", err)
	}

	plaintext, err := s.envelope.OpenField(ctx, orgID, &sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypt analysis: %w", err)
	}

	var analysis domain.AudioAnalysis
	if err := json.Unmarshal(plaintext, &analysis); err != nil {
		return nil, fmt.Errorf("unmarshal analysis: %w", err)
	}

	// Redact raw tags before returning -- they may contain PII
	analysis.FormatTags = nil
	analysis.StreamTags = nil

	return &analysis, nil
}
