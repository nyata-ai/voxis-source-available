package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// StorageQuotaRepository owns durable, per-user logical-storage allocations.
// PostgreSQL implementations serialize reservations by locking the users row.
type StorageQuotaRepository interface {
	GetStorageQuotaPolicy(ctx context.Context) (domain.StorageQuotaPolicy, error)
	UpdateStorageQuotaPolicy(ctx context.Context, policy domain.StorageQuotaPolicy, updatedBy string) (domain.StorageQuotaPolicy, error)

	// CreateMediaReservation atomically creates the pending media row and its
	// durable reservation. replacingSessionID is set only for stitched media.
	CreateMediaReservation(ctx context.Context, media *domain.Media, replacingSessionID string) error
	FinalizeMediaReservation(ctx context.Context, media *domain.Media) error
	ReleaseMediaReservation(ctx context.Context, mediaID string) error
	DeleteMediaAndRelease(ctx context.Context, mediaID, storageKey string) error
	FindCommittedStitchedMedia(ctx context.Context, sessionID string) (string, bool, error)
	MarkMediaAudioDeletedAndRelease(ctx context.Context, mediaID, storageKey string) error

	ReserveRecordingChunk(ctx context.Context, session *domain.RecordingSession, seq int, bytes int64) error
	FinalizeRecordingChunk(ctx context.Context, chunk *domain.RecordingChunk) error
	ReleaseRecordingChunk(ctx context.Context, sessionID string, seq int) error
	ReleaseRecordingChunks(ctx context.Context, sessionID string) error

	GetUserStorageUsedBytes(ctx context.Context, orgID, userID string) (int64, error)
}
