package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	defaultQAEvidenceSegments = 20
	maxQAEvidenceSegments     = 60
)

// TranscriptQAService prepares transcript evidence and validates grounded answers.
type TranscriptQAService struct {
	transSvc *TranscriptionService
	answerer port.TranscriptAnswerer
}

// NewTranscriptQAService creates a TranscriptQAService.
func NewTranscriptQAService(transSvc *TranscriptionService, answerer port.TranscriptAnswerer) *TranscriptQAService {
	return &TranscriptQAService{transSvc: transSvc, answerer: answerer}
}

// AskTranscript answers a question using only cited transcript evidence.
func (s *TranscriptQAService) AskTranscript(
	ctx context.Context,
	orgID string,
	req port.AskTranscriptRequest,
) (*port.AskTranscriptResult, error) {
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return nil, fmt.Errorf("question cannot be empty: %w", domain.ErrInvalidInput)
	}
	if len([]rune(question)) > 1000 {
		return nil, fmt.Errorf("question cannot exceed 1000 characters: %w", domain.ErrInvalidInput)
	}
	if s.transSvc == nil {
		return nil, fmt.Errorf("transcription service not available")
	}
	if s.answerer == nil {
		return nil, fmt.Errorf("transcript answerer not available")
	}

	evidence, err := s.evidenceForTranscript(ctx, orgID, req.TranscriptionID, req.MaxSegments)
	if err != nil {
		return nil, err
	}
	if len(evidence) == 0 {
		return &port.AskTranscriptResult{
			Answer:  "I could not find cited transcript evidence to answer this question.",
			Refusal: true,
		}, nil
	}

	answer, err := s.answerer.AnswerTranscriptQuestion(ctx, question, evidence)
	if err != nil {
		return nil, fmt.Errorf("answer transcript question: %w", err)
	}
	return buildAskTranscriptResult(answer, evidence)
}

func (s *TranscriptQAService) evidenceForTranscript(
	ctx context.Context,
	orgID, transcriptionID string,
	maxSegments int,
) ([]port.TranscriptEvidence, error) {
	trans, err := s.transSvc.GetByID(ctx, orgID, transcriptionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get transcription: %w", err)
	}
	segments, err := ParseTranscriptSegments(trans.Utterances, trans.FullTranscript, trans.SpeakerMap)
	if err != nil {
		return nil, err
	}

	limit := clampQAEvidenceLimit(maxSegments)
	if len(segments) > limit {
		segments = segments[:limit]
	}
	evidence := make([]port.TranscriptEvidence, len(segments))
	for i, segment := range segments {
		evidence[i] = port.TranscriptEvidence{
			ID:              fmt.Sprintf("e%d", i+1),
			TranscriptionID: trans.ID,
			MediaID:         trans.MediaID,
			SourceType:      "media",
			DisplayName:     transcriptQADisplayName(trans),
			Speaker:         segment.Speaker,
			StartSeconds:    segment.StartSeconds,
			EndSeconds:      segment.EndSeconds,
			Text:            segment.Text,
		}
	}
	return evidence, nil
}

func buildAskTranscriptResult(
	answer *port.TranscriptAnswer,
	evidence []port.TranscriptEvidence,
) (*port.AskTranscriptResult, error) {
	if answer == nil {
		return nil, fmt.Errorf("answerer returned no answer")
	}
	evidenceByID := make(map[string]port.TranscriptEvidence, len(evidence))
	for _, item := range evidence {
		evidenceByID[item.ID] = item
	}
	citations := make([]port.TranscriptEvidence, 0, len(answer.CitationIDs))
	for _, id := range answer.CitationIDs {
		item, ok := evidenceByID[id]
		if !ok {
			return nil, fmt.Errorf("answer citation references unknown evidence id %q: %w", id, domain.ErrInvalidInput)
		}
		citations = append(citations, item)
	}
	if strings.TrimSpace(answer.Answer) == "" || len(citations) == 0 {
		return &port.AskTranscriptResult{
			Answer:        "I could not answer from the provided transcript evidence.",
			EvidenceCount: len(evidence),
			Refusal:       true,
		}, nil
	}
	return &port.AskTranscriptResult{
		Answer:        answer.Answer,
		Citations:     citations,
		EvidenceCount: len(evidence),
	}, nil
}

func clampQAEvidenceLimit(limit int) int {
	if limit <= 0 {
		return defaultQAEvidenceSegments
	}
	if limit > maxQAEvidenceSegments {
		return maxQAEvidenceSegments
	}
	return limit
}

func transcriptQADisplayName(trans *domain.Transcription) string {
	if strings.TrimSpace(trans.MediaTitle) != "" {
		return trans.MediaTitle
	}
	if strings.TrimSpace(trans.MediaFilename) != "" {
		return trans.MediaFilename
	}
	return trans.ID
}
