package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/voxis/backend/internal/domain"
)

// maxExaminationSummaryScan bounds the summary scan for a transcription.
// At most one active summary per type exists, so this only guards against a
// repository returning an unexpectedly large set.
const maxExaminationSummaryScan = 64

// ExaminationCitedSegment is one citation-resolved source utterance. Unlike
// SummaryCitation (API/MCP presentation) it carries the utterance Text, which
// the Berita Acara Pemeriksaan export needs to render answers in the
// examinee's own words.
type ExaminationCitedSegment struct {
	Speaker      string // "" when the utterance is not attributable
	StartSeconds *float64
	EndSeconds   *float64
	Text         string
}

// ExaminationQuestionAnswer is one structured Q&A item with its citations
// resolved to source text. AnswerExactQuote is empty when the model produced
// no verbatim quote for the item.
type ExaminationQuestionAnswer struct {
	Question         string
	QuestionSpeaker  string
	Answer           string
	AnswerSpeaker    string
	AnswerExactQuote string
	CitedSegments    []ExaminationCitedSegment
}

// ExaminationQuestionAnswers returns the structured Q&A items of the
// transcription's completed q_and_a summary, with every citation resolved to
// the source utterance text.
//
// The caller passes the already-decrypted transcription (the export handler
// has one) because citation resolution re-derives the source package from the
// transcript and compares its hash with the one recorded on the summary.
//
// Three failure modes are distinguished so the handler can advise the user:
//   - domain.ErrExaminationSummaryMissing — no completed q_and_a summary
//   - domain.ErrExaminationSummaryUnstructured — summary has no structured content
//   - domain.ErrExaminationSourceMismatch — citations no longer resolve
func (s *SummaryService) ExaminationQuestionAnswers(
	ctx context.Context,
	orgID string,
	trans *domain.Transcription,
) ([]ExaminationQuestionAnswer, error) {
	if trans == nil {
		return nil, fmt.Errorf("transcription is required: %w", domain.ErrInvalidInput)
	}
	if orgID == "" {
		return nil, fmt.Errorf("organization id is required: %w", domain.ErrInvalidInput)
	}

	summary, err := s.completedQAndASummary(ctx, orgID, trans.ID)
	if err != nil {
		return nil, err
	}

	structured, ok := parsePresentationStructuredContent(summary)
	if !ok || structured.QAndA == nil {
		return nil, domain.ErrExaminationSummaryUnstructured
	}

	source, ok := presentationSource(summary, trans)
	if !ok {
		return nil, domain.ErrExaminationSourceMismatch
	}

	return examinationItems(structured.QAndA.Items, source), nil
}

// completedQAndASummary loads the transcription's completed q_and_a summary
// with its content (including structured content) decrypted.
func (s *SummaryService) completedQAndASummary(
	ctx context.Context,
	orgID, transcriptionID string,
) (*domain.Summary, error) {
	items, err := s.ListByTranscription(ctx, orgID, transcriptionID)
	if err != nil {
		return nil, err
	}
	for i, item := range items {
		if i >= maxExaminationSummaryScan {
			break
		}
		if item.SummaryType != domain.SummaryTypeQAndA || item.Status != domain.SummaryStatusCompleted {
			continue
		}
		return s.GetByID(ctx, orgID, item.ID)
	}
	return nil, domain.ErrExaminationSummaryMissing
}

// examinationItems maps structured Q&A items to citation-resolved items,
// preserving transcript order.
func examinationItems(
	items []domain.StructuredSummaryQuestionAnswer,
	source *SummarySource,
) []ExaminationQuestionAnswer {
	byID := make(map[string]SummarySourceSegment, len(source.Segments))
	for _, segment := range source.Segments {
		byID[segment.ID] = segment
	}

	resolved := make([]ExaminationQuestionAnswer, 0, len(items))
	for _, item := range items {
		resolved = append(resolved, ExaminationQuestionAnswer{
			Question:         strings.TrimSpace(item.Question),
			QuestionSpeaker:  optionalString(item.QuestionSpeaker),
			Answer:           strings.TrimSpace(item.Answer),
			AnswerSpeaker:    optionalString(item.AnswerSpeaker),
			AnswerExactQuote: optionalString(item.AnswerExactQuote),
			CitedSegments:    examinationCitedSegments(item.CitationIDs, byID),
		})
	}
	return resolved
}

// examinationCitedSegments resolves citation ids to source segments, keeping
// the item's own order and dropping duplicates and unknown ids.
func examinationCitedSegments(
	ids []string,
	byID map[string]SummarySourceSegment,
) []ExaminationCitedSegment {
	if len(ids) == 0 {
		return nil
	}
	segments := make([]ExaminationCitedSegment, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		segment, exists := byID[id]
		if !exists {
			continue
		}
		seen[id] = struct{}{}
		segments = append(segments, ExaminationCitedSegment{
			Speaker:      optionalString(segment.Speaker),
			StartSeconds: segment.StartSeconds,
			EndSeconds:   segment.EndSeconds,
			Text:         strings.TrimSpace(segment.Text),
		})
	}
	if len(segments) == 0 {
		return nil
	}
	return segments
}

// optionalString flattens an optional structured-summary field.
func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
