package service

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// RecordingRetentionPolicyService manages the platform-wide live recording audio retention policy.
type RecordingRetentionPolicyService struct {
	repo port.RecordingRetentionPolicyRepository
	now  func() time.Time
}

// NewRecordingRetentionPolicyService creates a RecordingRetentionPolicyService.
func NewRecordingRetentionPolicyService(repo port.RecordingRetentionPolicyRepository, now func() time.Time) *RecordingRetentionPolicyService {
	if repo == nil {
		panic("service: recording retention repository must not be nil")
	}
	if now == nil {
		now = time.Now
	}
	return &RecordingRetentionPolicyService{repo: repo, now: now}
}

// Get returns the current platform-wide retention policy.
func (s *RecordingRetentionPolicyService) Get(ctx context.Context) (domain.LiveRecordingRetentionPolicy, error) {
	return s.repo.GetLiveRecordingRetentionPolicy(ctx)
}

// Prepare normalizes and validates a proposed policy before preview or persistence.
func (s *RecordingRetentionPolicyService) Prepare(ctx context.Context, proposed domain.LiveRecordingRetentionPolicy) (domain.LiveRecordingRetentionPolicy, error) {
	current, err := s.repo.GetLiveRecordingRetentionPolicy(ctx)
	if err != nil {
		return domain.LiveRecordingRetentionPolicy{}, err
	}
	if proposed.Days == 0 {
		proposed.Days = 7
	}
	if err := proposed.Validate(); err != nil {
		return domain.LiveRecordingRetentionPolicy{}, err
	}
	now := s.now().UTC()
	switch {
	case !proposed.Enabled:
		proposed.ApplyToExisting = false
		proposed.EffectiveAt = nil
	case proposed.ApplyToExisting:
		proposed.EffectiveAt = nil
	case current.Enabled && current.EffectiveAt != nil:
		proposed.EffectiveAt = current.EffectiveAt
	default:
		proposed.EffectiveAt = &now
	}
	proposed.UpdatedAt = &now
	return proposed, nil
}

// Preview returns the immediate-delete impact of a proposed platform-wide policy.
func (s *RecordingRetentionPolicyService) Preview(ctx context.Context, policy domain.LiveRecordingRetentionPolicy) (domain.LiveRecordingRetentionPreview, error) {
	if err := policy.Validate(); err != nil {
		return domain.LiveRecordingRetentionPreview{}, err
	}
	preview, err := s.repo.PreviewLiveRecordingRetentionPolicy(ctx, policy, s.now().UTC())
	if err != nil {
		return domain.LiveRecordingRetentionPreview{}, err
	}
	preview.ProspectiveEffectiveAt = policy.EffectiveAt
	return preview, nil
}

// Update persists a validated platform-wide policy.
func (s *RecordingRetentionPolicyService) Update(ctx context.Context, policy domain.LiveRecordingRetentionPolicy, updatedBy string) (domain.LiveRecordingRetentionPolicy, error) {
	if err := policy.Validate(); err != nil {
		return domain.LiveRecordingRetentionPolicy{}, err
	}
	return s.repo.UpdateLiveRecordingRetentionPolicy(ctx, policy, updatedBy)
}
