package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// RecordingService orchestrates live audio recording sessions.
type RecordingService struct {
	recordingRepo port.RecordingRepository
	chunkRepo     port.ChunkRepository
	mediaSvc      *MediaService
	storage       port.StorageClient
	envelope      *crypto.EnvelopeService
	prober        port.AudioProber
	stitcher      port.AudioStitcher
	scanner       port.MalwareScanner
	jobInserter   port.RecordingJobInserter
	quota         port.StorageQuotaRepository
	quotaApplies  bool
	logger        *slog.Logger
	policy        RecordingPolicy
	now           func() time.Time
	// deleteMedia compensates an upload whose session anchor failed to persist.
	// Defaults to mediaSvc.Delete; a field so tests can observe the call.
	deleteMedia func(ctx context.Context, orgID, mediaID string) error
	// setRecordingMode must succeed before a privilege session is completed so
	// downstream privilege lifecycle work is never silently skipped.
	setRecordingMode func(ctx context.Context, orgID, mediaID string, mode domain.RecordingMode) error
}

const (
	defaultRecordingMaxDuration            = 8 * time.Hour
	defaultRecordingMaxChunksPerSess       = 1200
	defaultRecordingOrphanThreshold        = 30 * time.Minute
	defaultStitchServiceTimeout            = 50 * time.Minute
	defaultStitchDirMaxAge                 = 6 * time.Hour
	stitchDirPrefix                        = "voxis-stitch-"
	defaultStitchOverheadBytes       int64 = 64 * 1024 * 1024
	defaultRecordingScanTimeout            = 10 * time.Minute
	cleanupSessionsLimit                   = 100
	devShmRoot                             = "/dev/shm"
	// defaultRetentionSweepLimit caps the drain rate at 100 candidates per
	// 5-minute sweep; a larger backlog drains gradually by design.
	defaultRetentionSweepLimit = 100
	// failedRecordingRetention is how long a failed session's chunks are kept.
	// The activity feed keeps failed sessions visible for the same window so a
	// user always has a chance to see a lost recording before it is shredded.
	failedRecordingRetention = 48 * time.Hour
	// interruptedRecordingRetention is how long an interrupted session's chunks
	// are kept before the recovery option expires.
	interruptedRecordingRetention = 7 * 24 * time.Hour
	// stuckCompletingThreshold is how long a session may sit in 'completing'
	// before the periodic sweep resolves it. Comfortably above the 60-minute
	// stitch job timeout so a slow-but-alive stitch is never pre-empted.
	stuckCompletingThreshold = 2 * time.Hour
)

// RetentionSweepResult summarizes one live recording audio retention sweep.
type RetentionSweepResult struct {
	CandidatesScanned int
	Deleted           int
	// AlreadyMissing counts candidates whose storage object was already gone.
	AlreadyMissing int
	// AlreadyProcessed counts candidates whose MarkAudioDeleted matched no row
	// (a concurrent sweep already processed them or the storage key changed).
	// Benign: not counted as a failure.
	AlreadyProcessed int
	DeleteFailures   int
	MarkFailures     int
	OldestDueAt      *time.Time
}

// RecordingPolicy configures runtime recording constraints.
type RecordingPolicy struct {
	MaxDuration          time.Duration
	MaxChunksPerSession  int
	OrphanThreshold      time.Duration
	StitchServiceTimeout time.Duration
}

// RecordingServiceOption configures RecordingService construction.
type RecordingServiceOption func(*RecordingService)

// WithRecordingStorageQuota enables durable chunk accounting for bucket-like
// storage. Local filesystem and memory recordings remain unlimited.
func WithRecordingStorageQuota(repo port.StorageQuotaRepository, backend string) RecordingServiceOption {
	return func(s *RecordingService) {
		s.quota = repo
		s.quotaApplies = repo != nil && domain.IsBucketStorageBackend(backend)
	}
}

// WithRecordingPolicy sets recording policy limits and thresholds.
func WithRecordingPolicy(policy RecordingPolicy) RecordingServiceOption {
	return func(s *RecordingService) {
		s.policy = normalizeRecordingPolicy(policy)
	}
}

func normalizeRecordingPolicy(policy RecordingPolicy) RecordingPolicy {
	normalized := policy
	if normalized.MaxDuration <= 0 {
		normalized.MaxDuration = defaultRecordingMaxDuration
	}
	if normalized.MaxChunksPerSession <= 0 {
		normalized.MaxChunksPerSession = defaultRecordingMaxChunksPerSess
	}
	if normalized.MaxChunksPerSession > domain.MaxChunksPerSession {
		normalized.MaxChunksPerSession = domain.MaxChunksPerSession
	}
	if normalized.OrphanThreshold <= 0 {
		normalized.OrphanThreshold = defaultRecordingOrphanThreshold
	}
	if normalized.StitchServiceTimeout <= 0 {
		normalized.StitchServiceTimeout = defaultStitchServiceTimeout
	}
	return normalized
}

// NewRecordingService creates a new RecordingService.
func NewRecordingService(
	recordingRepo port.RecordingRepository,
	chunkRepo port.ChunkRepository,
	mediaSvc *MediaService,
	storage port.StorageClient,
	envelope *crypto.EnvelopeService,
	prober port.AudioProber,
	stitcher port.AudioStitcher,
	jobInserter port.RecordingJobInserter,
	logger *slog.Logger,
	opts ...RecordingServiceOption,
) *RecordingService {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &RecordingService{
		recordingRepo: recordingRepo,
		chunkRepo:     chunkRepo,
		mediaSvc:      mediaSvc,
		storage:       storage,
		envelope:      envelope,
		prober:        prober,
		stitcher:      stitcher,
		jobInserter:   jobInserter,
		logger:        logger,
		policy:        normalizeRecordingPolicy(RecordingPolicy{}),
		now:           time.Now,
	}
	if mediaSvc != nil {
		svc.deleteMedia = mediaSvc.Delete
		svc.setRecordingMode = mediaSvc.SetRecordingMode
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// SetJobInserter updates the job inserter used for background stitch jobs.
func (s *RecordingService) SetJobInserter(jobInserter port.RecordingJobInserter) {
	s.jobInserter = jobInserter
}

// SetMalwareScanner configures the pre-stitch scanner. A nil scanner preserves
// the optional-development behavior; configured scanners fail closed.
func (s *RecordingService) SetMalwareScanner(scanner port.MalwareScanner) {
	s.scanner = scanner
}

func (s *RecordingService) withSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if locker, ok := s.recordingRepo.(port.RecordingSessionLocker); ok {
		return locker.WithSessionLock(ctx, sessionID, fn)
	}
	return fn(ctx)
}

func (s *RecordingService) trySessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if locker, ok := s.recordingRepo.(port.RecordingSessionTryLocker); ok {
		return locker.TryWithSessionLock(ctx, sessionID, fn)
	}
	return s.withSessionLock(ctx, sessionID, fn)
}

// CreateSession creates a new regular-mode recording session.
// For privilege mode, use CreateSessionWithMode.
func (s *RecordingService) CreateSession(ctx context.Context, orgID, userID, mimeType, micLabel string) (*domain.RecordingSession, error) {
	return s.CreateSessionWithMode(ctx, orgID, userID, mimeType, micLabel, domain.RecordingModeRegular)
}

// CreateSessionWithMode is like CreateSession but accepts a recording mode.
func (s *RecordingService) CreateSessionWithMode(ctx context.Context, orgID, userID, mimeType, micLabel string, mode domain.RecordingMode) (*domain.RecordingSession, error) {
	return s.CreateSessionWithModeAndCaptureSource(ctx, orgID, userID, mimeType, micLabel, mode, domain.RecordingCaptureSourceMicrophone)
}

