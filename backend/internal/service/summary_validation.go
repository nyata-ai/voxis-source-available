package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
	"golang.org/x/text/unicode/norm"
)

const (
	maxStructuredSummaryJSONBytes = 1 << 20
	maxStructuredItemIDRunes      = 64
	maxStructuredCitationIDRunes  = 128
	maxStructuredMatrixLanguage   = 64
	maxStructuredSpeakerRunes     = 512
	maxStructuredParagraphRunes   = 4_000
	maxStructuredPointRunes       = 2_000
	maxStructuredActionRunes      = 2_000
	maxStructuredQuestionRunes    = 2_000
	maxStructuredAnswerRunes      = 4_000
	maxStructuredCitations        = 8
	// maxStructuredParagraphs backstops both per-chunk normalization and merged
	// validation: 5 paragraphs per chunk x at most 5 source chunks. The Gemini
	// response schema is what enforces the 5-per-chunk bound.
	maxStructuredParagraphs      = 25
	maxStructuredKeyPoints       = 20
	maxStructuredActionItems     = 30
	maxStructuredQuestionAnswers = 30

	unansweredAnswerText = "Not answered in the transcript."
)

// ParseAndValidateStructuredSummary parses Gemini's selected typed response
// and checks every item against the exact source package sent to Gemini.
func ParseAndValidateStructuredSummary(summaryType string, raw []byte, source *SummarySource) (*domain.StructuredSummary, error) {
	summary, err := ParseStructuredSummary(summaryType, raw)
	if err != nil {
		return nil, err
	}
	normalizeStructuredSummary(summary, source)
	if err := ValidateStructuredSummary(summary, source); err != nil {
		return nil, err
	}
	return summary, nil
}

func normalizeStructuredSummary(summary *domain.StructuredSummary, source *SummarySource) {
	if summary == nil {
		return
	}
	truncateStructuredSummaryItems(summary)
	if summary.QAndA == nil {
		return
	}
	normalizeStructuredQAndAItems(summary, source)
}

// truncateStructuredSummaryItems trims each contract to its item limit,
// recording a degradation code per trimmed contract.
func truncateStructuredSummaryItems(summary *domain.StructuredSummary) {
	if summary.General != nil && len(summary.General.Paragraphs) > maxStructuredParagraphs {
		summary.General.Paragraphs = summary.General.Paragraphs[:maxStructuredParagraphs]
		summary.DegradationCodes = append(summary.DegradationCodes, domain.SummaryDegradationItemLimitApplied)
	}
	if summary.KeyPoints != nil && len(summary.KeyPoints.Items) > maxStructuredKeyPoints {
		summary.KeyPoints.Items = summary.KeyPoints.Items[:maxStructuredKeyPoints]
		summary.DegradationCodes = append(summary.DegradationCodes, domain.SummaryDegradationItemLimitApplied)
	}
	if summary.ActionItems != nil && len(summary.ActionItems.Items) > maxStructuredActionItems {
		summary.ActionItems.Items = summary.ActionItems.Items[:maxStructuredActionItems]
		summary.DegradationCodes = append(summary.DegradationCodes, domain.SummaryDegradationItemLimitApplied)
	}
	if summary.QAndA != nil && len(summary.QAndA.Items) > maxStructuredQuestionAnswers {
		summary.QAndA.Items = summary.QAndA.Items[:maxStructuredQuestionAnswers]
		summary.DegradationCodes = append(summary.DegradationCodes, domain.SummaryDegradationItemLimitApplied)
	}
}

