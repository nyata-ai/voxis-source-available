package port

import "time"

// ExportFormat represents the output document format.
type ExportFormat string

// Export format constants.
const (
	ExportFormatPDF  ExportFormat = "pdf"
	ExportFormatDOCX ExportFormat = "docx"
	ExportFormatJSON ExportFormat = "json"
)

// ValidExportFormats is the set of supported export formats.
var ValidExportFormats = map[ExportFormat]bool{
	ExportFormatPDF:  true,
	ExportFormatDOCX: true,
	ExportFormatJSON: true,
}

// ExportMIMETypes maps export formats to HTTP content types.
var ExportMIMETypes = map[ExportFormat]string{
	ExportFormatPDF:  "application/pdf",
	ExportFormatDOCX: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	ExportFormatJSON: "application/json",
}

// ExportFileExtensions maps export formats to file extensions.
var ExportFileExtensions = map[ExportFormat]string{
	ExportFormatPDF:  ".pdf",
	ExportFormatDOCX: ".docx",
	ExportFormatJSON: ".json",
}

// ExportData contains all the content needed to generate an export document.
type ExportData struct {
	Title        string
	Filename     string
	Languages    []string
	Duration     float64 // seconds
	SpeakerCount int
	WordCount    int
	CreatedAt    time.Time
	CompletedAt  *time.Time // pointer — may be nil for incomplete

	// Transcript content.
	FullTranscript string
	Utterances     []ExportUtterance
	SpeakerMap     map[string]string

	// Summaries (zero or more, for transcription export).
	Summaries []ExportSummary
}

// ExportUtterance is a single speaker turn in the transcript.
type ExportUtterance struct {
	Speaker int
	Start   float64
	End     float64
	Text    string
	Words   []ExportWord // fallback when Text is empty
	// Confidence is nil when the provider reports none (Speechmatics Melia).
	// A pointer rather than a float64 so an absent score is omitted from the
	// export instead of rendering as a fabricated 0.0.
	Confidence *float64
}

// ExportWord is a single word within an utterance (from the provider's words array).
type ExportWord struct {
	Word  string
	Start float64
	End   float64
	// Confidence is nil when the provider reports none. See ExportUtterance.
	Confidence *float64
}

// ResolvedText returns Text if non-empty, otherwise concatenates Words.
// Matches the fallback logic in domain.CountSpeakersAndWords.
func (u ExportUtterance) ResolvedText() string {
	if u.Text != "" {
		return u.Text
	}
	if len(u.Words) == 0 {
		return ""
	}
	result := u.Words[0].Word
	for _, w := range u.Words[1:] {
		result += " " + w.Word
	}
	return result
}

// ExportSummary is a single summary to include in the export.
type ExportSummary struct {
	Type        string // "general", "key_points", "action_items", "q_and_a"
	Content     string
	WordCount   int
	CompletedAt *time.Time // pointer — matches domain.Summary.CompletedAt
}

// DocumentFormatter generates formatted documents from export data.
type DocumentFormatter interface {
	FormatPDF(data *ExportData) ([]byte, error)
	FormatDOCX(data *ExportData) ([]byte, error)
}
