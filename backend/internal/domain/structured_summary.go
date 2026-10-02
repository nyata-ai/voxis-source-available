package domain

// Structured summary contract identifiers: the schema version plus the
// action-kind and answer-status enumerations shared with the Gemini
// response schema.
const (
	// StructuredSummarySchemaVersion is the first citation-addressable summary
	// response contract shared with the Gemini response schema.
	StructuredSummarySchemaVersion = "structured_summary_v1"

	SummaryActionKindActionItem = "action_item"
	SummaryActionKindDecision   = "decision"
	SummaryActionKindNextStep   = "next_step"
	SummaryActionKindOpenIssue  = "open_issue"

	SummaryAnswerStatusAnswered          = "answered"
	SummaryAnswerStatusPartiallyAnswered = "partially_answered"
	SummaryAnswerStatusConflicting       = "conflicting"
	SummaryAnswerStatusUnanswered        = "unanswered"
)

// StructuredSummary is a typed envelope for exactly one parsed summary
// response. It is not the persisted JSON contract; that remains the selected
// concrete root so the stored payload matches the provider schema.
type StructuredSummary struct {
	SummaryType      string
	DegradationCodes []string `json:"-"`
	General          *StructuredGeneralSummary
	KeyPoints        *StructuredKeyPointsSummary
	ActionItems      *StructuredActionItemsSummary
	QAndA            *StructuredQuestionAndAnswerSummary
}

// StructuredGeneralSummary is the response contract for general summaries.
type StructuredGeneralSummary struct {
	SchemaVersion  string                       `json:"schema_version"`
	MatrixLanguage string                       `json:"matrix_language"`
	Paragraphs     []StructuredSummaryParagraph `json:"paragraphs"`
}

// StructuredKeyPointsSummary is the response contract for key points.
type StructuredKeyPointsSummary struct {
	SchemaVersion  string                      `json:"schema_version"`
	MatrixLanguage string                      `json:"matrix_language"`
	Items          []StructuredSummaryKeyPoint `json:"items"`
}

// StructuredActionItemsSummary is the response contract for the combined
// action-items view: actions, decisions, next steps, and open issues.
type StructuredActionItemsSummary struct {
	SchemaVersion  string                        `json:"schema_version"`
	MatrixLanguage string                        `json:"matrix_language"`
	Items          []StructuredSummaryActionItem `json:"items"`
}

// StructuredQuestionAndAnswerSummary is the response contract for Q&A.
type StructuredQuestionAndAnswerSummary struct {
	SchemaVersion  string                            `json:"schema_version"`
	MatrixLanguage string                            `json:"matrix_language"`
	Items          []StructuredSummaryQuestionAnswer `json:"items"`
}

// StructuredSummaryParagraph is one citation-supported general-summary
// paragraph. Speaker is null when the paragraph is not attributable.
type StructuredSummaryParagraph struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Speaker     *string  `json:"speaker"`
	CitationIDs []string `json:"citation_ids"`
}

// StructuredSummaryKeyPoint is one citation-supported key point.
type StructuredSummaryKeyPoint struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Speaker     *string  `json:"speaker"`
	CitationIDs []string `json:"citation_ids"`
}

// StructuredSummaryActionItem is one item in the combined Action Items view.
// Owner, Deadline, and Speaker are null when the transcript does not state
// them; the renderer supplies the compatible legacy placeholder.
type StructuredSummaryActionItem struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	Owner       *string  `json:"owner"`
	Deadline    *string  `json:"deadline"`
	Speaker     *string  `json:"speaker"`
	CitationIDs []string `json:"citation_ids"`
}

// StructuredSummaryQuestionAnswer preserves Voxis's sanitized,
// verbatim-like answer behavior. Answer is deliberately not an exact quote;
// AnswerExactQuote is optional source evidence and is validated separately.
type StructuredSummaryQuestionAnswer struct {
	ID               string   `json:"id"`
	Question         string   `json:"question"`
	QuestionSpeaker  *string  `json:"question_speaker"`
	Answer           string   `json:"answer"`
	AnswerSpeaker    *string  `json:"answer_speaker"`
	AnswerStatus     string   `json:"answer_status"`
	AnswerExactQuote *string  `json:"answer_exact_quote"`
	CitationIDs      []string `json:"citation_ids"`
}