// normalizeStructuredQAndAItems enforces the explicit unanswered form and
// drops exact quotes that are not contained in a cited source segment.
func normalizeStructuredQAndAItems(summary *domain.StructuredSummary, source *SummarySource) {
	segments, err := structuredSourceSegments(source)
	if err != nil {
		return
	}
	for index := range summary.QAndA.Items {
		item := &summary.QAndA.Items[index]
		if item.AnswerStatus == domain.SummaryAnswerStatusUnanswered {
			if item.Answer != unansweredAnswerText || item.AnswerSpeaker != nil || item.AnswerExactQuote != nil {
				item.Answer = unansweredAnswerText
				item.AnswerSpeaker = nil
				item.AnswerExactQuote = nil
				summary.DegradationCodes = append(summary.DegradationCodes, domain.SummaryDegradationUnansweredAnswerNormalized)
			}
			continue
		}
		if item.AnswerExactQuote != nil && validateStructuredExactQuote(item.AnswerExactQuote, item.CitationIDs, segments) != nil {
			item.AnswerExactQuote = nil
			summary.DegradationCodes = append(summary.DegradationCodes, domain.SummaryDegradationExactQuoteDropped)
		}
	}
}

// NormalizeStructuredSummary applies bounded, non-content normalization before validation.
func NormalizeStructuredSummary(summary *domain.StructuredSummary, source *SummarySource) {
	normalizeStructuredSummary(summary, source)
}

// ParseStructuredSummary accepts only the provider-shaped JSON contract for
// the selected summary type. Unknown fields, missing required fields, and
// trailing JSON are rejected before a value reaches the renderer.
func ParseStructuredSummary(summaryType string, raw []byte) (*domain.StructuredSummary, error) {
	if len(raw) == 0 || len(raw) > maxStructuredSummaryJSONBytes {
		return nil, structuredSummaryError("response size is unsupported")
	}

	switch summaryType {
	case domain.SummaryTypeGeneral:
		return parseStructuredGeneralSummary(raw)
	case domain.SummaryTypeKeyPoints:
		return parseStructuredKeyPointsSummary(raw)
	case domain.SummaryTypeActionItems:
		return parseStructuredActionItemsSummary(raw)
	case domain.SummaryTypeQAndA:
		return parseStructuredQuestionAndAnswerSummary(raw)
	default:
		return nil, structuredSummaryError("summary type is unsupported")
	}
}

// ValidateStructuredSummary verifies all semantic constraints after parsing.
// It accepts constructed values as well as values returned by the JSON parser.
func ValidateStructuredSummary(summary *domain.StructuredSummary, source *SummarySource) error {
	segments, err := structuredSourceSegments(source)
	if err != nil {
		return err
	}
	if summary == nil {
		return structuredSummaryError("response is required")
	}

	if err := validateStructuredSummaryShape(summary); err != nil {
		return err
	}

	switch summary.SummaryType {
	case domain.SummaryTypeGeneral:
		return validateStructuredGeneralSummary(summary.General, segments)
	case domain.SummaryTypeKeyPoints:
		return validateStructuredKeyPointsSummary(summary.KeyPoints, segments)
	case domain.SummaryTypeActionItems:
		return validateStructuredActionItemsSummary(summary.ActionItems, segments)
	case domain.SummaryTypeQAndA:
		return validateStructuredQuestionAndAnswerSummary(summary.QAndA, segments)
	default:
		return structuredSummaryError("summary type is unsupported")
	}
}

// structuredContractCount counts the populated per-type contracts.
func structuredContractCount(summary *domain.StructuredSummary) int {
	count := 0
	if summary.General != nil {
		count++
	}
	if summary.KeyPoints != nil {
		count++
	}
	if summary.ActionItems != nil {
		count++
	}
	if summary.QAndA != nil {
		count++
	}
	return count
}

// validateStructuredSummaryShape checks that exactly one contract is set and
// that it matches the declared summary type. Unknown types pass through so the
// caller can report its own unsupported-type error.
func validateStructuredSummaryShape(summary *domain.StructuredSummary) error {
	count := structuredContractCount(summary)
	switch summary.SummaryType {
	case domain.SummaryTypeGeneral:
		if summary.General == nil || count != 1 {
			return structuredSummaryError("general response must have one general contract")
		}
	case domain.SummaryTypeKeyPoints:
		if summary.KeyPoints == nil || count != 1 {
			return structuredSummaryError("key-points response must have one key-points contract")
		}
	case domain.SummaryTypeActionItems:
		if summary.ActionItems == nil || count != 1 {
			return structuredSummaryError("action-items response must have one action-items contract")
		}
	case domain.SummaryTypeQAndA:
		if summary.QAndA == nil || count != 1 {
			return structuredSummaryError("Q&A response must have one Q&A contract")
		}
	}
	return nil
}