// CreateSessionWithModeAndCaptureSource creates a recording session with explicit mode and capture source.
func (s *RecordingService) CreateSessionWithModeAndCaptureSource(
	ctx context.Context,
	orgID, userID, mimeType, micLabel string,
	mode domain.RecordingMode,
	captureSource domain.RecordingCaptureSource,
) (*domain.RecordingSession, error) {
	// Check interrupted session limit.
	count, err := s.recordingRepo.CountInterruptedByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count interrupted sessions: %w", err)
	}
	if count >= domain.MaxInterruptedPerUser {
		return nil, fmt.Errorf("too many interrupted sessions (%d/%d), recover or discard before starting: %w",
			count, domain.MaxInterruptedPerUser, domain.ErrConflict)
	}

	session, err := domain.NewRecordingSessionWithModeAndCaptureSource(orgID, userID, mimeType, micLabel, mode, captureSource)
	if err != nil {
		return nil, fmt.Errorf("create recording session: %w", err)
	}

	// Create will fail with ErrConflict if user already has an active session
	// (enforced by unique partial index).
	if err := s.recordingRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("persist recording session: %w", err)
	}

	s.logger.Info("recording session created",
		"session_id", session.ID,
		"org_id", orgID,
		"user_id", userID,
		"mime_type", mimeType,
		"recording_mode", mode,
		"capture_source", session.CaptureSource,
	)

	return session, nil
}

// UploadChunk validates, encrypts, and stores a single audio chunk.
func (s *RecordingService) UploadChunk(
	ctx context.Context,
	sessionID, userID string,
	seq int,
	data io.Reader,
	size int64,
	contentType string,
) error {
	return s.trySessionLock(ctx, sessionID, func(lockCtx context.Context) error {
		return s.uploadChunk(lockCtx, sessionID, userID, seq, data, contentType)
	})
}

func (s *RecordingService) uploadChunk(
	ctx context.Context,
	sessionID, userID string,
	seq int,
	data io.Reader,
	contentType string,
) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	if validateErr := s.validateChunkUpload(ctx, session, userID, seq, contentType); validateErr != nil {
		return validateErr
	}

	// Read chunk data (bounded by MaxChunkSize).
	chunkData, readErr := s.readChunkData(data, seq, session.MimeType)
	if readErr != nil {
		return readErr
	}
	checksum := checksumChunk(chunkData)

	existing, existingErr := s.chunkRepo.GetBySessionAndSeq(ctx, session.ID, seq)
	if existingErr == nil && existing != nil {
		if isMatchingChunk(existing, checksum, int64(len(chunkData))) {
			return nil
		}
		return fmt.Errorf("chunk sequence %d conflicts with stored content: %w", seq, domain.ErrConflict)
	}
	if existingErr != nil && !errors.Is(existingErr, domain.ErrNotFound) {
		return fmt.Errorf("check existing chunk: %w", existingErr)
	}

	if sequenceErr := s.validateNextChunkSequence(ctx, session.ID, seq); sequenceErr != nil {
		return sequenceErr
	}
	if quotaErr := s.validateChunkQuota(ctx, session.ID, int64(len(chunkData))); quotaErr != nil {
		return quotaErr
	}
	if s.quotaApplies {
		if err := s.quota.ReserveRecordingChunk(ctx, session, seq, int64(len(chunkData))); err != nil {
			return fmt.Errorf("reserve recording chunk storage: %w", err)
		}
	}

	// Encrypt, upload, and persist the chunk.
	if storeErr := s.encryptAndStoreChunk(ctx, session, seq, chunkData, checksum); storeErr != nil {
		return storeErr
	}

	s.logger.Info("chunk uploaded",
		"session_id", sessionID,
		"seq", seq,
		"size", len(chunkData),
	)

	return nil
}

// CleanupRetainedAudio deletes and crypto-shreds live recording audio that is due for retention cleanup.
func (s *RecordingService) CleanupRetainedAudio(ctx context.Context) (RetentionSweepResult, error) {
	var result RetentionSweepResult
	if s.mediaSvc == nil || s.storage == nil {
		return result, nil
	}
	now := s.now()
	candidates, err := s.recordingRepo.ListLiveRecordingAudioRetentionCandidates(ctx, now, defaultRetentionSweepLimit)
	if err != nil {
		return result, fmt.Errorf("list retention candidates: %w", err)
	}
	result.CandidatesScanned = len(candidates)
	for _, candidate := range candidates {
		if result.OldestDueAt == nil || candidate.CompletedAt.Before(*result.OldestDueAt) {
			t := candidate.CompletedAt
			result.OldestDueAt = &t
		}
		s.sweepRetentionCandidate(ctx, candidate, now, &result)
	}
	if result.DeleteFailures > 0 || result.MarkFailures > 0 {
		return result, fmt.Errorf("retention cleanup failed: delete_failures=%d mark_failures=%d", result.DeleteFailures, result.MarkFailures)
	}
	return result, nil
}

// sweepRetentionCandidate deletes one candidate's audio object, crypto-shreds
// its media encryption metadata, and updates the sweep counters in place.
func (s *RecordingService) sweepRetentionCandidate(
	ctx context.Context,
	candidate port.LiveRecordingAudioRetentionCandidate,
	now time.Time,
	result *RetentionSweepResult,
) {
	deleteErr := s.storage.Delete(ctx, candidate.StorageKey)
	if deleteErr != nil && !errors.Is(deleteErr, domain.ErrNotFound) {
		result.DeleteFailures++
		s.logger.Error("failed to delete retained live recording audio",
			"error", deleteErr,
			"org_id", candidate.OrganizationID,
			"session_id", candidate.SessionID,
			"media_id", candidate.MediaID,
		)
		return
	}
	if errors.Is(deleteErr, domain.ErrNotFound) {
		result.AlreadyMissing++
	}
	if _, markErr := s.mediaSvc.MarkAudioDeleted(ctx, candidate.MediaID, candidate.StorageKey, now); markErr != nil {
		// ErrConflict means no matching row: another sweep already processed
		// this media or its storage key changed. Benign — do not fail the job.
		if errors.Is(markErr, domain.ErrConflict) {
			result.AlreadyProcessed++
			s.logger.Info("retained live recording audio already processed, skipping",
				"org_id", candidate.OrganizationID,
				"session_id", candidate.SessionID,
				"media_id", candidate.MediaID,
			)
			return
		}
		result.MarkFailures++
		s.logger.Error("failed to mark retained live recording audio deleted",
			"error", markErr,
			"org_id", candidate.OrganizationID,
			"session_id", candidate.SessionID,
			"media_id", candidate.MediaID,
		)
		return
	}
	result.Deleted++
	s.logger.Info("retained live recording audio deleted",
		"org_id", candidate.OrganizationID,
		"session_id", candidate.SessionID,
		"media_id", candidate.MediaID,
		"already_missing", errors.Is(deleteErr, domain.ErrNotFound),
	)
}

