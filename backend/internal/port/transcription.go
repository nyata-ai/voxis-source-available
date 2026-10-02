package port

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// TranscriptionJobInserter queues bounded media transcription work.
type TranscriptionJobInserter interface {
	InsertTranscribeJob(ctx context.Context, transcriptionID, mediaID, orgID string) error
}

// TranscriptSearchCursor identifies the final transcription examined by a
// keyset-paginated transcript-content search.
type TranscriptSearchCursor struct {
	CreatedAt time.Time
	ID        string
}

// TranscriptionRepository stores media-derived transcriptions only.
type TranscriptionRepository interface {
	Create(ctx context.Context, transcription *domain.Transcription) error
	GetByID(ctx context.Context, id string) (*domain.Transcription, error)
	GetByMediaID(ctx context.Context, mediaID string) (*domain.Transcription, error)
	ListByOrganization(ctx context.Context, orgID string, limit, offset int, search string) ([]*domain.Transcription, error)
	CountByOrganization(ctx context.Context, orgID, search string) (int64, error)
	Update(ctx context.Context, transcription *domain.Transcription) error
	UpdateContentCAS(ctx context.Context, transcription *domain.Transcription, expectedNonce []byte) (bool, error)
	Delete(ctx context.Context, id string) error
	CascadeDelete(ctx context.Context, id string) error
	DeleteByMediaID(ctx context.Context, mediaID string) (int64, error)
	ListRecentActivity(ctx context.Context, orgID string, limit int) ([]*domain.Transcription, error)
	ListActiveActivity(ctx context.Context, orgID string, limit int) ([]*domain.Transcription, error)
	ListByOrganizationFiltered(ctx context.Context, orgID string, filter TranscriptionListFilter) ([]*domain.Transcription, error)
	ListCompletedForTranscriptSearch(ctx context.Context, orgID string, cursor *TranscriptSearchCursor, limit int) ([]*domain.Transcription, error)
}
