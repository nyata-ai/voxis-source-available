package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/voxis/backend/internal/domain"
)

// TranscriptionDetailMedia is the media metadata included with a transcript.
type TranscriptionDetailMedia struct {
	ID          string  `json:"id"`
	Filename    string  `json:"filename"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Duration    float64 `json:"duration"`
}

// TranscriptionDetailSummary is the public metadata for one transcript summary.
type TranscriptionDetailSummary struct {
	ID          string     `json:"id"`
	SummaryType string     `json:"summary_type"`
	Status      string     `json:"status"`
	WordCount   int        `json:"word_count"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TranscriptionDetail contains a media-derived transcript and its summary metadata.
type TranscriptionDetail struct {
	Transcription *domain.Transcription        `json:"transcription"`
	Media         *TranscriptionDetailMedia    `json:"media,omitempty"`
	Summaries     []TranscriptionDetailSummary `json:"summaries"`
}

// GetTranscriptionDetail returns one organization-scoped transcript with its summaries.
func (s *TranscriptionService) GetTranscriptionDetail(ctx context.Context, orgID, transcriptionID string) (*TranscriptionDetail, error) {
	transcription, err := s.GetByID(ctx, orgID, transcriptionID)
	if err != nil {
		return nil, err
	}
	result := &TranscriptionDetail{Transcription: transcription, Summaries: []TranscriptionDetailSummary{}}
	media, err := s.mediaRepo.GetByID(ctx, transcription.MediaID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("load media for detail: %w", err)
	}
	if media != nil {
		result.Media = &TranscriptionDetailMedia{ID: media.ID, Filename: media.Filename, Title: media.Title, Description: media.Description, Duration: media.Duration}
	}
	summaries, err := s.summaryRepo.ListByTranscription(ctx, transcriptionID)
	if err != nil {
		return nil, fmt.Errorf("list summaries for detail: %w", err)
	}
	for _, summary := range summaries {
		result.Summaries = append(result.Summaries, TranscriptionDetailSummary{ID: summary.ID, SummaryType: summary.SummaryType, Status: summary.Status, WordCount: summary.WordCount, CompletedAt: summary.CompletedAt})
	}
	return result, nil
}