// validateChunkUpload validates ownership, state, range, and content type.
func (s *RecordingService) validateChunkUpload(
	ctx context.Context,
	session *domain.RecordingSession,
	userID string,
	seq int,
	contentType string,
) error {
	if session.UserID != userID {
		// 404, not 403: a foreign session ID must be indistinguishable from a
		// nonexistent one, matching media/transcription/summary lookups.
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}
	if !session.AcceptsChunks() {
		return fmt.Errorf("session in %q state does not accept chunks: %w", session.Status, domain.ErrInvalidInput)
	}
	if session.Status != domain.RecordingStatusInterrupted && !session.CreatedAt.IsZero() {
		maxAllowedAt := session.CreatedAt.Add(s.policy.MaxDuration)
		if s.now().After(maxAllowedAt) {
			return fmt.Errorf("session exceeded max duration of %s: %w", s.policy.MaxDuration, domain.ErrInvalidInput)
		}
	}
	if seq < 0 || seq >= s.policy.MaxChunksPerSession {
		return fmt.Errorf("chunk sequence %d out of range [0, %d): %w", seq, s.policy.MaxChunksPerSession, domain.ErrInvalidInput)
	}
	normalized, err := normalizeMediaType(contentType)
	if err != nil {
		return fmt.Errorf("parse content type %q: %w", contentType, domain.ErrInvalidInput)
	}
	if normalized != session.MimeType {
		return fmt.Errorf("content type %q does not match session type %q: %w", contentType, session.MimeType, domain.ErrInvalidInput)
	}

	return nil
}

func (s *RecordingService) validateNextChunkSequence(ctx context.Context, sessionID string, seq int) error {
	maxSeq, maxErr := s.chunkRepo.MaxSeqBySession(ctx, sessionID)
	if maxErr != nil && !errors.Is(maxErr, domain.ErrNotFound) {
		return fmt.Errorf("get max seq: %w", maxErr)
	}
	if expectedSeq := maxSeq + 1; seq != expectedSeq {
		return fmt.Errorf("out-of-order chunk: expected seq %d, got %d: %w", expectedSeq, seq, domain.ErrConflict)
	}
	return nil
}

func (s *RecordingService) validateChunkQuota(ctx context.Context, sessionID string, chunkBytes int64) error {
	sizer, ok := s.chunkRepo.(port.RecordingChunkSizer)
	if !ok {
		return fmt.Errorf("recording chunk quota is not configured: %w", domain.ErrInternal)
	}
	total, err := sizer.TotalPlaintextSizeBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get recording chunk total: %w", err)
	}
	if total > domain.MaxRecordingSessionBytes-chunkBytes {
		return fmt.Errorf("recording exceeds maximum encoded size of %d bytes: %w", domain.MaxRecordingSessionBytes, domain.ErrInvalidInput)
	}
	return nil
}

// readChunkData reads and validates chunk data from the reader.
func (s *RecordingService) readChunkData(data io.Reader, seq int, mimeType string) ([]byte, error) {
	limitedReader := io.LimitReader(data, int64(domain.MaxChunkSize)+1)
	chunkData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read chunk data: %w", err)
	}
	if int64(len(chunkData)) > int64(domain.MaxChunkSize) {
		return nil, fmt.Errorf("chunk exceeds maximum size of %d bytes: %w", domain.MaxChunkSize, domain.ErrInvalidInput)
	}
	if seq == 0 {
		if validateErr := domain.ValidateFirstChunk(mimeType, chunkData); validateErr != nil {
			return nil, fmt.Errorf("validate first chunk: %w", validateErr)
		}
	}
	return chunkData, nil
}

// encryptAndStoreChunk encrypts chunk data, uploads to storage, and persists
// the manifest. The session's recording mode namespaces the object key, which
// is what routes a privilege session's chunks into the shred bucket.
func (s *RecordingService) encryptAndStoreChunk(ctx context.Context, session *domain.RecordingSession, seq int, chunkData []byte, checksum string) error {
	orgID, sessionID := session.OrganizationID, session.ID
	storageKey := domain.ChunkStorageKey(orgID, sessionID, seq, session.RecordingMode)

	// Encrypt via SealStream.
	sealMeta, err := s.sealAndUpload(ctx, orgID, storageKey, chunkData)
	if err != nil {
		if s.deleteUnmanifestedChunk(ctx, storageKey, err) && s.quotaApplies {
			if releaseErr := s.quota.ReleaseRecordingChunk(ctx, sessionID, seq); releaseErr != nil {
				s.logger.Error("failed to release chunk quota after upload failure", "session_id", sessionID, "seq", seq, "error", releaseErr)
			}
		}
		return err
	}

	// Persist chunk manifest.
	chunk, err := domain.NewRecordingChunk(sessionID, seq, storageKey)
	if err != nil {
		if s.deleteUnmanifestedChunk(ctx, storageKey, err) && s.quotaApplies {
			if releaseErr := s.quota.ReleaseRecordingChunk(ctx, sessionID, seq); releaseErr != nil {
				s.logger.Error("failed to release invalid chunk quota", "session_id", sessionID, "seq", seq, "error", releaseErr)
			}
		}
		return fmt.Errorf("create chunk entity: %w", err)
	}
	chunk.SetEncryptionMeta(
		sealMeta.WrappedDEK,
		sealMeta.WrappingNonce,
		sealMeta.Algorithm,
		sealMeta.ChunkSize,
		sealMeta.ChunkCount,
		int64(len(chunkData)),
		checksum,
	)

	// The advisory session lock and prior checksum check make this insert the
	// first writer. Never upsert here: replacing an existing manifest could pair
	// one encrypted object with another chunk's DEK.
	var persistErr error
	if s.quotaApplies {
		persistErr = s.quota.FinalizeRecordingChunk(ctx, chunk)
	} else {
		persistErr = s.persistChunkManifest(ctx, chunk)
	}
	if persistErr != nil {
		// A transaction can commit before its response is lost. Keep both the
		// object and its reservation until a retry proves the outcome instead of
		// risking deletion or an unmetered object on an ambiguous failure.
		return fmt.Errorf("persist chunk manifest: %w", persistErr)
	}

	// Update last chunk timestamp.
	if err := s.recordingRepo.UpdateLastChunkAt(ctx, sessionID, s.now()); err != nil {
		s.logger.Warn("failed to update last_chunk_at", "session_id", sessionID, "error", err)
	}

	return nil
}

func (s *RecordingService) persistChunkManifest(ctx context.Context, chunk *domain.RecordingChunk) error {
	createErr := s.chunkRepo.Create(ctx, chunk)
	if createErr == nil {
		return nil
	}

	// PostgreSQL can commit and then lose the response. Never delete the
	// deterministic object on an ambiguous insert error: an exact reread proves
	// success, while any other outcome is left for client retry or prefix cleanup.
	stored, readErr := s.chunkRepo.GetBySessionAndSeq(ctx, chunk.SessionID, chunk.Seq)
	if readErr == nil && isSameChunkManifest(stored, chunk) {
		return nil
	}
	if readErr != nil && !errors.Is(readErr, domain.ErrNotFound) {
		s.logger.Warn("failed to resolve recording manifest insert outcome",
			"session_id", chunk.SessionID,
			"seq", chunk.Seq,
			"error", readErr,
		)
	}
	return fmt.Errorf("persist chunk manifest: %w", createErr)
}

func (s *RecordingService) deleteUnmanifestedChunk(ctx context.Context, storageKey string, persistErr error) bool {
	if deleteErr := s.storage.Delete(ctx, storageKey); deleteErr != nil && !errors.Is(deleteErr, domain.ErrNotFound) {
		s.logger.Error("failed to delete unmanifested recording chunk",
			"storage_key", storageKey,
			"persist_error", persistErr,
			"delete_error", deleteErr,
		)
		return false
	}
	return true
}