// MarshalStructuredSummary returns the selected provider-shaped contract for
// encrypted persistence. The internal union envelope is deliberately omitted.
func MarshalStructuredSummary(summary *domain.StructuredSummary) ([]byte, error) {
	if summary == nil {
		return nil, structuredSummaryError("response is required")
	}

	encoded, err := marshalStructuredSummaryRoot(summary)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func marshalStructuredSummaryRoot(summary *domain.StructuredSummary) ([]byte, error) {
	if validateStructuredSummaryShape(summary) != nil {
		return nil, structuredSummaryError("response contract does not match summary type")
	}
	switch summary.SummaryType {
	case domain.SummaryTypeGeneral:
		return marshalStructuredSummaryValue(summary.General)
	case domain.SummaryTypeKeyPoints:
		return marshalStructuredSummaryValue(summary.KeyPoints)
	case domain.SummaryTypeActionItems:
		return marshalStructuredSummaryValue(summary.ActionItems)
	case domain.SummaryTypeQAndA:
		return marshalStructuredSummaryValue(summary.QAndA)
	default:
		return nil, structuredSummaryError("response contract does not match summary type")
	}
}

func marshalStructuredSummaryValue(value interface{}) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal structured summary: %w", err)
	}
	return encoded, nil
}

type nullableStructuredString struct {
	Present bool
	Value   *string
}

func (value *nullableStructuredString) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if string(raw) == "null" {
		value.Value = nil
		return nil
	}
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	value.Value = &decoded
	return nil
}

type generalSummaryWire struct {
	SchemaVersion  *string                        `json:"schema_version"`
	MatrixLanguage *string                        `json:"matrix_language"`
	Paragraphs     *[]generalSummaryParagraphWire `json:"paragraphs"`
}

type generalSummaryParagraphWire struct {
	ID          *string                  `json:"id"`
	Text        *string                  `json:"text"`
	Speaker     nullableStructuredString `json:"speaker"`
	CitationIDs *[]string                `json:"citation_ids"`
}

type keyPointsSummaryWire struct {
	SchemaVersion  *string                    `json:"schema_version"`
	MatrixLanguage *string                    `json:"matrix_language"`
	Items          *[]keyPointSummaryItemWire `json:"items"`
}

type keyPointSummaryItemWire struct {
	ID          *string                  `json:"id"`
	Text        *string                  `json:"text"`
	Speaker     nullableStructuredString `json:"speaker"`
	CitationIDs *[]string                `json:"citation_ids"`
}

type actionItemsSummaryWire struct {
	SchemaVersion  *string                      `json:"schema_version"`
	MatrixLanguage *string                      `json:"matrix_language"`
	Items          *[]actionItemSummaryItemWire `json:"items"`
}

type actionItemSummaryItemWire struct {
	ID          *string                  `json:"id"`
	Kind        *string                  `json:"kind"`
	Text        *string                  `json:"text"`
	Owner       nullableStructuredString `json:"owner"`
	Deadline    nullableStructuredString `json:"deadline"`
	Speaker     nullableStructuredString `json:"speaker"`
	CitationIDs *[]string                `json:"citation_ids"`
}

type questionAndAnswerSummaryWire struct {
	SchemaVersion  *string                             `json:"schema_version"`
	MatrixLanguage *string                             `json:"matrix_language"`
	Items          *[]questionAndAnswerSummaryItemWire `json:"items"`
}

type questionAndAnswerSummaryItemWire struct {
	ID               *string                  `json:"id"`
	Question         *string                  `json:"question"`
	QuestionSpeaker  nullableStructuredString `json:"question_speaker"`
	Answer           *string                  `json:"answer"`
	AnswerSpeaker    nullableStructuredString `json:"answer_speaker"`
	AnswerStatus     *string                  `json:"answer_status"`
	AnswerExactQuote nullableStructuredString `json:"answer_exact_quote"`
	CitationIDs      *[]string                `json:"citation_ids"`
}

