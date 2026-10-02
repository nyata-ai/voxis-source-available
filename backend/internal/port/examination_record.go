package port

import "time"

// ExportTemplate selects the document template used by an export.
type ExportTemplate string

// Export template constants. The default template is the generic transcript
// document; "bap" renders an Indonesian Berita Acara Pemeriksaan draft.
const (
	ExportTemplateStandard ExportTemplate = "standard"
	ExportTemplateBAP      ExportTemplate = "bap"
)

// ValidExportTemplates is the set of supported export templates.
var ValidExportTemplates = map[ExportTemplate]bool{
	ExportTemplateStandard: true,
	ExportTemplateBAP:      true,
}

// BAPAnswerSource records which fidelity tier supplied a rendered answer.
// KUHAP Pasal 117(2) requires the examinee's own words, so the renderer must
// be able to mark answers that are only a summary of what was said.
type BAPAnswerSource string

// BAP answer fidelity tiers, best first.
const (
	// BAPAnswerExactQuote is the model's verbatim quote of the answer.
	BAPAnswerExactQuote BAPAnswerSource = "exact_quote"
	// BAPAnswerCitation is the concatenated text of the cited source utterances.
	BAPAnswerCitation BAPAnswerSource = "citation"
	// BAPAnswerSummary is the summarized answer — rendered with a visible
	// "periksa rekaman" marker because it is not verbatim.
	BAPAnswerSummary BAPAnswerSource = "summary"
)

// BAPQuestionAnswer is one numbered question/answer pair ready for rendering.
// Answer already carries the text chosen by the fidelity rule; AnswerSource
// tells the renderer whether to mark it.
type BAPQuestionAnswer struct {
	Number          int
	Question        string
	QuestionSpeaker string // "" when the transcript does not attribute it
	Answer          string
	AnswerSpeaker   string // "" when the transcript does not attribute it
	AnswerSource    BAPAnswerSource
}

// BAPExportData contains everything needed to render a draft Berita Acara
// Pemeriksaan. Every timestamp is supplied by the caller — the formatter never
// reads a clock, so its output is deterministic and testable.
type BAPExportData struct {
	// Provenance of the examined recording.
	Title           string
	MediaFilename   string
	AudioSHA256     string
	RecordingDate   time.Time // zero when neither forensics nor a fallback supplied one
	Languages       []string
	DurationSeconds float64

	// Transcription provenance.
	TranscriptionID string
	CreatedAt       time.Time
	CompletedAt     *time.Time

	// GeneratedAt stamps the provenance footer.
	GeneratedAt time.Time

	// Items are the examination questions and answers in transcript order.
	Items []BAPQuestionAnswer
}

// ExaminationRecordFormatter renders a draft Berita Acara Pemeriksaan.
//
// DOCX only, deliberately: examiners finish the document in Word, so there is
// no PDF counterpart here. This is a conscious divergence from
// DocumentFormatter's PDF/DOCX parity and is not an oversight.
type ExaminationRecordFormatter interface {
	FormatBAP(data *BAPExportData) ([]byte, error)
}