func checksumChunk(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func isMatchingChunk(chunk *domain.RecordingChunk, checksum string, plaintextSize int64) bool {
	return chunk != nil && chunk.Checksum == checksum && chunk.PlaintextSize == plaintextSize
}

func isSameChunkManifest(a, b *domain.RecordingChunk) bool {
	return a != nil && b != nil &&
		a.SessionID == b.SessionID && a.Seq == b.Seq && a.StorageKey == b.StorageKey &&
		a.EncryptionAlgo == b.EncryptionAlgo && a.ChunkSize == b.ChunkSize &&
		a.ChunkCount == b.ChunkCount && a.PlaintextSize == b.PlaintextSize &&
		a.Checksum == b.Checksum && bytes.Equal(a.WrappedDEK, b.WrappedDEK) &&
		bytes.Equal(a.WrappingNonce, b.WrappingNonce)
}

// sealAndUpload encrypts data with SealStream and uploads to storage in parallel.
func (s *RecordingService) sealAndUpload(ctx context.Context, orgID, storageKey string, data []byte) (*crypto.SealedMeta, error) {
	pr, pw := io.Pipe()
	type sealResult struct {
		meta *crypto.SealedMeta
		err  error
	}
	sealCh := make(chan sealResult, 1)

	go func() {
		src := io.NopCloser(io.NewSectionReader(newBytesReaderAt(data), 0, int64(len(data))))
		meta, sealErr := s.envelope.SealStream(ctx, orgID, src, pw)
		if sealErr != nil {
			_ = pw.CloseWithError(sealErr) //nolint:errcheck // pipe close cannot fail
			sealCh <- sealResult{err: sealErr}
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close cannot fail
		sealCh <- sealResult{meta: meta}
	}()

	uploadErr := s.storage.Upload(ctx, storageKey, pr, "application/octet-stream", nil)
	_ = pr.Close() //nolint:errcheck // ensure goroutine completes
	sealRes := <-sealCh

	if sealRes.err != nil {
		return nil, fmt.Errorf("encrypt chunk: %w", sealRes.err)
	}
	if uploadErr != nil {
		return nil, fmt.Errorf("upload chunk to storage: %w", uploadErr)
	}
	return sealRes.meta, nil
}

// CompleteSession transitions a session to completing and enqueues stitch.
func (s *RecordingService) CompleteSession(ctx context.Context, sessionID, userID string) error {
	return s.trySessionLock(ctx, sessionID, func(lockCtx context.Context) error {
		return s.completeSession(lockCtx, sessionID, userID)
	})
}

func (s *RecordingService) completeSession(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}

	originalStatus := session.Status
	if err := session.Complete(); err != nil {
		return fmt.Errorf("complete session: %w", err)
	}

	if err := s.recordingRepo.UpdateStatus(ctx, sessionID, originalStatus, domain.RecordingStatusCompleting); err != nil {
		return err
	}

	if err := s.enqueueStitchJob(ctx, sessionID); err != nil {
		// Best-effort rollback to avoid leaving the session stuck in completing.
		if rbErr := s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusCompleting, originalStatus); rbErr != nil {
			s.logger.Warn("failed to rollback recording status after enqueue failure",
				"session_id", sessionID,
				"error", rbErr,
			)
		}
		return err
	}
	return nil
}

// PauseSession pauses an active recording.
func (s *RecordingService) PauseSession(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}

	if err := session.Pause(); err != nil {
		return err
	}

	return s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusRecording, domain.RecordingStatusPaused)
}

// ResumeSession resumes a paused recording.
func (s *RecordingService) ResumeSession(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}

	if err := session.Resume(); err != nil {
		return err
	}

	return s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusPaused, domain.RecordingStatusRecording)
}

// RecoverSession transitions an interrupted session to completing for stitching.
func (s *RecordingService) RecoverSession(ctx context.Context, sessionID, userID string) error {
	return s.trySessionLock(ctx, sessionID, func(lockCtx context.Context) error {
		return s.recoverSession(lockCtx, sessionID, userID)
	})
}

func (s *RecordingService) recoverSession(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}

	if err := session.Recover(); err != nil {
		return err
	}

	if err := s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusInterrupted, domain.RecordingStatusCompleting); err != nil {
		return err
	}

	if err := s.enqueueStitchJob(ctx, sessionID); err != nil {
		// Best-effort rollback to avoid leaving the session stuck in completing.
		if rbErr := s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusCompleting, domain.RecordingStatusInterrupted); rbErr != nil {
			s.logger.Warn("failed to rollback recovered recording status after enqueue failure",
				"session_id", sessionID,
				"error", rbErr,
			)
		}
		return err
	}
	return nil
}

// AbandonSession transitions an interrupted session to abandoned and cleans up.
func (s *RecordingService) AbandonSession(ctx context.Context, sessionID, userID string) error {
	return s.trySessionLock(ctx, sessionID, func(lockCtx context.Context) error {
		return s.abandonSession(lockCtx, sessionID, userID)
	})
}

func (s *RecordingService) abandonSession(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}

	if session.Status == domain.RecordingStatusAbandoned {
		return s.cleanupChunks(ctx, session)
	}
	if err := session.Abandon(); err != nil {
		return err
	}

	if err := s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusInterrupted, domain.RecordingStatusAbandoned); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	return s.cleanupChunks(ctx, session)
}

// GetSession returns a recording session for the owner.
func (s *RecordingService) GetSession(ctx context.Context, sessionID, userID string) (*domain.RecordingSession, error) {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return nil, fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}
	s.attachChunkCount(ctx, session)
	return session, nil
}

// GetActiveSession returns the caller's own active (recording or paused) session.
// Returns domain.ErrNotFound when the user has none.
func (s *RecordingService) GetActiveSession(ctx context.Context, userID string) (*domain.RecordingSession, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty: %w", domain.ErrInvalidInput)
	}
	session, err := s.recordingRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get active session: %w", err)
	}
	s.attachChunkCount(ctx, session)
	return session, nil
}

// ReleaseSession lets the owner hand an active session to the recovery flow
// immediately (recording|paused -> interrupted) instead of waiting for the
// orphan sweep. Returns domain.ErrConflict if the session is not active.
func (s *RecordingService) ReleaseSession(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}

	fromStatus := session.Status
	if err := session.Release(); err != nil {
		return err
	}

	if err := s.recordingRepo.UpdateStatus(ctx, sessionID, fromStatus, domain.RecordingStatusInterrupted); err != nil {
		return fmt.Errorf("release session: %w", err)
	}
	s.logger.Info("recording session released by user",
		"session_id", sessionID,
		"user_id", userID,
		"from_status", fromStatus,
	)
	return nil
}

// GetInterrupted returns interrupted sessions for a user.
func (s *RecordingService) GetInterrupted(ctx context.Context, userID string) ([]domain.RecordingSession, error) {
	sessions, err := s.recordingRepo.GetInterruptedByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// One count per row. Bounded by domain.MaxInterruptedPerUser (3), which
	// CreateSession enforces before a new session can become interrupted.
	for i := range sessions {
		s.attachChunkCount(ctx, &sessions[i])
	}
	return sessions, nil
}

// attachChunkCount populates the derived ChunkCount on a session the client will
// see. The recovery dialog needs it to estimate the recoverable duration. A
// failed count is not worth failing the read over.
func (s *RecordingService) attachChunkCount(ctx context.Context, session *domain.RecordingSession) {
	if session == nil || s.chunkRepo == nil {
		return
	}
	count, err := s.chunkRepo.CountBySession(ctx, session.ID)
	if err != nil {
		s.logger.Warn("failed to count session chunks", "session_id", session.ID, "error", err)
		return
	}
	session.ChunkCount = count
}