func parseStructuredGeneralSummary(raw []byte) (*domain.StructuredSummary, error) {
	var wire generalSummaryWire
	if err := decodeStructuredSummaryJSON(raw, &wire); err != nil {
		return nil, err
	}
	response, err := generalSummaryFromWire(wire)
	if err != nil {
		return nil, err
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeGeneral, General: response}, nil
}

func parseStructuredKeyPointsSummary(raw []byte) (*domain.StructuredSummary, error) {
	var wire keyPointsSummaryWire
	if err := decodeStructuredSummaryJSON(raw, &wire); err != nil {
		return nil, err
	}
	response, err := keyPointsSummaryFromWire(wire)
	if err != nil {
		return nil, err
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeKeyPoints, KeyPoints: response}, nil
}

func parseStructuredActionItemsSummary(raw []byte) (*domain.StructuredSummary, error) {
	var wire actionItemsSummaryWire
	if err := decodeStructuredSummaryJSON(raw, &wire); err != nil {
		return nil, err
	}
	response, err := actionItemsSummaryFromWire(wire)
	if err != nil {
		return nil, err
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeActionItems, ActionItems: response}, nil
}

func parseStructuredQuestionAndAnswerSummary(raw []byte) (*domain.StructuredSummary, error) {
	var wire questionAndAnswerSummaryWire
	if err := decodeStructuredSummaryJSON(raw, &wire); err != nil {
		return nil, err
	}
	response, err := questionAndAnswerSummaryFromWire(wire)
	if err != nil {
		return nil, err
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeQAndA, QAndA: response}, nil
}

func decodeStructuredSummaryJSON(raw []byte, destination interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return structuredSummaryError("invalid JSON response: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return structuredSummaryError("response contains trailing JSON")
	}
	return nil
}

func generalSummaryFromWire(wire generalSummaryWire) (*domain.StructuredGeneralSummary, error) {
	version, language, err := structuredSummaryHeader(wire.SchemaVersion, wire.MatrixLanguage)
	if err != nil {
		return nil, err
	}
	if wire.Paragraphs == nil {
		return nil, structuredSummaryError("paragraphs is required")
	}
	paragraphs := make([]domain.StructuredSummaryParagraph, 0, len(*wire.Paragraphs))
	for _, item := range *wire.Paragraphs {
		paragraph, err := generalParagraphFromWire(item)
		if err != nil {
			return nil, err
		}
		paragraphs = append(paragraphs, paragraph)
	}
	return &domain.StructuredGeneralSummary{SchemaVersion: version, MatrixLanguage: language, Paragraphs: paragraphs}, nil
}

func generalParagraphFromWire(wire generalSummaryParagraphWire) (domain.StructuredSummaryParagraph, error) {
	id, text, citations, err := structuredItemFields(wire.ID, wire.Text, wire.CitationIDs)
	if err != nil {
		return domain.StructuredSummaryParagraph{}, err
	}
	speaker, err := structuredNullableString(wire.Speaker, "speaker")
	if err != nil {
		return domain.StructuredSummaryParagraph{}, err
	}
	return domain.StructuredSummaryParagraph{ID: id, Text: text, Speaker: speaker, CitationIDs: citations}, nil
}

func keyPointsSummaryFromWire(wire keyPointsSummaryWire) (*domain.StructuredKeyPointsSummary, error) {
	version, language, err := structuredSummaryHeader(wire.SchemaVersion, wire.MatrixLanguage)
	if err != nil {
		return nil, err
	}
	if wire.Items == nil {
		return nil, structuredSummaryError("items is required")
	}
	items := make([]domain.StructuredSummaryKeyPoint, 0, len(*wire.Items))
	for _, item := range *wire.Items {
		point, err := keyPointFromWire(item)
		if err != nil {
			return nil, err
		}
		items = append(items, point)
	}
	return &domain.StructuredKeyPointsSummary{SchemaVersion: version, MatrixLanguage: language, Items: items}, nil
}

