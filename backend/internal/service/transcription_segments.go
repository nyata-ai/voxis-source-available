package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const maxSegmentsPerTranscription = 200

type segmentContent struct {
	FullTranscript string          `json:"full_transcript"`
	Utterances     json.RawMessage `json:"utterances"`
}

// SegmentTranscriptionService coordinates chunk-level transcription state and
// final parent transcript assembly.
type SegmentTranscriptionService struct {
	transRepo port.TranscriptionRepository
	segRepo   port.TranscriptionSegmentRepository
	transSvc  *TranscriptionService
	logger    *slog.Logger
}

// NewSegmentTranscriptionService creates a new SegmentTranscriptionService.
func NewSegmentTranscriptionService(
	transRepo port.TranscriptionRepository,
	segRepo port.TranscriptionSegmentRepository,
	transSvc *TranscriptionService,
	logger *slog.Logger,
) *SegmentTranscriptionService {
	if transRepo == nil {
		panic("segment transcription service: transcription repo cannot be nil")
	}
	if segRepo == nil {
		panic("segment transcription service: segment repo cannot be nil")
	}
	if transSvc == nil {
		panic("segment transcription service: transcription service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SegmentTranscriptionService{
		transRepo: transRepo,
		segRepo:   segRepo,
		transSvc:  transSvc,
		logger:    logger,
	}
}

// FinalizeIfReady assembles all completed segments and finalizes parent transcription.
// Returns true if parent completion was attempted.
func (s *SegmentTranscriptionService) FinalizeIfReady(ctx context.Context, transcriptionID string) (bool, error) {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return false, err
	}

	switch trans.Status {
	case domain.TranscriptionStatusCompleted, domain.TranscriptionStatusFailed, domain.TranscriptionStatusDeleted:
		return false, nil
	}

	segments, err := s.segRepo.ListByTranscriptionID(ctx, transcriptionID)
	if err != nil {
		return false, err
	}
	if len(segments) == 0 {
		return false, nil
	}
	if len(segments) > maxSegmentsPerTranscription {
		return false, fmt.Errorf("segment count exceeds max %d: %w", maxSegmentsPerTranscription, domain.ErrInvalidInput)
	}

	sort.Slice(segments, func(i, j int) bool { return segments[i].SegmentIndex < segments[j].SegmentIndex })

	ready, err := s.ensureSegmentsReady(ctx, transcriptionID, trans.OrganizationID, segments)
	if err != nil || !ready {
		return false, err
	}

	stitched, err := stitchSegmentResults(segments)
	if err != nil {
		return false, err
	}

	if err := s.transSvc.ProcessResult(ctx, transcriptionID, stitched); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, err
		}
		return false, fmt.Errorf("finalize stitched transcription: %w", err)
	}

	s.clearSegmentPlaintext(ctx, transcriptionID, segments)

	s.logger.Info("finalized segmented transcription", "transcription_id", transcriptionID, "segments", len(segments))
	return true, nil
}

// ensureSegmentsReady fails the parent on the first failed segment, reports
// readiness, and hydrates completed segment content for stitching. ready=false
// with a nil error means the parent was failed or a segment is still in flight.
func (s *SegmentTranscriptionService) ensureSegmentsReady(
	ctx context.Context,
	transcriptionID, orgID string,
	segments []*domain.TranscriptionSegment,
) (bool, error) {
	for _, seg := range segments {
		if seg.Status == domain.TranscriptionSegmentStatusFailed {
			errMsg := seg.ErrorMessage
			if errMsg == "" {
				errMsg = fmt.Sprintf("segment %d failed", seg.SegmentIndex)
			}
			if handleErr := s.transSvc.HandleFailure(ctx, transcriptionID, errMsg); handleErr != nil {
				return false, handleErr
			}
			return false, nil
		}
		if seg.Status != domain.TranscriptionSegmentStatusCompleted {
			return false, nil
		}

		if hydrateErr := s.hydrateSegmentContent(ctx, orgID, seg); hydrateErr != nil {
			return false, hydrateErr
		}
	}
	return true, nil
}

// clearSegmentPlaintext is best-effort minimization: the parent stores the
// encrypted canonical content, so segment plaintext can be cleared after
// successful finalization.
func (s *SegmentTranscriptionService) clearSegmentPlaintext(
	ctx context.Context,
	transcriptionID string,
	segments []*domain.TranscriptionSegment,
) {
	for _, seg := range segments {
		seg.ContentEncrypted = nil
		seg.ContentNonce = nil
		seg.WrappedDEK = nil
		seg.WrappingNonce = nil
		seg.FullTranscript = ""
		seg.Utterances = nil
		if updateErr := s.segRepo.UpdateSegment(ctx, seg); updateErr != nil {
			s.logger.Warn("failed to clear segment plaintext after finalization",
				"segment_id", seg.ID,
				"transcription_id", transcriptionID,
				"error", updateErr,
			)
		}
	}
}