// Heartbeat marks activity for an active recording session.
func (s *RecordingService) Heartbeat(ctx context.Context, sessionID, userID string) error {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.UserID != userID {
		return fmt.Errorf("session ownership mismatch: %w", domain.ErrNotFound)
	}
	if !session.IsActive() {
		return fmt.Errorf("session in %q state does not accept heartbeat: %w", session.Status, domain.ErrInvalidInput)
	}
	if err := s.recordingRepo.UpdateLastActivityAt(ctx, sessionID, s.now()); err != nil {
		return fmt.Errorf("update last activity: %w", err)
	}
	return nil
}

func (s *RecordingService) enqueueStitchJob(ctx context.Context, sessionID string) error {
	if s.jobInserter == nil {
		return fmt.Errorf("recording job inserter not configured: %w", domain.ErrInternal)
	}
	if err := s.jobInserter.InsertStitchJob(ctx, sessionID); err != nil {
		return fmt.Errorf("enqueue stitch job: %w", err)
	}
	return nil
}

// DetectOrphans finds stale recording sessions and marks them as interrupted.
func (s *RecordingService) DetectOrphans(ctx context.Context) error {
	stale, err := s.recordingRepo.FindStaleRecording(ctx, s.policy.OrphanThreshold)
	if err != nil {
		return fmt.Errorf("find stale recordings: %w", err)
	}

	for i := range stale {
		session := &stale[i]
		// CAS from the observed status: the sweep covers both 'recording' and
		// 'paused', and a concurrent client transition must lose the race.
		if err := s.recordingRepo.UpdateStatus(ctx, session.ID, session.Status, domain.RecordingStatusInterrupted); err != nil {
			s.logger.Warn("failed to mark session as interrupted",
				"session_id", session.ID,
				"from_status", session.Status,
				"error", err,
			)
			continue
		}
		s.logger.Info("marked stale recording as interrupted",
			"session_id", session.ID,
			"user_id", session.UserID,
			"from_status", session.Status,
		)
	}

	return nil
}

// SweepStuckCompleting recovers sessions stranded in 'completing' because their
// stitch job was lost between the status change and the River insert. A session
// with media only missed finalization; a session without media is re-enqueued.
func (s *RecordingService) SweepStuckCompleting(ctx context.Context) error {
	threshold := s.now().Add(-stuckCompletingThreshold)
	sessions, err := s.recordingRepo.ListByStatusOlderThan(ctx, domain.RecordingStatusCompleting, threshold)
	if err != nil {
		return fmt.Errorf("list stuck completing recordings: %w", err)
	}

	var firstErr error
	for i := range sessions {
		if resolveErr := s.resolveStuckCompleting(ctx, sessions[i], threshold); resolveErr != nil {
			s.logger.Warn("failed to resolve stuck completing recording",
				"session_id", sessions[i].ID,
				"error", resolveErr,
			)
			if firstErr == nil {
				firstErr = resolveErr
			}
		}
	}
	return firstErr
}

// resolveStuckCompleting rechecks the row while holding the same lock as stitch
// and upload. A fresh job/status transition is therefore never overwritten by
// this stale sweep result.
func (s *RecordingService) resolveStuckCompleting(ctx context.Context, listed domain.RecordingSession, threshold time.Time) error {
	return s.withSessionLock(ctx, listed.ID, func(lockCtx context.Context) error {
		session, err := s.recordingRepo.GetByID(lockCtx, listed.ID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("re-read stuck completing session: %w", err)
		}
		if session.Status != domain.RecordingStatusCompleting || !session.UpdatedAt.Before(threshold) {
			return nil
		}
		if session.MediaID == "" {
			if err := s.enqueueStitchJob(lockCtx, session.ID); err != nil {
				return err
			}
			s.logger.Info("re-enqueued stuck completing recording", "session_id", session.ID, "user_id", session.UserID)
			return nil
		}
		if err := s.finalizeSession(lockCtx, session, session.ID, session.TotalDuration); err != nil {
			return err
		}
		s.logger.Info("completed stuck recording with existing media", "session_id", session.ID, "media_id", session.MediaID)
		return nil
	})
}

// MarkFailed transitions a session from completing to failed.
func (s *RecordingService) MarkFailed(ctx context.Context, sessionID string) error {
	if err := s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusCompleting, domain.RecordingStatusFailed); err != nil {
		return fmt.Errorf("mark recording failed: %w", err)
	}
	return nil
}

// CleanupExpired enforces retention policy by deleting chunk artifacts for stale sessions.
func (s *RecordingService) CleanupExpired(ctx context.Context) error {
	cleanupByStatus := []struct {
		status    string
		retention time.Duration
	}{
		{status: domain.RecordingStatusFailed, retention: failedRecordingRetention},
		{status: domain.RecordingStatusInterrupted, retention: interruptedRecordingRetention},
		{status: domain.RecordingStatusAbandoned, retention: 0},
	}

	now := s.now()
	var firstErr error
	for _, rule := range cleanupByStatus {
		threshold := now.Add(-rule.retention)
		sessions, err := s.recordingRepo.ListByStatusOlderThan(ctx, rule.status, threshold)
		if err != nil {
			return fmt.Errorf("list %s recordings for cleanup: %w", rule.status, err)
		}

		for i := range sessions {
			if cleanupErr := s.cleanupExpiredSession(ctx, sessions[i].ID, rule.status, threshold); cleanupErr != nil && firstErr == nil {
				firstErr = cleanupErr
			}
		}
	}
	if lister, ok := s.recordingRepo.(port.RecordingChunkCleanupLister); ok {
		sessions, err := lister.ListWithChunksByStatusOlderThan(ctx, domain.RecordingStatusCompleted, now, cleanupSessionsLimit)
		if err != nil {
			return fmt.Errorf("list completed recordings with chunks: %w", err)
		}
		for i := range sessions {
			if cleanupErr := s.cleanupExpiredSession(ctx, sessions[i].ID, domain.RecordingStatusCompleted, now); cleanupErr != nil && firstErr == nil {
				firstErr = cleanupErr
			}
		}
	}
	return firstErr
}

// cleanupExpiredSession rechecks lifecycle state while holding the same lock
// used by recovery. A listed interrupted session that was just recovered must
// never lose its chunks to a stale retention sweep.
func (s *RecordingService) cleanupExpiredSession(ctx context.Context, sessionID, status string, threshold time.Time) error {
	return s.withSessionLock(ctx, sessionID, func(lockCtx context.Context) error {
		session, err := s.recordingRepo.GetByID(lockCtx, sessionID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("re-read recording for cleanup: %w", err)
		}
		if session.Status != status || !session.UpdatedAt.Before(threshold) {
			return nil
		}
		return s.cleanupChunks(lockCtx, session)
	})
}

// Stitch decrypts and concatenates all chunks for a session into a Media entity.
func (s *RecordingService) Stitch(ctx context.Context, sessionID string) (string, error) {
	var mediaID string
	err := s.withSessionLock(ctx, sessionID, func(lockCtx context.Context) error {
		var stitchErr error
		mediaID, stitchErr = s.stitch(lockCtx, sessionID)
		return stitchErr
	})
	return mediaID, err
}

func (s *RecordingService) stitch(ctx context.Context, sessionID string) (string, error) {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	if session.Status == domain.RecordingStatusCompleted && session.MediaID != "" {
		if cleanupErr := s.cleanupChunks(ctx, session); cleanupErr != nil {
			return "", cleanupErr
		}
		return session.MediaID, nil
	}
	if session.Status != domain.RecordingStatusCompleting {
		return "", fmt.Errorf("session not in completing state: %w", domain.ErrInvalidInput)
	}

	mediaID, resumed, err := s.resumeCompletedStitch(ctx, session, sessionID)
	if err != nil {
		return "", err
	}
	if resumed {
		return mediaID, nil
	}

	return s.runStitchPipeline(ctx, session, sessionID)
}