func keyPointFromWire(wire keyPointSummaryItemWire) (domain.StructuredSummaryKeyPoint, error) {
	id, text, citations, err := structuredItemFields(wire.ID, wire.Text, wire.CitationIDs)
	if err != nil {
		return domain.StructuredSummaryKeyPoint{}, err
	}
	speaker, err := structuredNullableString(wire.Speaker, "speaker")
	if err != nil {
		return domain.StructuredSummaryKeyPoint{}, err
	}
	return domain.StructuredSummaryKeyPoint{ID: id, Text: text, Speaker: speaker, CitationIDs: citations}, nil
}

func actionItemsSummaryFromWire(wire actionItemsSummaryWire) (*domain.StructuredActionItemsSummary, error) {
	version, language, err := structuredSummaryHeader(wire.SchemaVersion, wire.MatrixLanguage)
	if err != nil {
		return nil, err
	}
	if wire.Items == nil {
		return nil, structuredSummaryError("items is required")
	}
	items := make([]domain.StructuredSummaryActionItem, 0, len(*wire.Items))
	for _, item := range *wire.Items {
		action, err := actionItemFromWire(item)
		if err != nil {
			return nil, err
		}
		items = append(items, action)
	}
	return &domain.StructuredActionItemsSummary{SchemaVersion: version, MatrixLanguage: language, Items: items}, nil
}

func actionItemFromWire(wire actionItemSummaryItemWire) (domain.StructuredSummaryActionItem, error) {
	id, text, citations, err := structuredItemFields(wire.ID, wire.Text, wire.CitationIDs)
	if err != nil {
		return domain.StructuredSummaryActionItem{}, err
	}
	kind, err := requiredStructuredString(wire.Kind, "kind")
	if err != nil {
		return domain.StructuredSummaryActionItem{}, err
	}
	owner, err := structuredNullableString(wire.Owner, "owner")
	if err != nil {
		return domain.StructuredSummaryActionItem{}, err
	}
	deadline, err := structuredNullableString(wire.Deadline, "deadline")
	if err != nil {
		return domain.StructuredSummaryActionItem{}, err
	}
	speaker, err := structuredNullableString(wire.Speaker, "speaker")
	if err != nil {
		return domain.StructuredSummaryActionItem{}, err
	}
	return domain.StructuredSummaryActionItem{ID: id, Kind: kind, Text: text, Owner: owner, Deadline: deadline, Speaker: speaker, CitationIDs: citations}, nil
}

func questionAndAnswerSummaryFromWire(wire questionAndAnswerSummaryWire) (*domain.StructuredQuestionAndAnswerSummary, error) {
	version, language, err := structuredSummaryHeader(wire.SchemaVersion, wire.MatrixLanguage)
	if err != nil {
		return nil, err
	}
	if wire.Items == nil {
		return nil, structuredSummaryError("items is required")
	}
	items := make([]domain.StructuredSummaryQuestionAnswer, 0, len(*wire.Items))
	for _, item := range *wire.Items {
		questionAndAnswer, err := questionAndAnswerFromWire(item)
		if err != nil {
			return nil, err
		}
		items = append(items, questionAndAnswer)
	}
	return &domain.StructuredQuestionAndAnswerSummary{SchemaVersion: version, MatrixLanguage: language, Items: items}, nil
}

