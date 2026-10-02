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
	defaultCollectionLimit = 20
	maxCollectionLimit     = 100
)

// CreateCollection creates a named MCP transcript collection.
func (s *MCPService) CreateCollection(
	ctx context.Context,
	orgID, name, description, createdBy string,
) (*port.MCPCollection, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("collection name cannot be empty: %w", domain.ErrInvalidInput)
	}
	if len([]rune(name)) > 120 {
		return nil, fmt.Errorf("collection name cannot exceed 120 characters: %w", domain.ErrInvalidInput)
	}
	if len([]rune(description)) > 1000 {
		return nil, fmt.Errorf("collection description cannot exceed 1000 characters: %w", domain.ErrInvalidInput)
	}
	c, err := s.mcpRepo.CreateCollection(ctx, orgID, name, strings.TrimSpace(description), createdBy)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}
	return c, nil
}

// ListCollections lists MCP collections for an organization.
func (s *MCPService) ListCollections(ctx context.Context, orgID string, limit, offset int) ([]port.MCPCollection, error) {
	items, err := s.mcpRepo.ListCollections(ctx, orgID, clampCollectionLimit(limit), clampNonNegative(offset))
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	return items, nil
}

// AddCollectionItem adds a transcription to a collection.
func (s *MCPService) AddCollectionItem(ctx context.Context, orgID, collectionID, transcriptionID string) error {
	if s.transSvc == nil {
		return fmt.Errorf("transcription service not available")
	}
	if _, err := s.mcpRepo.GetCollection(ctx, orgID, collectionID); err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	if _, err := s.transSvc.GetByID(ctx, orgID, transcriptionID); err != nil {
		return err
	}
	if err := s.mcpRepo.AddCollectionItem(ctx, orgID, collectionID, transcriptionID); err != nil {
		return fmt.Errorf("add collection item: %w", err)
	}
	return nil
}

// RemoveCollectionItem removes a transcription from a collection.
func (s *MCPService) RemoveCollectionItem(ctx context.Context, orgID, collectionID, transcriptionID string) error {
	if _, err := s.mcpRepo.GetCollection(ctx, orgID, collectionID); err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	if err := s.mcpRepo.RemoveCollectionItem(ctx, orgID, collectionID, transcriptionID); err != nil {
		return fmt.Errorf("remove collection item: %w", err)
	}
	return nil
}

// ListCollectionItems lists transcription items in a collection.
func (s *MCPService) ListCollectionItems(
	ctx context.Context,
	orgID, collectionID string,
	limit, offset int,
) ([]port.MCPCollectionItem, error) {
	if _, err := s.mcpRepo.GetCollection(ctx, orgID, collectionID); err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	items, err := s.mcpRepo.ListCollectionItems(ctx, orgID, collectionID, clampCollectionLimit(limit), clampNonNegative(offset))
	if err != nil {
		return nil, fmt.Errorf("list collection items: %w", err)
	}
	return items, nil
}

// AskCollection answers a question using bounded evidence from collection items.
func (s *MCPService) AskCollection(
	ctx context.Context,
	orgID string,
	req port.AskCollectionRequest,
) (*port.AskTranscriptResult, error) {
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return nil, fmt.Errorf("question cannot be empty: %w", domain.ErrInvalidInput)
	}
	if len([]rune(question)) > 1000 {
		return nil, fmt.Errorf("question cannot exceed 1000 characters: %w", domain.ErrInvalidInput)
	}
	if s.transSvc == nil || s.answerer == nil {
		return nil, fmt.Errorf("collection answer dependencies not available")
	}
	items, err := s.ListCollectionItems(ctx, orgID, req.CollectionID, maxQAEvidenceSegments, 0)
	if err != nil {
		return nil, fmt.Errorf("list collection items: %w", err)
	}
	evidence := s.collectionEvidence(ctx, orgID, items, req.MaxSegments)
	if len(evidence) == 0 {
		return &port.AskTranscriptResult{Answer: "I could not find cited transcript evidence to answer this question.", Refusal: true}, nil
	}
	answer, err := s.answerer.AnswerTranscriptQuestion(ctx, question, evidence)
	if err != nil {
		return nil, fmt.Errorf("answer collection question: %w", err)
	}
	return buildAskTranscriptResult(answer, evidence)
}

func (s *MCPService) collectionEvidence(
	ctx context.Context,
	orgID string,
	items []port.MCPCollectionItem,
	maxSegments int,
) []port.TranscriptEvidence {
	limit := clampQAEvidenceLimit(maxSegments)
	evidence := make([]port.TranscriptEvidence, 0, limit)
	for _, item := range items {
		if len(evidence) >= limit {
			break
		}
		trans, err := s.transSvc.GetByID(ctx, orgID, item.TranscriptionID)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			s.logger.Warn("failed to load collection transcription", "transcription_id", item.TranscriptionID, "error", err)
			continue
		}
		segments, err := ParseTranscriptSegments(trans.Utterances, trans.FullTranscript, trans.SpeakerMap)
		if err != nil {
			continue
		}
		for _, segment := range segments {
			if len(evidence) >= limit {
				break
			}
			evidence = append(evidence, port.TranscriptEvidence{
				ID:              fmt.Sprintf("e%d", len(evidence)+1),
				TranscriptionID: trans.ID,
				MediaID:         trans.MediaID,
				SourceType:      "media",
				DisplayName:     searchDisplayName(trans),
				Speaker:         segment.Speaker,
				StartSeconds:    segment.StartSeconds,
				EndSeconds:      segment.EndSeconds,
				Text:            segment.Text,
			})
		}
	}
	return evidence
}

func clampCollectionLimit(limit int) int {
	if limit <= 0 {
		return defaultCollectionLimit
	}
	if limit > maxCollectionLimit {
		return maxCollectionLimit
	}
	return limit
}

func clampNonNegative(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
