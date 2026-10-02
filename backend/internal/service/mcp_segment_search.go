package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	defaultSegmentSearchLimit = 10
	maxSegmentSearchLimit     = 50
	defaultSearchCandidates   = 25
)

// SearchTranscriptSegments performs bounded lexical search over decrypted
// transcript segments. It does not persist plaintext indexes or embeddings.
func (s *MCPService) SearchTranscriptSegments(
	ctx context.Context,
	orgID string,
	req port.SearchTranscriptSegmentsRequest,
) ([]port.TranscriptSegmentMatch, error) {
	search, err := normalizeSegmentSearch(req.Search)
	if err != nil {
		return nil, err
	}
	if s.transSvc == nil {
		return nil, fmt.Errorf("transcription service not available")
	}

	transcriptions, err := s.segmentSearchCandidates(ctx, orgID, req.TranscriptionID)
	if err != nil {
		return nil, err
	}

	matches := make([]port.TranscriptSegmentMatch, 0)
	for _, trans := range transcriptions {
		matches = append(matches, s.searchOneTranscript(trans, search)...)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})
	return paginateSegmentMatches(matches, req.Limit, req.Offset), nil
}

func normalizeSegmentSearch(search string) (string, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return "", fmt.Errorf("search query cannot be empty: %w", domain.ErrInvalidInput)
	}
	if len([]rune(search)) > 200 {
		return "", fmt.Errorf("search query cannot exceed 200 characters: %w", domain.ErrInvalidInput)
	}
	return strings.ToLower(search), nil
}

func (s *MCPService) segmentSearchCandidates(ctx context.Context, orgID, transcriptionID string) ([]*domain.Transcription, error) {
	if strings.TrimSpace(transcriptionID) != "" {
		trans, err := s.transSvc.GetByID(ctx, orgID, transcriptionID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []*domain.Transcription{trans}, nil
	}

	items, err := s.transSvc.ListFiltered(ctx, orgID, port.TranscriptionListFilter{
		Status:    domain.TranscriptionStatusCompleted,
		SortBy:    "created_at",
		SortOrder: "desc",
		Limit:     defaultSearchCandidates,
		Offset:    0,
	})
	if err != nil {
		return nil, err
	}
	return s.hydrateSearchCandidates(ctx, orgID, items), nil
}

func (s *MCPService) hydrateSearchCandidates(ctx context.Context, orgID string, items []*domain.Transcription) []*domain.Transcription {
	result := make([]*domain.Transcription, 0, len(items))
	for _, item := range items {
		trans, err := s.transSvc.GetByID(ctx, orgID, item.ID)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			s.logger.Warn("failed to hydrate transcript search candidate", "transcription_id", item.ID, "error", err)
			continue
		}
		result = append(result, trans)
	}
	return result
}

func (s *MCPService) searchOneTranscript(trans *domain.Transcription, search string) []port.TranscriptSegmentMatch {
	segments, err := ParseTranscriptSegments(trans.Utterances, trans.FullTranscript, trans.SpeakerMap)
	if err != nil {
		s.logger.Warn("failed to parse transcript search candidate", "transcription_id", trans.ID, "error", err)
		return nil
	}
	matches := make([]port.TranscriptSegmentMatch, 0, len(segments))
	for _, segment := range segments {
		score := scoreSegment(search, segment.Text)
		if score == 0 {
			continue
		}
		matches = append(matches, transcriptSegmentMatch(trans, segment, score))
	}
	return matches
}

func scoreSegment(search, text string) int {
	normalizedText := strings.ToLower(text)
	score := 0
	if strings.Contains(normalizedText, search) {
		score += 100
	}
	for _, token := range searchTokens(search) {
		if strings.Contains(normalizedText, token) {
			score += 10
		}
	}
	return score
}

func searchTokens(search string) []string {
	raw := strings.FieldsFunc(search, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	tokens := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, token := range raw {
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	return tokens
}

func transcriptSegmentMatch(trans *domain.Transcription, segment TranscriptSegment, score int) port.TranscriptSegmentMatch {
	return port.TranscriptSegmentMatch{
		TranscriptionID: trans.ID,
		MediaID:         trans.MediaID,
		SourceType:      "media",
		DisplayName:     searchDisplayName(trans),
		CreatedAt:       trans.CreatedAt.UTC().Format(time.RFC3339Nano),
		Speaker:         segment.Speaker,
		StartSeconds:    segment.StartSeconds,
		EndSeconds:      segment.EndSeconds,
		Text:            segment.Text,
		Score:           score,
	}
}

func searchDisplayName(trans *domain.Transcription) string {
	if strings.TrimSpace(trans.MediaTitle) != "" {
		return trans.MediaTitle
	}
	if strings.TrimSpace(trans.MediaFilename) != "" {
		return trans.MediaFilename
	}
	return trans.ID
}

func paginateSegmentMatches(matches []port.TranscriptSegmentMatch, limit, offset int) []port.TranscriptSegmentMatch {
	limit = clampSearchLimit(limit)
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matches) {
		return nil
	}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return matches[offset:end]
}

func clampSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSegmentSearchLimit
	}
	if limit > maxSegmentSearchLimit {
		return maxSegmentSearchLimit
	}
	return limit
}