func questionAndAnswerFromWire(wire questionAndAnswerSummaryItemWire) (domain.StructuredSummaryQuestionAnswer, error) {
	id, question, citations, err := structuredItemFields(wire.ID, wire.Question, wire.CitationIDs)
	if err != nil {
		return domain.StructuredSummaryQuestionAnswer{}, err
	}
	answer, err := requiredStructuredString(wire.Answer, "answer")
	if err != nil {
		return domain.StructuredSummaryQuestionAnswer{}, err
	}
	status, err := requiredStructuredString(wire.AnswerStatus, "answer_status")
	if err != nil {
		return domain.StructuredSummaryQuestionAnswer{}, err
	}
	questionSpeaker, err := structuredNullableString(wire.QuestionSpeaker, "question_speaker")
	if err != nil {
		return domain.StructuredSummaryQuestionAnswer{}, err
	}
	answerSpeaker, err := structuredNullableString(wire.AnswerSpeaker, "answer_speaker")
	if err != nil {
		return domain.StructuredSummaryQuestionAnswer{}, err
	}
	exactQuote, err := structuredNullableString(wire.AnswerExactQuote, "answer_exact_quote")
	if err != nil {
		return domain.StructuredSummaryQuestionAnswer{}, err
	}
	return domain.StructuredSummaryQuestionAnswer{ID: id, Question: question, QuestionSpeaker: questionSpeaker, Answer: answer, AnswerSpeaker: answerSpeaker, AnswerStatus: status, AnswerExactQuote: exactQuote, CitationIDs: citations}, nil
}

func structuredSummaryHeader(version, language *string) (resolvedVersion, resolvedLanguage string, err error) {
	resolvedVersion, err = requiredStructuredString(version, "schema_version")
	if err != nil {
		return "", "", err
	}
	resolvedLanguage, err = requiredStructuredString(language, "matrix_language")
	if err != nil {
		return "", "", err
	}
	return resolvedVersion, resolvedLanguage, nil
}

func structuredItemFields(id, text *string, citationIDs *[]string) (resolvedID, resolvedText string, resolvedCitations []string, err error) {
	resolvedID, err = requiredStructuredString(id, "id")
	if err != nil {
		return "", "", nil, err
	}
	resolvedText, err = requiredStructuredString(text, "text")
	if err != nil {
		return "", "", nil, err
	}
	if citationIDs == nil {
		return "", "", nil, structuredSummaryError("citation_ids is required")
	}
	return resolvedID, resolvedText, append([]string(nil), (*citationIDs)...), nil
}

func requiredStructuredString(value *string, field string) (string, error) {
	if value == nil {
		return "", structuredSummaryError("%s is required", field)
	}
	return *value, nil
}

func structuredNullableString(value nullableStructuredString, field string) (*string, error) {
	if !value.Present {
		return nil, structuredSummaryError("%s is required and must be null when missing", field)
	}
	return value.Value, nil
}

func structuredSourceSegments(source *SummarySource) (map[string]SummarySourceSegment, error) {
	if source == nil || len(source.Segments) == 0 {
		return nil, structuredSummaryError("summary source is required")
	}
	segments := make(map[string]SummarySourceSegment, len(source.Segments))
	for _, segment := range source.Segments {
		if strings.TrimSpace(segment.ID) == "" || strings.TrimSpace(segment.Text) == "" {
			return nil, structuredSummaryError("summary source segment is incomplete")
		}
		if _, exists := segments[segment.ID]; exists {
			return nil, structuredSummaryError("summary source has duplicate segment IDs")
		}
		segments[segment.ID] = segment
	}
	return segments, nil
}

func validateStructuredGeneralSummary(summary *domain.StructuredGeneralSummary, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredSummaryHeader(summary.SchemaVersion, summary.MatrixLanguage); err != nil {
		return err
	}
	if len(summary.Paragraphs) == 0 || len(summary.Paragraphs) > maxStructuredParagraphs {
		return structuredSummaryError("paragraph count is unsupported")
	}
	ids := make(map[string]struct{}, len(summary.Paragraphs))
	for _, paragraph := range summary.Paragraphs {
		if err := validateStructuredParagraph(paragraph, ids, segments); err != nil {
			return err
		}
	}
	return nil
}

func validateStructuredParagraph(item domain.StructuredSummaryParagraph, ids map[string]struct{}, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredItem(item.ID, item.Text, maxStructuredParagraphRunes, ids); err != nil {
		return err
	}
	if err := validateStructuredOptionalText(item.Speaker, "speaker", maxStructuredSpeakerRunes); err != nil {
		return err
	}
	return validateStructuredCitations(item.CitationIDs, segments)
}