// resumeCompletedStitch covers the idempotency-recovery paths that finalize a
// previous attempt's media instead of re-running the pipeline.
//
// Idempotency anchor: a previous attempt already created and anchored the
// media object, so re-running the pipeline would duplicate it. Finalize
// only. (This protects every retry after the anchor is persisted; a crash
// between upload and anchor is compensated in createMediaFromStitched.)
func (s *RecordingService) resumeCompletedStitch(
	ctx context.Context,
	session *domain.RecordingSession,
	sessionID string,
) (mediaID string, resumed bool, err error) {
	if session.MediaID != "" {
		s.logger.Info("recording already has media, finalizing only",
			"session_id", sessionID,
			"media_id", session.MediaID,
		)
		if finErr := s.finalizeSession(ctx, session, sessionID, session.TotalDuration); finErr != nil {
			return "", false, finErr
		}
		return session.MediaID, true, nil
	}
	if !s.quotaApplies {
		return "", false, nil
	}
	mediaID, found, findErr := s.quota.FindCommittedStitchedMedia(ctx, session.ID)
	if findErr != nil {
		return "", false, fmt.Errorf("find committed stitched media: %w", findErr)
	}
	if !found {
		return "", false, nil
	}
	if anchorErr := s.anchorStitchedMedia(ctx, session, mediaID); anchorErr != nil {
		return "", false, anchorErr
	}
	if finErr := s.finalizeSession(ctx, session, sessionID, session.TotalDuration); finErr != nil {
		return "", false, finErr
	}
	return mediaID, true, nil
}

// runStitchPipeline runs the full stitch pipeline: load chunks, decrypt and
// concatenate into a working dir, create the Media, and finalize the session.
func (s *RecordingService) runStitchPipeline(
	ctx context.Context,
	session *domain.RecordingSession,
	sessionID string,
) (string, error) {
	chunks, err := s.loadAndValidateChunks(ctx, sessionID)
	if err != nil {
		return "", err
	}

	cleanupStaleStitchDirs([]string{devShmRoot, os.TempDir()}, s.now(), defaultStitchDirMaxAge, s.logger)
	estimatedWorkingBytes := estimateStitchWorkingBytes(chunks)

	tmpDir, err := createStitchTmpDir(estimatedWorkingBytes)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup
	}()

	// Decrypt, concatenate, and optionally convert.
	finalPath, err := s.stitchToFile(ctx, session, chunks, tmpDir)
	if err != nil {
		return "", err
	}

	// Create Media and finalize session.
	mediaID, duration, err := s.createMediaFromStitched(ctx, session, finalPath)
	if err != nil {
		return "", err
	}

	if err := s.finalizeSession(ctx, session, sessionID, duration); err != nil {
		return "", err
	}

	s.logger.Info("recording stitched successfully",
		"session_id", sessionID,
		"media_id", mediaID,
		"chunks", len(chunks),
		"duration", duration,
	)
	return mediaID, nil
}

// loadAndValidateChunks loads and validates chunk sequence integrity.
func (s *RecordingService) loadAndValidateChunks(ctx context.Context, sessionID string) ([]domain.RecordingChunk, error) {
	chunks, err := s.chunkRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks found for session: %w", domain.ErrInvalidInput)
	}
	for i, chunk := range chunks {
		if chunk.Seq != i {
			return nil, fmt.Errorf("sequence gap: expected seq %d, got %d: %w", i, chunk.Seq, domain.ErrInvalidInput)
		}
	}
	return chunks, nil
}

func estimateStitchWorkingBytes(chunks []domain.RecordingChunk) int64 {
	var plaintextTotal int64
	for i := range chunks {
		plaintextTotal += max(chunks[i].PlaintextSize, 0)
	}

	// Browser continuation formats need the decrypted segments, an intermediate
	// concatenation, and the final WebM at once. Reserve all three payload-sized
	// files plus fixed ffmpeg/container overhead.
	outputEstimate := plaintextTotal * 2
	overhead := plaintextTotal / 2
	if overhead < defaultStitchOverheadBytes {
		overhead = defaultStitchOverheadBytes
	}

	total := plaintextTotal + outputEstimate + overhead
	if total <= 0 {
		return defaultStitchOverheadBytes * 3
	}
	return total
}

func chooseStitchRoot(
	requiredBytes int64,
	preferredRoot string,
	fallbackRoot string,
	freeSpaceFn func(string) (uint64, error),
) string {
	if preferredRoot == "" {
		return fallbackRoot
	}

	free, err := freeSpaceFn(preferredRoot)
	if err != nil {
		return fallbackRoot
	}

	if free > math.MaxInt64 {
		return preferredRoot
	}
	if int64(free) >= requiredBytes {
		return preferredRoot
	}
	return fallbackRoot
}

func freeSpaceBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	if stat.Bsize < 0 {
		return 0, fmt.Errorf("statfs returned negative block size %d for %s", stat.Bsize, path)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func cleanupStaleStitchDirs(roots []string, now time.Time, maxAge time.Duration, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	threshold := now.Add(-maxAge)
	for _, root := range roots {
		if root == "" {
			continue
		}

		pattern := filepath.Join(root, stitchDirPrefix+"*")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logger.Warn("failed to glob stitch dirs", "root", root, "error", err)
			continue
		}

		for _, path := range matches {
			info, statErr := os.Stat(path)
			if statErr != nil || !info.IsDir() {
				continue
			}
			if info.ModTime().Before(threshold) {
				if rmErr := os.RemoveAll(path); rmErr != nil {
					logger.Warn("failed to remove stale stitch dir", "path", path, "error", rmErr)
				}
			}
		}
	}
}

// createStitchTmpDir creates a fresh, private (0700) stitch working directory
// with an unpredictable name, preferring /dev/shm and falling back to disk tmp.
// It fails with domain.ErrInsufficientScratchSpace when neither can hold the
// estimate plus the scratch reserve.
func createStitchTmpDir(requiredBytes int64) (string, error) {
	root := chooseStitchRoot(requiredBytes+scratchReserveBytes, devShmRoot, os.TempDir(), freeSpaceBytes)
	if err := EnsureScratchSpace(root, requiredBytes); err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp(root, stitchDirPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("create stitch dir: %w", err)
	}
	return tmpDir, nil
}

// stitchToFile decrypts chunk files and stitches them into a single WebM/Opus file.
func (s *RecordingService) stitchToFile(
	ctx context.Context,
	session *domain.RecordingSession,
	chunks []domain.RecordingChunk,
	tmpDir string,
) (string, error) {
	switch session.MimeType {
	case domain.RecordingMimeWebM, domain.RecordingMimeMP4:
	default:
		return "", fmt.Errorf("unsupported recording mime type %q: %w", session.MimeType, domain.ErrInvalidInput)
	}

	if s.stitcher == nil || !s.stitcher.Available() {
		return "", fmt.Errorf("ffmpeg required for %s stitching but unavailable: %w", session.MimeType, domain.ErrInternal)
	}

	inputPaths, err := s.decryptChunksToFiles(ctx, session, chunks, tmpDir)
	if err != nil {
		return "", err
	}
	if err := s.scanDecryptedChunks(ctx, inputPaths); err != nil {
		return "", err
	}

	webmPath := filepath.Join(tmpDir, "stitched.webm")
	stitchCtx, cancel := context.WithTimeout(ctx, s.policy.StitchServiceTimeout)
	defer cancel()
	if stitchErr := s.stitcher.ConcatToWebMOpus(stitchCtx, inputPaths, webmPath); stitchErr != nil {
		return "", fmt.Errorf("concat to webm: %w", stitchErr)
	}

	return webmPath, nil
}

