package service

import (
	"context"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Provider-neutral segment entry points.
//
// ProcessSegmentSuccess/ProcessSegmentFailure look the segment up by provider job ID,
// which a Speechmatics segment does not have. Rather than edit those (and risk
// the other provider chunked path), the two functions below take a segment the caller
// already loaded through its own provider-specific query and run the identical
// post-lookup logic — sealing, status transition, and FinalizeIfReady.

// ProcessSegmentResult records a completed segment and attempts final assembly.
// The caller performs the provider-specific lookup; everything after that is
// shared with the other provider path.
func (s *SegmentTranscriptionService) ProcessSegmentResult(
	ctx context.Context,
	seg *domain.TranscriptionSegment,
	result *port.TranscriptionResult,
) (*domain.TranscriptionSegment, error) {
	if seg == nil {
		return nil, domain.ErrInvalidInput
	}

	// Idempotent: an already-completed segment only re-checks assembly.
	if seg.Status == domain.TranscriptionSegmentStatusCompleted {
		if _, finalizeErr := s.FinalizeIfReady(ctx, seg.TranscriptionID); finalizeErr != nil {
			return nil, finalizeErr
		}
		return seg, nil
	}

	// Ignore a late success after terminal failure.
	if seg.Status == domain.TranscriptionSegmentStatusFailed {
		return seg, nil
	}

	trans, err := s.transRepo.GetByID(ctx, seg.TranscriptionID)
	if err != nil {
		return nil, err
	}

	sealed, err := s.sealSegmentContent(ctx, trans.OrganizationID, result)
	if err != nil {
		return nil, err
	}

	seg.SetCompleted("", nil, result.SpeakerCount, result.WordCount, result.AudioDuration)
	seg.ContentEncrypted = sealed.Ciphertext
	seg.ContentNonce = sealed.Nonce
	seg.WrappedDEK = sealed.WrappedDEK
	seg.WrappingNonce = sealed.WrappingNonce
	if err := s.segRepo.UpdateSegment(ctx, seg); err != nil {
		return nil, err
	}

	if _, err := s.FinalizeIfReady(ctx, seg.TranscriptionID); err != nil {
		return nil, err
	}

	return seg, nil
}

// ProcessSegmentFailureFor records a failed segment and fails the parent.
// The caller performs the provider-specific lookup.
func (s *SegmentTranscriptionService) ProcessSegmentFailureFor(
	ctx context.Context,
	seg *domain.TranscriptionSegment,
	errMsg string,
) (*domain.TranscriptionSegment, error) {
	if seg == nil {
		return nil, domain.ErrInvalidInput
	}

	if seg.Status != domain.TranscriptionSegmentStatusFailed {
		seg.SetFailed(errMsg)
		if updateErr := s.segRepo.UpdateSegment(ctx, seg); updateErr != nil {
			return nil, updateErr
		}
	}

	// Parent failure is idempotent in TranscriptionService.
	if err := s.transSvc.HandleFailure(ctx, seg.TranscriptionID, errMsg); err != nil {
		return nil, err
	}

	return seg, nil
}
