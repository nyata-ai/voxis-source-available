package port

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// RecordingRetentionPolicyRepository stores the platform-wide live recording retention policy.
type RecordingRetentionPolicyRepository interface {
	GetLiveRecordingRetentionPolicy(ctx context.Context) (domain.LiveRecordingRetentionPolicy, error)
	PreviewLiveRecordingRetentionPolicy(ctx context.Context, policy domain.LiveRecordingRetentionPolicy, now time.Time) (domain.LiveRecordingRetentionPreview, error)
	UpdateLiveRecordingRetentionPolicy(ctx context.Context, policy domain.LiveRecordingRetentionPolicy, updatedBy string) (domain.LiveRecordingRetentionPolicy, error)
}
