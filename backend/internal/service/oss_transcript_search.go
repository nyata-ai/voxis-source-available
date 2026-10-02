package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	transcriptSearchCandidatePageSize = 25
	transcriptSearchCursorMaxBytes    = 512
)

// TranscriptSearchPage reports one bounded, keyset-paginated content-search
// candidate page. It deliberately has no total because encrypted transcript
// content is evaluated only as each candidate page is read.
type TranscriptSearchPage struct {
	Matches    []port.TranscriptSegmentMatch
	NextCursor string
	Complete   bool
}

type transcriptSearchCursorPayload struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

// SearchTranscriptMediaPage searches one bounded page of completed transcript
// candidates. A cursor advances the same organization-scoped sequence without
// storing transcript text or silently omitting older candidates.
func (s *MCPService) SearchTranscriptMediaPage(
	ctx context.Context,
	orgID string,
	search string,
	rawCursor string,
) (*TranscriptSearchPage, error) {
	query, err := normalizeSegmentSearch(search)
	if err != nil {
		return nil, err
	}
	if s.transSvc == nil {
		return nil, fmt.Errorf("transcription service not available")
	}
	cursor, err := decodeTranscriptSearchCursor(rawCursor)
	if err != nil {
		return nil, err
	}

	candidates, err := s.transSvc.ListCompletedForTranscriptSearch(
		ctx, orgID, cursor, transcriptSearchCandidatePageSize+1,
	)
	if err != nil {
		return nil, err
	}
	return s.searchTranscriptCandidatePage(ctx, orgID, query, candidates)
}

func (s *MCPService) searchTranscriptCandidatePage(
	ctx context.Context,
	orgID, query string,
	candidates []*domain.Transcription,
) (*TranscriptSearchPage, error) {
	complete := len(candidates) <= transcriptSearchCandidatePageSize
	if !complete {
		candidates = candidates[:transcriptSearchCandidatePageSize]
	}

	matches := make([]port.TranscriptSegmentMatch, 0, len(candidates))
	for _, candidate := range candidates {
		transcription, err := s.transSvc.GetByID(ctx, orgID, candidate.ID)
		if err != nil {
			return nil, fmt.Errorf("read transcript search candidate: %w", err)
		}
		match, found, err := bestTranscriptMatch(transcription, query)
		if err != nil {
			return nil, err
		}
		if found {
			matches = append(matches, match)
		}
	}

	page := &TranscriptSearchPage{Matches: matches, Complete: complete}
	if !complete {
		nextCursor, err := encodeTranscriptSearchCursor(candidates[len(candidates)-1])
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

func bestTranscriptMatch(transcription *domain.Transcription, query string) (port.TranscriptSegmentMatch, bool, error) {
	segments, err := ParseTranscriptSegments(transcription.Utterances, transcription.FullTranscript, transcription.SpeakerMap)
	if err != nil {
		return port.TranscriptSegmentMatch{}, false, fmt.Errorf("parse transcript search candidate: %w", err)
	}
	var best port.TranscriptSegmentMatch
	for _, segment := range segments {
		score := scoreSegment(query, segment.Text)
		if score == 0 {
			continue
		}
		if score > best.Score {
			best = transcriptSegmentMatch(transcription, segment, score)
		}
	}
	return best, best.Score > 0, nil
}

func decodeTranscriptSearchCursor(raw string) (*port.TranscriptSearchCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) > transcriptSearchCursorMaxBytes {
		return nil, fmt.Errorf("transcript search cursor is too long: %w", domain.ErrInvalidInput)
	}
	encoded, decodeErr := base64.RawURLEncoding.DecodeString(raw)
	if decodeErr != nil {
		return nil, fmt.Errorf("invalid transcript search cursor: %w", domain.ErrInvalidInput)
	}
	var payload transcriptSearchCursorPayload
	if unmarshalErr := json.Unmarshal(encoded, &payload); unmarshalErr != nil {
		return nil, fmt.Errorf("invalid transcript search cursor: %w", domain.ErrInvalidInput)
	}
	createdAt, parseErr := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if parseErr != nil || createdAt.IsZero() {
		return nil, fmt.Errorf("invalid transcript search cursor: %w", domain.ErrInvalidInput)
	}
	if _, uuidErr := uuid.Parse(payload.ID); uuidErr != nil {
		return nil, fmt.Errorf("invalid transcript search cursor: %w", domain.ErrInvalidInput)
	}
	return &port.TranscriptSearchCursor{CreatedAt: createdAt, ID: payload.ID}, nil
}

func encodeTranscriptSearchCursor(transcription *domain.Transcription) (string, error) {
	if transcription == nil || transcription.CreatedAt.IsZero() {
		return "", fmt.Errorf("invalid transcript search candidate: %w", domain.ErrInvalidInput)
	}
	if _, err := uuid.Parse(transcription.ID); err != nil {
		return "", fmt.Errorf("invalid transcript search candidate: %w", domain.ErrInvalidInput)
	}
	encoded, err := json.Marshal(transcriptSearchCursorPayload{
		CreatedAt: transcription.CreatedAt.UTC().Format(time.RFC3339Nano), ID: transcription.ID,
	})
	if err != nil {
		return "", fmt.Errorf("encode transcript search cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}
