package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
)

const (
	legacyNoKeyPointsText   = "No key points stated in the transcript."
	legacyNoActionItemsText = "No action items, decisions, next steps, or open issues stated in the transcript."
	legacyNoQuestionsText   = "No questions or requests found in the transcript."
	legacyEvidenceRunes     = 240
)

// RenderStructuredSummary creates the legacy content contract from a validated
// typed response. It intentionally keeps Evidence, semicolon fields, and Q:/A:
// markers so existing REST, export, MCP, and frontend readers remain compatible.
func RenderStructuredSummary(summary *domain.StructuredSummary, source *SummarySource) (string, error) {
	if err := ValidateStructuredSummary(summary, source); err != nil {
		return "", err
	}
	segments, err := structuredSourceSegments(source)
	if err != nil {
		return "", err
	}

	switch summary.SummaryType {
	case domain.SummaryTypeGeneral:
		return renderStructuredGeneralSummary(summary.General, segments), nil
	case domain.SummaryTypeKeyPoints:
		return renderStructuredKeyPointsSummary(summary.KeyPoints, segments), nil
	case domain.SummaryTypeActionItems:
		return renderStructuredActionItemsSummary(summary.ActionItems, segments), nil
	case domain.SummaryTypeQAndA:
		return renderStructuredQuestionAndAnswerSummary(summary.QAndA), nil
	default:
		return "", structuredSummaryError("summary type is unsupported")
	}
}

func renderStructuredGeneralSummary(summary *domain.StructuredGeneralSummary, segments map[string]SummarySourceSegment) string {
	paragraphs := make([]string, 0, len(summary.Paragraphs))
	citationIDs := make([]string, 0, len(summary.Paragraphs)*2)
	for _, paragraph := range summary.Paragraphs {
		paragraphs = append(paragraphs, legacySummaryText(paragraph.Text))
		citationIDs = append(citationIDs, paragraph.CitationIDs...)
	}
	evidence := legacyEvidencePhrases(citationIDs, segments, 4)
	return strings.Join(paragraphs, "\n\n") + "\n\nEvidence: " + legacyQuotedEvidence(evidence)
}

func renderStructuredKeyPointsSummary(summary *domain.StructuredKeyPointsSummary, segments map[string]SummarySourceSegment) string {
	if len(summary.Items) == 0 {
		return legacyNoKeyPointsText
	}

	lines := make([]string, 0, len(summary.Items))
	for _, item := range summary.Items {
		evidence := legacyEvidencePhrases(item.CitationIDs, segments, 1)
		lines = append(lines, fmt.Sprintf("- %s Evidence: %s", legacySummaryText(item.Text), legacyQuote(evidence[0])))
	}
	return strings.Join(lines, "\n")
}

func renderStructuredActionItemsSummary(summary *domain.StructuredActionItemsSummary, segments map[string]SummarySourceSegment) string {
	if len(summary.Items) == 0 {
		return legacyNoActionItemsText
	}

	lines := make([]string, 0, len(summary.Items))
	for index, item := range summary.Items {
		evidence := legacyEvidencePhrases(item.CitationIDs, segments, 1)
		line := fmt.Sprintf("%d. Type: %s; Owner: %s; Due: %s; Item: %s; Evidence: %s",
			index+1, legacyActionKind(item.Kind), legacyOptionalValue(item.Owner), legacyOptionalValue(item.Deadline),
			legacyActionValue(item.Text), legacyQuote(evidence[0]))
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderStructuredQuestionAndAnswerSummary(summary *domain.StructuredQuestionAndAnswerSummary) string {
	if len(summary.Items) == 0 {
		return legacyNoQuestionsText
	}

	items := make([]string, 0, len(summary.Items))
	for _, item := range summary.Items {
		question := "Q: " + legacySpeakerPrefix(item.QuestionSpeaker) + legacySummaryText(item.Question)
		answer := "A: " + legacySpeakerPrefix(item.AnswerSpeaker) + legacySummaryText(item.Answer)
		items = append(items, question+"\n"+answer)
	}
	return strings.Join(items, "\n\n")
}

func legacyEvidencePhrases(citationIDs []string, segments map[string]SummarySourceSegment, limit int) []string {
	phrases := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, citationID := range citationIDs {
		if len(phrases) == limit {
			break
		}
		if _, exists := seen[citationID]; exists {
			continue
		}
		seen[citationID] = struct{}{}
		phrases = append(phrases, legacyEvidenceText(segments[citationID].Text))
	}
	return phrases
}

func legacyEvidenceText(value string) string {
	text := strings.ReplaceAll(legacySummaryText(value), ";", ",")
	if utf8.RuneCountInString(text) <= legacyEvidenceRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:legacyEvidenceRunes]) + "…"
}

func legacyQuotedEvidence(phrases []string) string {
	quoted := make([]string, 0, len(phrases))
	for _, phrase := range phrases {
		quoted = append(quoted, legacyQuote(phrase))
	}
	return strings.Join(quoted, ", ")
}

// legacyQuote wraps a phrase in plain double quotes. Deliberately not %q: the
// legacy rendering must not Go-escape quotes/backslashes inside transcript text.
func legacyQuote(phrase string) string {
	return "\"" + phrase + "\""
}

func legacySummaryText(value string) string {
	return normalizedSummaryText(value)
}

func legacyOptionalValue(value *string) string {
	if value == nil {
		return unansweredActionFieldText()
	}
	return legacyActionValue(*value)
}

func legacyActionValue(value string) string {
	return strings.ReplaceAll(legacySummaryText(value), ";", ",")
}

func unansweredActionFieldText() string {
	return "Not stated in the transcript."
}

func legacySpeakerPrefix(value *string) string {
	if value == nil {
		return ""
	}
	return "[" + legacySummaryText(*value) + "] "
}

func legacyActionKind(kind string) string {
	switch kind {
	case domain.SummaryActionKindActionItem:
		return "Action item"
	case domain.SummaryActionKindDecision:
		return "Decision"
	case domain.SummaryActionKindNextStep:
		return "Next step"
	default:
		return "Open issue"
	}
}