func validateStructuredKeyPointsSummary(summary *domain.StructuredKeyPointsSummary, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredSummaryHeader(summary.SchemaVersion, summary.MatrixLanguage); err != nil {
		return err
	}
	if len(summary.Items) > maxStructuredKeyPoints {
		return structuredSummaryError("key-point count is unsupported")
	}
	ids := make(map[string]struct{}, len(summary.Items))
	for _, item := range summary.Items {
		if err := validateStructuredKeyPoint(item, ids, segments); err != nil {
			return err
		}
	}
	return nil
}

func validateStructuredKeyPoint(item domain.StructuredSummaryKeyPoint, ids map[string]struct{}, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredItem(item.ID, item.Text, maxStructuredPointRunes, ids); err != nil {
		return err
	}
	if err := validateStructuredOptionalText(item.Speaker, "speaker", maxStructuredSpeakerRunes); err != nil {
		return err
	}
	return validateStructuredCitations(item.CitationIDs, segments)
}

func validateStructuredActionItemsSummary(summary *domain.StructuredActionItemsSummary, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredSummaryHeader(summary.SchemaVersion, summary.MatrixLanguage); err != nil {
		return err
	}
	if len(summary.Items) > maxStructuredActionItems {
		return structuredSummaryError("action-item count is unsupported")
	}
	ids := make(map[string]struct{}, len(summary.Items))
	for _, item := range summary.Items {
		if err := validateStructuredActionItem(item, ids, segments); err != nil {
			return err
		}
	}
	return nil
}

func validateStructuredActionItem(item domain.StructuredSummaryActionItem, ids map[string]struct{}, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredItem(item.ID, item.Text, maxStructuredActionRunes, ids); err != nil {
		return err
	}
	if !isStructuredActionKind(item.Kind) {
		return structuredSummaryError("action item kind is unsupported")
	}
	if err := validateActionOptionalText(item.Owner, "owner"); err != nil {
		return err
	}
	if err := validateActionOptionalText(item.Deadline, "deadline"); err != nil {
		return err
	}
	if err := validateActionOptionalText(item.Speaker, "speaker"); err != nil {
		return err
	}
	return validateStructuredCitations(item.CitationIDs, segments)
}

func validateStructuredQuestionAndAnswerSummary(summary *domain.StructuredQuestionAndAnswerSummary, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredSummaryHeader(summary.SchemaVersion, summary.MatrixLanguage); err != nil {
		return err
	}
	if len(summary.Items) > maxStructuredQuestionAnswers {
		return structuredSummaryError("Q&A count is unsupported")
	}
	ids := make(map[string]struct{}, len(summary.Items))
	for _, item := range summary.Items {
		if err := validateStructuredQuestionAndAnswer(item, ids, segments); err != nil {
			return err
		}
	}
	return nil
}

func validateStructuredQuestionAndAnswer(item domain.StructuredSummaryQuestionAnswer, ids map[string]struct{}, segments map[string]SummarySourceSegment) error {
	if err := validateStructuredItem(item.ID, item.Question, maxStructuredQuestionRunes, ids); err != nil {
		return err
	}
	if err := validateStructuredText(item.Answer, "answer", maxStructuredAnswerRunes); err != nil {
		return err
	}
	if err := validateStructuredOptionalText(item.QuestionSpeaker, "question_speaker", maxStructuredSpeakerRunes); err != nil {
		return err
	}
	if err := validateStructuredOptionalText(item.AnswerSpeaker, "answer_speaker", maxStructuredSpeakerRunes); err != nil {
		return err
	}
	if !isStructuredAnswerStatus(item.AnswerStatus) {
		return structuredSummaryError("answer status is unsupported")
	}
	if err := validateUnansweredQuestion(item); err != nil {
		return err
	}
	if err := validateStructuredExactQuote(item.AnswerExactQuote, item.CitationIDs, segments); err != nil {
		return err
	}
	return validateStructuredCitations(item.CitationIDs, segments)
}

