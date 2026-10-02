package service

import (
	"context"
	"fmt"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

var roleBriefPrompts = map[string]string{
	"journalist":        "Build a journalist story brief. Focus on newsworthy facts, source statements, unresolved claims, and follow-up questions.",
	"law_enforcement":   "Build a law enforcement case timeline. Focus on stated events, people, times, evidence references, and gaps. Do not draw legal conclusions.",
	"investment_banker": "Build an investment banking diligence memo. Focus on financial metrics, claims by management, risks, forecasts, and open diligence questions.",
	"student":           "Build a student study guide. Focus on concepts, definitions, examples, questions for review, and cited source segments.",
	"meeting_minutes":   "Create concise meeting minutes. Focus on attendees or speakers, agenda topics, decisions, action items, owners, dates, unresolved questions, and cited source segments.",
}

// BuildRoleBrief builds a role-specific brief from cited transcript evidence.
func (s *MCPService) BuildRoleBrief(
	ctx context.Context,
	orgID string,
	req port.RoleBriefRequest,
) (*port.AskTranscriptResult, error) {
	prompt, ok := roleBriefPrompts[req.Role]
	if !ok {
		return nil, fmt.Errorf("unsupported role %q: %w", req.Role, domain.ErrInvalidInput)
	}
	if req.TranscriptionID == "" && req.CollectionID == "" {
		return nil, fmt.Errorf("source selector required: %w", domain.ErrInvalidInput)
	}
	if req.TranscriptionID != "" && req.CollectionID != "" {
		return nil, fmt.Errorf("only one source selector is supported: %w", domain.ErrInvalidInput)
	}
	if s.answerer == nil {
		return nil, fmt.Errorf("role brief answerer not available")
	}

	evidence, err := s.roleBriefEvidence(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	if len(evidence) == 0 {
		return &port.AskTranscriptResult{Answer: "I could not find cited transcript evidence to build this brief.", Refusal: true}, nil
	}
	answer, err := s.answerer.AnswerTranscriptQuestion(ctx, prompt, evidence)
	if err != nil {
		return nil, fmt.Errorf("build role brief: %w", err)
	}
	return buildAskTranscriptResult(answer, evidence)
}

func (s *MCPService) roleBriefEvidence(
	ctx context.Context,
	orgID string,
	req port.RoleBriefRequest,
) ([]port.TranscriptEvidence, error) {
	if req.TranscriptionID != "" {
		qa := NewTranscriptQAService(s.transSvc, s.answerer)
		return qa.evidenceForTranscript(ctx, orgID, req.TranscriptionID, req.MaxSegments)
	}
	items, err := s.ListCollectionItems(ctx, orgID, req.CollectionID, maxQAEvidenceSegments, 0)
	if err != nil {
		return nil, fmt.Errorf("list collection items: %w", err)
	}
	return s.collectionEvidence(ctx, orgID, items, req.MaxSegments), nil
}