// openStitchedFile opens the stitched output and rejects files below the media
// minimum: a sub-second capture contains essentially no audio frames, media
// creation would reject it on every attempt, so the dedicated sentinel lets the
// worker fail the session without retries (and without probing garbage).
func openStitchedFile(finalPath string) (*os.File, int64, error) {
	file, err := os.Open(finalPath) //nolint:gosec // path is our own temp file
	if err != nil {
		return nil, 0, fmt.Errorf("open stitched file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close() //nolint:errcheck // best-effort
		return nil, 0, fmt.Errorf("stat stitched file: %w", err)
	}

	if info.Size() < domain.MinMediaSize {
		_ = file.Close() //nolint:errcheck // best-effort
		return nil, 0, fmt.Errorf("stitched output is %d bytes, below the %d-byte media minimum: %w",
			info.Size(), domain.MinMediaSize, domain.ErrRecordingTooShort)
	}

	return file, info.Size(), nil
}

// anchorStitchedMedia persists the media ID on the session immediately after
// upload: once anchored, a retried stitch skips the pipeline instead of
// uploading a duplicate. If the anchor write fails, the just-uploaded media is
// deleted so the retry does not leave a visible duplicate behind. A crash in
// the upload-to-anchor window is repaired by looking up the committed stitched
// reservation before another upload is attempted.
func (s *RecordingService) anchorStitchedMedia(
	ctx context.Context,
	session *domain.RecordingSession,
	mediaID string,
) error {
	if err := s.recordingRepo.SetMediaID(ctx, session.ID, mediaID); err != nil {
		if s.deleteMedia != nil {
			if delErr := s.deleteMedia(ctx, session.OrganizationID, mediaID); delErr != nil {
				s.logger.Error("failed to delete unanchored stitched media",
					"media_id", mediaID,
					"session_id", session.ID,
					"error", delErr,
				)
			}
		}
		return fmt.Errorf("set media ID on session %s: %w", session.ID, err)
	}
	session.MediaID = mediaID
	return nil
}

// createMediaFromStitched probes the stitched file, creates a Media entity, and stores forensics.
func (s *RecordingService) createMediaFromStitched(
	ctx context.Context,
	session *domain.RecordingSession,
	finalPath string,
) (mediaID string, duration float64, err error) {
	file, size, err := openStitchedFile(finalPath)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // best-effort

	if s.prober == nil || !s.prober.Available() {
		return "", 0, fmt.Errorf("ffprobe is required for stitched recording validation: %w", domain.ErrInternal)
	}

	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	probeResult, probeErr := s.prober.Probe(probeCtx, finalPath)
	cancel()
	if probeErr != nil {
		return "", 0, fmt.Errorf("validate stitched recording with ffprobe: %w", probeErr)
	}
	duration = probeResult.Duration
	if limitErr := s.mediaSvc.CheckStitchedDuration(duration); limitErr != nil {
		return "", 0, fmt.Errorf("stitched recording: %w", limitErr)
	}

	ct := "audio/webm"
	filename := fmt.Sprintf("recording-%s.webm", time.Now().Format("2006-01-02-150405"))

	media, err := s.mediaSvc.UploadStitchedRecording(
		ctx, session.OrganizationID, session.UserID, session.ID, filename, ct, size, file, session.RecordingMode)
	if err != nil {
		return "", 0, fmt.Errorf("create media from stitched recording: %w", err)
	}

	if err := s.anchorStitchedMedia(ctx, session, media.ID); err != nil {
		return "", 0, err
	}

	if duration > 0 {
		if durErr := s.mediaSvc.UpdateDuration(ctx, session.OrganizationID, media.ID, duration); durErr != nil {
			s.logger.Warn("failed to update media duration", "media_id", media.ID, "error", durErr)
		}
	}

	hash, analysis := s.computeForensics(finalPath, probeResult)
	if hash != "" || analysis != nil {
		if forErr := s.mediaSvc.StoreForensics(ctx, session.OrganizationID, media.ID, hash, analysis); forErr != nil {
			s.logger.Warn("failed to store forensics", "media_id", media.ID, "error", forErr)
		}
	}

	return media.ID, duration, nil
}

// finalizeSession atomically records completion metadata and status before
// cleanup. This makes retention unable to observe a completed timestamp while
// the session remains non-completed, and leaves failed cleanup retryable.
func (s *RecordingService) finalizeSession(
	ctx context.Context,
	session *domain.RecordingSession,
	sessionID string,
	duration float64,
) error {
	if err := s.ensureStitchedMediaMode(ctx, session); err != nil {
		return err
	}

	completedAt := s.now()
	if completer, ok := s.recordingRepo.(port.RecordingCompleter); ok {
		err := completer.Complete(ctx, sessionID, completedAt, duration)
		if err != nil && (!errors.Is(err, domain.ErrConflict) || !s.hasStatus(ctx, sessionID, domain.RecordingStatusCompleted)) {
			return fmt.Errorf("complete recording session: %w", err)
		}
	} else if err := s.completeSessionFallback(ctx, sessionID, completedAt, duration); err != nil {
		return err
	}

	return s.cleanupChunks(ctx, session)
}

func (s *RecordingService) completeSessionFallback(ctx context.Context, sessionID string, completedAt time.Time, duration float64) error {
	if err := s.recordingRepo.SetCompletedAt(ctx, sessionID, completedAt, duration); err != nil {
		return fmt.Errorf("set completed_at on session: %w", err)
	}
	err := s.recordingRepo.UpdateStatus(ctx, sessionID, domain.RecordingStatusCompleting, domain.RecordingStatusCompleted)
	if err == nil || (errors.Is(err, domain.ErrConflict) && s.hasStatus(ctx, sessionID, domain.RecordingStatusCompleted)) {
		return nil
	}
	return fmt.Errorf("mark session %s completed: %w", sessionID, err)
}

func (s *RecordingService) ensureStitchedMediaMode(ctx context.Context, session *domain.RecordingSession) error {
	if session.RecordingMode != domain.RecordingModePrivilege {
		return nil
	}
	if session.MediaID == "" || s.setRecordingMode == nil {
		return fmt.Errorf("privilege recording media mode is unavailable: %w", domain.ErrInternal)
	}
	if err := s.setRecordingMode(ctx, session.OrganizationID, session.MediaID, domain.RecordingModePrivilege); err != nil {
		return fmt.Errorf("set privilege recording mode: %w", err)
	}
	return nil
}

// hasStatus reports whether the session is currently in the given status.
// Used to tell "already done" apart from "unexpected state" after a failed CAS.
func (s *RecordingService) hasStatus(ctx context.Context, sessionID, status string) bool {
	session, err := s.recordingRepo.GetByID(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to re-read session after status conflict", "session_id", sessionID, "error", err)
		return false
	}
	return session.Status == status
}

// decryptChunksToFiles decrypts each chunk into an ordered temporary segment file.
func (s *RecordingService) decryptChunksToFiles(
	ctx context.Context,
	session *domain.RecordingSession,
	chunks []domain.RecordingChunk,
	tmpDir string,
) ([]string, error) {
	ext := ".webm"
	if session.MimeType == domain.RecordingMimeMP4 {
		ext = ".m4a"
	}

	paths := make([]string, 0, len(chunks))

	for _, chunk := range chunks {
		segmentPath := filepath.Join(tmpDir, fmt.Sprintf("chunk-%03d%s", chunk.Seq, ext))
		segmentFile, err := os.Create(segmentPath) //nolint:gosec // path built from our own tmpDir
		if err != nil {
			return nil, fmt.Errorf("create segment file seq=%d: %w", chunk.Seq, err)
		}

		decryptErr := s.decryptChunk(ctx, session.OrganizationID, &chunk, segmentFile)
		closeErr := segmentFile.Close()
		if decryptErr != nil {
			if closeErr != nil {
				return nil, fmt.Errorf("decrypt chunk seq=%d: %w", chunk.Seq, errors.Join(decryptErr, closeErr))
			}
			return nil, fmt.Errorf("decrypt chunk seq=%d: %w", chunk.Seq, decryptErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close segment file seq=%d: %w", chunk.Seq, closeErr)
		}

		paths = append(paths, segmentPath)
	}

	return paths, nil
}

// scanDecryptedChunks scans exactly the byte stream that will be passed to
// ffmpeg. It opens one segment at a time so a long recording cannot exhaust
// the process file-descriptor limit. Configured scanners fail closed; nil keeps
// local development usable where ClamAV is intentionally optional.
func (s *RecordingService) scanDecryptedChunks(ctx context.Context, inputPaths []string) error {
	if s.scanner == nil {
		return nil
	}
	scanCtx, cancel := context.WithTimeout(ctx, defaultRecordingScanTimeout)
	defer cancel()
	if !s.scanner.Available(scanCtx) {
		return fmt.Errorf("recording malware scanner unavailable: %w", domain.ErrInternal)
	}
	reader, writer := io.Pipe()
	copyErrCh := make(chan error, 1)
	go streamRecordingChunkFiles(inputPaths, writer, copyErrCh)

	result, scanErr := s.scanner.Scan(scanCtx, reader)
	_ = reader.Close() //nolint:errcheck // unblock the producer on early scanner failure
	copyErr := <-copyErrCh
	if scanErr != nil {
		return fmt.Errorf("scan decrypted recording chunks: %w", scanErr)
	}
	if result == nil || !result.Clean {
		if result != nil && result.ThreatName != "" {
			return fmt.Errorf("recording malware scan detected %q: %w", result.ThreatName, domain.ErrInvalidInput)
		}
		return fmt.Errorf("recording malware scan did not pass: %w", domain.ErrInvalidInput)
	}
	if copyErr != nil {
		return copyErr
	}
	return nil
}

func streamRecordingChunkFiles(paths []string, writer *io.PipeWriter, done chan<- error) {
	var streamErr error
	defer func() {
		if streamErr != nil {
			_ = writer.CloseWithError(streamErr) //nolint:errcheck // scanner receives the error
		} else {
			_ = writer.Close() //nolint:errcheck // scanner reached EOF
		}
		done <- streamErr
	}()

	for _, path := range paths {
		file, err := os.Open(path) //nolint:gosec // paths are service-created temp files
		if err != nil {
			streamErr = fmt.Errorf("open decrypted recording chunk: %w", err)
			return
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			streamErr = fmt.Errorf("stream decrypted recording chunk: %w", errors.Join(copyErr, closeErr))
			return
		}
	}
}

// decryptChunk downloads and decrypts a single chunk, verifying its checksum.
func (s *RecordingService) decryptChunk(ctx context.Context, orgID string, chunk *domain.RecordingChunk, dst io.Writer) error {
	encrypted, err := s.storage.Download(ctx, chunk.StorageKey)
	if err != nil {
		return fmt.Errorf("download chunk: %w", err)
	}
	defer func() { _ = encrypted.Close() }() //nolint:errcheck // best-effort

	sealedMeta := &crypto.SealedMeta{
		EncryptionMeta: crypto.EncryptionMeta{
			Algorithm:     chunk.EncryptionAlgo,
			ChunkSize:     chunk.ChunkSize,
			ChunkCount:    chunk.ChunkCount,
			PlaintextSize: chunk.PlaintextSize,
		},
		WrappedDEK:    chunk.WrappedDEK,
		WrappingNonce: chunk.WrappingNonce,
	}

	// Decrypt through a hasher to verify checksum.
	hasher := sha256.New()
	hashingWriter := io.MultiWriter(dst, hasher)

	pr, pw := io.Pipe()
	decryptErrCh := make(chan error, 1)

	go func() {
		dErr := s.envelope.OpenStream(ctx, orgID, encrypted, pw, sealedMeta)
		if dErr != nil {
			_ = pw.CloseWithError(dErr) //nolint:errcheck // pipe close cannot fail
			decryptErrCh <- dErr
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close cannot fail
		decryptErrCh <- nil
	}()

	if _, err := io.Copy(hashingWriter, pr); err != nil {
		_ = pr.Close() //nolint:errcheck // ensure goroutine completes
		<-decryptErrCh
		return fmt.Errorf("copy decrypted data: %w", err)
	}
	_ = pr.Close() //nolint:errcheck // ensure goroutine completes
	decryptErr := <-decryptErrCh
	if decryptErr != nil {
		return fmt.Errorf("decrypt: %w", decryptErr)
	}

	// Verify checksum.
	computed := hex.EncodeToString(hasher.Sum(nil))
	if computed != chunk.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s: %w", chunk.Checksum, computed, domain.ErrInvalidInput)
	}

	return nil
}

// computeForensics computes SHA-256 hash and builds audio analysis.
func (s *RecordingService) computeForensics(filePath string, probeResult *port.AudioProbeResult) (string, *domain.AudioAnalysis) {
	f, err := os.Open(filePath) //nolint:gosec // our own temp file
	if err != nil {
		return "", nil
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // best-effort

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", nil
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	analysis := s.mediaSvc.BuildAnalysis(probeResult, ".webm")
	return hash, analysis
}

// cleanupChunks removes chunk data before its manifest. The manifest remains
// when storage is unavailable so a later sweep can still decrypt and clean it.
func (s *RecordingService) cleanupChunks(ctx context.Context, session *domain.RecordingSession) error {
	prefix := domain.RecordingChunksPrefix(session.OrganizationID, session.ID, session.RecordingMode)
	keys, err := s.storage.ListByPrefix(ctx, prefix)
	if err != nil {
		return fmt.Errorf("list chunk objects: %w", err)
	}
	for _, key := range keys {
		if delErr := s.storage.Delete(ctx, key); delErr != nil && !errors.Is(delErr, domain.ErrNotFound) {
			return fmt.Errorf("delete chunk object %q: %w", key, delErr)
		}
	}
	if err := s.chunkRepo.DeleteBySession(ctx, session.ID); err != nil {
		return fmt.Errorf("delete chunk manifest: %w", err)
	}
	if s.quotaApplies {
		if err := s.quota.ReleaseRecordingChunks(ctx, session.ID); err != nil {
			return fmt.Errorf("release recording chunk quota: %w", err)
		}
	}
	return nil
}

func normalizeMediaType(contentType string) (string, error) {
	if contentType == "" {
		return "", fmt.Errorf("empty content type")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	return mediaType, nil
}

// bytesReaderAt wraps a byte slice to implement io.ReaderAt.
type bytesReaderAt struct {
	data []byte
}

func newBytesReaderAt(data []byte) *bytesReaderAt {
	return &bytesReaderAt{data: data}
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
