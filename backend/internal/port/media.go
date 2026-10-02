package port

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// MediaRepository handles media persistence.
type MediaRepository interface {
	// Create persists a new media record.
	// The media ID will be populated after creation.
	Create(ctx context.Context, media *domain.Media) error

	// GetByID retrieves a media record by ID.
	// Returns domain.ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*domain.Media, error)

	// ListByOrganization retrieves media records for an organization
	// ordered by creation time descending, with pagination.
	ListByOrganization(ctx context.Context, orgID string, limit, offset int) ([]*domain.Media, error)

	// Update updates an existing media record.
	Update(ctx context.Context, media *domain.Media) error

	// Delete removes a media record by ID.
	// Returns domain.ErrNotFound if the media does not exist.
	Delete(ctx context.Context, id string) error

	// CascadeDelete atomically soft-deletes a media record and all its child
	// transcriptions and summaries in a single database statement.
	CascadeDelete(ctx context.Context, id string) error

	// CountByOrganization returns the total number of media records
	// for the given organization.
	CountByOrganization(ctx context.Context, orgID string) (int64, error)

	// SearchByOrganization searches media with optional filename and status filters.
	// An empty search or status string means "no filter".
	SearchByOrganization(ctx context.Context, orgID, search, status string, limit, offset int) ([]*domain.Media, error)

	// CountSearchByOrganization counts media matching search/status filters.
	CountSearchByOrganization(ctx context.Context, orgID, search, status string) (int64, error)

	// UpdateDuration sets duration on a media record.
	UpdateDuration(ctx context.Context, mediaID string, duration float64) error

	// UpdateMetadata updates title and/or description on a media record.
	// Use the pointer flags to indicate which fields should be set (nil = don't change).
	UpdateMetadata(ctx context.Context, id string, title *string, description *string) (*domain.Media, error)

	// UpdateForensics stores the file hash and encrypted audio metadata.
	UpdateForensics(ctx context.Context, id string, fileHash string, audioMetadata []byte) error

	// UpdateScanStatus updates the malware scan status for a media record.
	UpdateScanStatus(ctx context.Context, id, status string) error

	// MarkAudioDeleted clears active audio encryption metadata after retention deletion.
	MarkAudioDeleted(ctx context.Context, mediaID, storageKey string, deletedAt time.Time) (*domain.Media, error)

	// ListByOrganizationFiltered retrieves media records with optional filters
	// (date range, duration range, status, search, sort). Used by MCP list_media.
	ListByOrganizationFiltered(ctx context.Context, orgID string, filter MediaListFilter) ([]*domain.Media, error)

	// SetRecordingMode updates the recording_mode column on a media record.
	// Used by RecordingService.Stitch to propagate session mode to media.
	SetRecordingMode(ctx context.Context, mediaID string, mode domain.RecordingMode) error
}
