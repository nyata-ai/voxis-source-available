package port

import (
	"context"

	"github.com/voxis/backend/internal/domain"
)

// TranscriptionSegmentRepository stores bounded local chunk state.
type TranscriptionSegmentRepository interface {
	CreateBatch(ctx context.Context, segments []*domain.TranscriptionSegment) error
	ListByTranscriptionID(ctx context.Context, transcriptionID string) ([]*domain.TranscriptionSegment, error)
	UpdateSegment(ctx context.Context, segment *domain.TranscriptionSegment) error
}