func validateStructuredSummaryHeader(version, language string) error {
	if version != domain.StructuredSummarySchemaVersion {
		return structuredSummaryError("schema version is unsupported")
	}
	return validateStructuredText(language, "matrix_language", maxStructuredMatrixLanguage)
}

func validateStructuredItem(id, text string, maxTextRunes int, ids map[string]struct{}) error {
	if err := validateStructuredText(id, "id", maxStructuredItemIDRunes); err != nil {
		return err
	}
	if _, exists := ids[id]; exists {
		return structuredSummaryError("duplicate item ID %q", id)
	}
	ids[id] = struct{}{}
	return validateStructuredText(text, "text", maxTextRunes)
}

func validateStructuredText(value, field string, maxRunes int) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > maxRunes {
		return structuredSummaryError("%s is empty or oversized", field)
	}
	return nil
}

func validateStructuredOptionalText(value *string, field string, maxRunes int) error {
	if value == nil {
		return nil
	}
	return validateStructuredText(*value, field, maxRunes)
}

func validateActionOptionalText(value *string, field string) error {
	return validateStructuredOptionalText(value, field, maxStructuredSpeakerRunes)
}

func validateStructuredCitations(citationIDs []string, segments map[string]SummarySourceSegment) error {
	if len(citationIDs) == 0 || len(citationIDs) > maxStructuredCitations {
		return structuredSummaryError("citation count is unsupported")
	}
	seen := make(map[string]struct{}, len(citationIDs))
	for _, citationID := range citationIDs {
		if err := validateStructuredText(citationID, "citation ID", maxStructuredCitationIDRunes); err != nil {
			return err
		}
		if _, duplicate := seen[citationID]; duplicate {
			return structuredSummaryError("duplicate citation ID %q", citationID)
		}
		if _, exists := segments[citationID]; !exists {
			return structuredSummaryError("citation ID %q is not in the summary source", citationID)
		}
		seen[citationID] = struct{}{}
	}
	return nil
}

// normalizedSummaryText uses Unicode NFC plus whitespace collapsing. It does
// not fold case or remove punctuation, so exact quotes remain exact in meaning.
func normalizedSummaryText(value string) string {
	return norm.NFC.String(strings.Join(strings.Fields(value), " "))
}

func validateStructuredExactQuote(quote *string, citationIDs []string, segments map[string]SummarySourceSegment) error {
	if quote == nil {
		return nil
	}
	if err := validateStructuredText(*quote, "answer_exact_quote", maxStructuredAnswerRunes); err != nil {
		return err
	}
	normalizedQuote := normalizedSummaryText(*quote)
	for _, citationID := range citationIDs {
		if strings.Contains(normalizedSummaryText(segments[citationID].Text), normalizedQuote) {
			return nil
		}
	}
	return structuredSummaryError("answer_exact_quote is not in a cited source segment")
}

func validateUnansweredQuestion(item domain.StructuredSummaryQuestionAnswer) error {
	if item.AnswerStatus != domain.SummaryAnswerStatusUnanswered {
		return nil
	}
	if item.Answer != unansweredAnswerText || item.AnswerSpeaker != nil || item.AnswerExactQuote != nil {
		return structuredSummaryError("unanswered Q&A must use the explicit unanswered form")
	}
	return nil
}

func isStructuredActionKind(kind string) bool {
	return kind == domain.SummaryActionKindActionItem || kind == domain.SummaryActionKindDecision ||
		kind == domain.SummaryActionKindNextStep || kind == domain.SummaryActionKindOpenIssue
}

func isStructuredAnswerStatus(status string) bool {
	return status == domain.SummaryAnswerStatusAnswered || status == domain.SummaryAnswerStatusPartiallyAnswered ||
		status == domain.SummaryAnswerStatusConflicting || status == domain.SummaryAnswerStatusUnanswered
}

func structuredSummaryError(format string, args ...interface{}) error {
	return fmt.Errorf("structured summary: "+format+": %w", append(args, domain.ErrInvalidInput)...)
}
