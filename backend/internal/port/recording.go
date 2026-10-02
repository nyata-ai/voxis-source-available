package port

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// RecordingRepository handles recording session persistence.
type RecordingRepository interface {
	// Create persists a new recording session.
	// The session ID will be populated after creation.
	// Returns domain.ErrConflict if the user already has an active session
	// (enforced by unique partial index).
	Create(ctx context.Context, session *domain.RecordingSession) error

	// GetByID retrieves a recording session by ID.
	// Returns domain.ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*domain.RecordingSession, error)

	// GetActiveByUserID retrieves the user's active session (recording or paused).
	// Returns domain.ErrNotFound if no active session exists.
	GetActiveByUserID(ctx context.Context, userID string) (*domain.RecordingSession, error)

	// GetInterruptedByUserID retrieves all interrupted sessions for a user.
	GetInterruptedByUserID(ctx context.Context, userID string) ([]domain.RecordingSession, error)

	// CountInterruptedByUserID returns the number of interrupted sessions for a user.
	CountInterruptedByUserID(ctx context.Context, userID string) (int, error)

	// UpdateStatus atomically transitions a session from one status to another.
	// Uses WHERE id = $1 AND status = $expected for atomicity.
	// Returns domain.ErrConflict if the session is not in the expected state.
	UpdateStatus(ctx context.Context, id string, fromStatus, toStatus string) error

	// SetMediaID stores the resulting media ID on a completed session.
	SetMediaID(ctx context.Context, id string, mediaID string) error

	// UpdateLastChunkAt updates the last chunk timestamp.
	UpdateLastChunkAt(ctx context.Context, id string, t time.Time) error

	// UpdateLastActivityAt updates the last activity timestamp.
	// Implementations should perform a monotonic write.
	UpdateLastActivityAt(ctx context.Context, id string, t time.Time) error

	// FindStaleRecording finds sessions in an active status ('recording' or
	// 'paused') where the last activity timestamp is older than the threshold.
	// Both states are covered because the one-active-session unique index is.
	FindStaleRecording(ctx context.Context, threshold time.Duration) ([]domain.RecordingSession, error)

	// ListByStatusOlderThan returns sessions in the given status whose updated_at
	// is older than the threshold timestamp.
	ListByStatusOlderThan(ctx context.Context, status string, threshold time.Time) ([]domain.RecordingSession, error)

	// SetCompletedAt sets the completed_at and total_duration on a session.
	SetCompletedAt(ctx context.Context, id string, completedAt time.Time, totalDuration float64) error

	// ListLiveRecordingAudioRetentionCandidates returns completed live recordings due for audio deletion.
	ListLiveRecordingAudioRetentionCandidates(ctx context.Context, now time.Time, limit int) ([]LiveRecordingAudioRetentionCandidate, error)

	// ListActiveActivity returns recording sessions that are in progress
	// ('completing'), recently completed, or failed within the 48h failed-chunk
	// retention window, LEFT JOINed to their media for status/scan_status/title.
	// User-scoped. Bounded to 20 rows.
	ListActiveActivity(ctx context.Context, userID string) ([]domain.RecordingActivityRow, error)
}

// RecordingSessionLocker serializes operations for one recording session when
// the backing repository supports cross-process locking. Implementations may
// invoke fn directly when no distributed lock is available (e.g. test doubles).
type RecordingSessionLocker interface {
	WithSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error
}

// RecordingSessionTryLocker rejects busy recording mutations instead of
// queueing them: ErrConflict when the session is locked elsewhere, ErrRateLimited
// when no lock capacity is free (retryable).
type RecordingSessionTryLocker interface {
	TryWithSessionLock(ctx context.Context, sessionID string, fn func(context.Context) error) error
}

// RecordingCompleter atomically finalizes a completing session. It is separate
// from RecordingRepository so existing lightweight test repositories can use a
// safe in-process fallback while PostgreSQL uses one guarded update.
type RecordingCompleter interface {
	Complete(ctx context.Context, id string, completedAt time.Time, totalDuration float64) error
}

// RecordingChunkCleanupLister locates completed sessions whose chunk manifests
// remain after a transient storage-cleanup failure.
type RecordingChunkCleanupLister interface {
	ListWithChunksByStatusOlderThan(ctx context.Context, status string, threshold time.Time, limit int) ([]domain.RecordingSession, error)
}

// RecordingChunkSizer returns the aggregate encoded source bytes held by a
// recording session. It is optional to keep lightweight non-recording test
// doubles compatible while production PostgreSQL enforces the session quota.
type RecordingChunkSizer interface {
	TotalPlaintextSizeBySession(ctx context.Context, sessionID string) (int64, error)
}

// LiveRecordingAudioRetentionCandidate identifies one live recording audio object due for deletion.
type LiveRecordingAudioRetentionCandidate struct {
	SessionID      string
	MediaID        string
	OrganizationID string
	StorageKey     string
	CompletedAt    time.Time
}

// ChunkRepository handles recording chunk manifest persistence.
type ChunkRepository interface {
	// Create persists a new chunk manifest row.
	// Returns domain.ErrConflict if a chunk with the same (session_id, seq) exists.
	Create(ctx context.Context, chunk *domain.RecordingChunk) error

	// Upsert creates or replaces a chunk manifest row (idempotent retry).
	Upsert(ctx context.Context, chunk *domain.RecordingChunk) error

	// GetBySessionAndSeq retrieves a chunk by session ID and sequence number.
	// Returns domain.ErrNotFound if not found.
	GetBySessionAndSeq(ctx context.Context, sessionID string, seq int) (*domain.RecordingChunk, error)

	// ListBySession retrieves all chunks for a session ordered by seq ASC.
	ListBySession(ctx context.Context, sessionID string) ([]domain.RecordingChunk, error)

	// CountBySession returns the number of chunks for a session.
	CountBySession(ctx context.Context, sessionID string) (int, error)

	// MaxSeqBySession returns the maximum sequence number for a session.
	// Returns -1 if no chunks exist.
	MaxSeqBySession(ctx context.Context, sessionID string) (int, error)

	// DeleteBySession removes all chunk manifest rows for a session.
	DeleteBySession(ctx context.Context, sessionID string) error
}

// AudioStitcher converts audio between formats (ffmpeg wrapper).
type AudioStitcher interface {
	// ConcatToWebMOpus concatenates ordered audio segment files into WebM/Opus.
	ConcatToWebMOpus(ctx context.Context, inputPaths []string, outputPath string) error

	// ConvertToWebMOpus converts an audio file to WebM/Opus format.
	ConvertToWebMOpus(ctx context.Context, inputPath, outputPath string) error

	// Available reports whether the ffmpeg binary is accessible.
	Available() bool
}