func (s *SegmentTranscriptionService) sealSegmentContent(
	ctx context.Context,
	orgID string,
	result *port.TranscriptionResult,
) (*crypto.SealedFieldMeta, error) {
	if s.transSvc.envelope == nil {
		return nil, fmt.Errorf("segment encryption unavailable: %w", domain.ErrInvalidInput)
	}

	content := segmentContent{
		FullTranscript: result.FullTranscript,
		Utterances:     result.Utterances,
	}
	payload, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal segment content: %w", err)
	}

	sealed, err := s.transSvc.envelope.SealField(ctx, orgID, payload)
	if err != nil {
		return nil, fmt.Errorf("encrypt segment content: %w", err)
	}
	return sealed, nil
}

func (s *SegmentTranscriptionService) hydrateSegmentContent(ctx context.Context, orgID string, seg *domain.TranscriptionSegment) error {
	if len(seg.ContentEncrypted) == 0 {
		if seg.FullTranscript == "" && len(seg.Utterances) == 0 {
			return fmt.Errorf("segment %d missing transcription payload: %w", seg.SegmentIndex, domain.ErrInvalidInput)
		}
		return nil
	}

	if s.transSvc.envelope == nil {
		return fmt.Errorf("segment decryption unavailable: %w", domain.ErrInvalidInput)
	}

	sealed := &crypto.SealedFieldMeta{
		Algorithm:     "AES-256-GCM",
		Ciphertext:    seg.ContentEncrypted,
		Nonce:         seg.ContentNonce,
		WrappedDEK:    seg.WrappedDEK,
		WrappingNonce: seg.WrappingNonce,
	}
	plaintext, err := s.transSvc.envelope.OpenField(ctx, orgID, sealed)
	if err != nil {
		return fmt.Errorf("decrypt segment content (segment=%d): %w", seg.SegmentIndex, err)
	}

	var content segmentContent
	if err := json.Unmarshal(plaintext, &content); err != nil {
		return fmt.Errorf("unmarshal segment content (segment=%d): %w", seg.SegmentIndex, err)
	}
	seg.FullTranscript = content.FullTranscript
	seg.Utterances = content.Utterances
	return nil
}

func stitchSegmentResults(segments []*domain.TranscriptionSegment) (*port.TranscriptionResult, error) {
	fullParts := make([]string, 0, len(segments))
	allUtterances := make([]map[string]interface{}, 0)
	totalDuration := 0.0

	for _, seg := range segments {
		part := strings.TrimSpace(seg.FullTranscript)
		if part != "" {
			fullParts = append(fullParts, part)
		}

		totalDuration += segmentDurationSeconds(seg)

		if len(seg.Utterances) == 0 {
			continue
		}

		utterances, err := shiftSegmentUtterances(seg)
		if err != nil {
			return nil, err
		}
		allUtterances = append(allUtterances, utterances...)
	}

	utterancesJSON, err := json.Marshal(allUtterances)
	if err != nil {
		return nil, fmt.Errorf("marshal stitched utterances: %w", err)
	}

	speakerCount, wordCount := domain.CountSpeakersAndWords(utterancesJSON)
	return &port.TranscriptionResult{
		Status:         "done",
		FullTranscript: strings.Join(fullParts, " "),
		Utterances:     utterancesJSON,
		SpeakerCount:   speakerCount,
		WordCount:      wordCount,
		AudioDuration:  totalDuration,
	}, nil
}

// segmentDurationSeconds returns the segment's measured duration, falling back
// to the planned offset window when the provider did not report one.
func segmentDurationSeconds(seg *domain.TranscriptionSegment) float64 {
	if seg.DurationSeconds > 0 {
		return seg.DurationSeconds
	}
	if segmentDur := seg.EndOffsetSec - seg.StartOffsetSec; segmentDur > 0 {
		return segmentDur
	}
	return 0
}

// shiftSegmentUtterances parses the segment's utterances and shifts all
// utterance and word timestamps by the segment's start offset.
func shiftSegmentUtterances(seg *domain.TranscriptionSegment) ([]map[string]interface{}, error) {
	var utterances []map[string]interface{}
	if err := json.Unmarshal(seg.Utterances, &utterances); err != nil {
		return nil, fmt.Errorf("unmarshal segment utterances (index=%d): %w", seg.SegmentIndex, err)
	}

	for i := 0; i < len(utterances); i++ {
		shiftNumberField(utterances[i], "start", seg.StartOffsetSec)
		shiftNumberField(utterances[i], "end", seg.StartOffsetSec)
		shiftUtteranceWords(utterances[i], seg.StartOffsetSec)
	}
	return utterances, nil
}

// shiftUtteranceWords shifts the word-level timestamps of one utterance.
func shiftUtteranceWords(utterance map[string]interface{}, offset float64) {
	rawWords, hasWords := utterance["words"]
	if !hasWords {
		return
	}

	words, ok := rawWords.([]interface{})
	if !ok {
		return
	}
	for j := 0; j < len(words); j++ {
		wordMap, isMap := words[j].(map[string]interface{})
		if !isMap {
			continue
		}
		shiftNumberField(wordMap, "start", offset)
		shiftNumberField(wordMap, "end", offset)
	}
}

func shiftNumberField(m map[string]interface{}, key string, offset float64) {
	v, ok := m[key]
	if !ok {
		return
	}

	switch n := v.(type) {
	case float64:
		m[key] = n + offset
	case int:
		m[key] = float64(n) + offset
	case int32:
		m[key] = float64(n) + offset
	case int64:
		m[key] = float64(n) + offset
	}
}
