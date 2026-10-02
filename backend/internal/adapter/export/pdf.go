package export

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/phpdave11/gofpdf"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// CJK languages that the core PDF fonts cannot render.
var cjkLanguages = map[string]bool{"zh": true, "ja": true, "ko": true}

// summaryTypeLabels maps summary types to display labels.
var summaryTypeLabels = map[string]string{
	"general":      "General Summary",
	"key_points":   "Key Points",
	"action_items": "Action Items",
	"q_and_a":      "Questions & Answers",
}

// Page geometry in millimeters and type sizes in points.
const (
	pdfMargin       = 20.0
	pdfFont         = "Helvetica"
	pdfTitleSize    = 16.0
	pdfTitleLine    = 7.0
	pdfHeadingSize  = 13.0
	pdfHeadingLine  = 6.5
	pdfBodySize     = 10.0
	pdfBodyLine     = 5.0
	pdfMetaSize     = 9.0
	pdfMetaLine     = 4.8
	pdfMetaLabelW   = 26.0
	pdfSpeakerSize  = 9.0
	pdfSpeakerLine  = 4.5
	pdfStampSize    = 8.0
	pdfEvidenceSize = 9.0
	pdfEvidenceLine = 4.6
	pdfFooterSize   = 8.0
	pdfFooterY      = -14.0
	pdfListIndent   = 7.0
	pdfParaGap      = 2.5
	pdfItemGap      = 1.5
	pdfTurnGap      = 3.0
	pdfSectionGap   = 6.0
)

type rgb struct{ r, g, b int }

var (
	pdfInk   = rgb{0, 0, 0}
	pdfDark  = rgb{40, 40, 40}
	pdfMuted = rgb{115, 115, 115}
	pdfRule  = rgb{190, 190, 190}
)

var (
	// labelRE matches the leading labels the legacy summary contract emits
	// (service.RenderStructuredSummary). A closed set, because free prose in
	// a general summary may also open with "Word:" and must not be bolded.
	labelRE = regexp.MustCompile(`^(Q|A|Evidence|Type|Owner|Due|Item):\s*(.*)$`)
	// bulletRE and numberRE match the list markers the summary renderers emit.
	bulletRE = regexp.MustCompile(`^[-*•]\s+(\S.*)$`)
	numberRE = regexp.MustCompile(`^(\d{1,3})[.)]\s+(\S.*)$`)
)

// Formatter implements port.DocumentFormatter using gofpdf (PDF) and godocx (DOCX).
type Formatter struct {
	logger *slog.Logger
}

// NewDocumentFormatter creates a new Formatter.
func NewDocumentFormatter() *Formatter {
	return &Formatter{logger: slog.Default()}
}

// NewDocumentFormatterWithLogger creates a Formatter with a custom logger.
func NewDocumentFormatterWithLogger(logger *slog.Logger) *Formatter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Formatter{logger: logger}
}

// Compile-time check.
var _ port.DocumentFormatter = (*Formatter)(nil)

// formatTimestamp converts seconds to "HH:MM:SS" format.
func formatTimestamp(seconds float64) string {
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// speakerName returns the speaker name from the map or a default label.
func speakerName(speaker int, speakerMap map[string]string) string {
	key := fmt.Sprintf("%d", speaker)
	if name, ok := speakerMap[key]; ok && name != "" {
		return name
	}
	return domain.DefaultSpeakerLabel(speaker)
}

// pdfWriter carries the drawing state for one export. gofpdf accumulates its
// own errors internally; FormatPDF checks pdf.Error() once at the end.
type pdfWriter struct {
	pdf *gofpdf.Fpdf
	tr  func(string) string
}

// FormatPDF generates a PDF document from export data.
//
// It draws with gofpdf directly rather than a row-based layout library:
// transcript turns and summary paragraphs are flowed text of arbitrary length,
// which must wrap to the page width and split across pages mid-paragraph.
func (f *Formatter) FormatPDF(data *port.ExportData) ([]byte, error) {
	if data == nil {
		return nil, errors.New("export data is nil")
	}
	f.warnCJK(data)

	pdf := gofpdf.New("P", "mm", "A4", "")
	w := &pdfWriter{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}

	pdf.SetTitle(data.Title, true)
	pdf.SetCreator("Voxis", true)
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetAutoPageBreak(true, pdfMargin)
	pdf.AliasNbPages("{nb}")
	pdf.SetFooterFunc(w.footer)

	pdf.AddPage()
	w.header(data)
	w.transcript(data)
	w.summaries(data)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("generating PDF: %w", err)
	}
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("generating PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func (f *Formatter) warnCJK(data *port.ExportData) {
	for _, lang := range data.Languages {
		if cjkLanguages[lang] {
			f.logger.Warn("PDF export for CJK language may have font rendering issues",
				"languages", data.Languages,
			)
			return
		}
	}
}

func (w *pdfWriter) font(style string, size float64, c rgb) {
	w.pdf.SetFont(pdfFont, style, size)
	w.pdf.SetTextColor(c.r, c.g, c.b)
}

// width is the writable width between the current left and right margins, so
// it shrinks while a list item is indented.
func (w *pdfWriter) width() float64 {
	pageW, _ := w.pdf.GetPageSize()
	left, _, right, _ := w.pdf.GetMargins()
	return pageW - left - right
}

// keepTogether starts a new page when h millimeters will not fit below the
// cursor, so a heading or a speaker line never strands at a page foot.
func (w *pdfWriter) keepTogether(h float64) {
	_, pageH := w.pdf.GetPageSize()
	_, _, _, bottom := w.pdf.GetMargins()
	if w.pdf.GetY()+h > pageH-bottom {
		w.pdf.AddPage()
	}
}

// footer stamps every page with the generator and the page count.
func (w *pdfWriter) footer() {
	pdf := w.pdf
	// The body may have indented the left margin; the footer spans the page.
	left, _, _, _ := pdf.GetMargins()
	pdf.SetLeftMargin(pdfMargin)
	pdf.SetY(pdfFooterY)
	w.font("I", pdfFooterSize, pdfMuted)
	half := w.width() / 2
	pdf.CellFormat(half, pdfBodyLine, w.tr("Generated by Voxis"), "", 0, "L", false, 0, "")
	pdf.CellFormat(half, pdfBodyLine, fmt.Sprintf("%d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	pdf.SetLeftMargin(left)
}

// header draws the title and the metadata block on the first page.
func (w *pdfWriter) header(data *port.ExportData) {
	pdf := w.pdf
	if data.Title != "" {
		w.font("B", pdfTitleSize, pdfInk)
		pdf.MultiCell(w.width(), pdfTitleLine, w.tr(data.Title), "", "L", false)
		pdf.Ln(2)
	}

	for _, row := range metadataRows(data) {
		w.font("", pdfMetaSize, pdfMuted)
		pdf.CellFormat(pdfMetaLabelW, pdfMetaLine, w.tr(row[0]), "", 0, "L", false, 0, "")
		w.font("", pdfMetaSize, pdfInk)
		pdf.MultiCell(w.width()-pdfMetaLabelW, pdfMetaLine, w.tr(row[1]), "", "L", false)
	}

	pdf.Ln(1.5)
	w.rule()
	pdf.Ln(2)
}

func metadataRows(data *port.ExportData) [][2]string {
	rows := make([][2]string, 0, 6)
	if len(data.Languages) > 0 {
		rows = append(rows, [2]string{"Language", strings.Join(data.Languages, ", ")})
	}
	if data.Duration > 0 {
		rows = append(rows, [2]string{"Duration", formatTimestamp(data.Duration)})
	}
	if data.SpeakerCount > 0 {
		rows = append(rows, [2]string{"Speakers", fmt.Sprintf("%d", data.SpeakerCount)})
	}
	if data.WordCount > 0 {
		rows = append(rows, [2]string{"Words", fmt.Sprintf("%d", data.WordCount)})
	}
	if !data.CreatedAt.IsZero() {
		rows = append(rows, [2]string{"Created", data.CreatedAt.Format("2006-01-02 15:04")})
	}
	if data.CompletedAt != nil {
		rows = append(rows, [2]string{"Completed", data.CompletedAt.Format("2006-01-02 15:04")})
	}
	return rows
}

// rule draws a thin horizontal line across the content width at the cursor.
func (w *pdfWriter) rule() {
	pdf := w.pdf
	pageW, _ := pdf.GetPageSize()
	y := pdf.GetY()
	pdf.SetDrawColor(pdfRule.r, pdfRule.g, pdfRule.b)
	pdf.SetLineWidth(0.2)
	pdf.Line(pdfMargin, y, pageW-pdfMargin, y)
}

func (w *pdfWriter) sectionHeading(title string) {
	pdf := w.pdf
	pdf.Ln(pdfSectionGap)
	// Two body lines of room, so a heading never lands alone at a page foot.
	w.keepTogether(pdfHeadingLine + 2*pdfBodyLine)
	w.font("B", pdfHeadingSize, pdfInk)
	pdf.MultiCell(w.width(), pdfHeadingLine, w.tr(title), "", "L", false)
	pdf.Ln(1.5)
}

// paragraph flows one block of text from the left margin at the given size.
func (w *pdfWriter) paragraph(text string, size, line float64, color rgb) {
	w.font("", size, color)
	left, _, _, _ := w.pdf.GetMargins()
	w.pdf.SetX(left)
	w.pdf.MultiCell(w.width(), line, w.tr(normalizeNewlines(text)), "", "L", false)
}

// transcript draws the utterances, the full text, or a placeholder. When
// summaries are present but no transcript content exists (summary-only
// export), the section is omitted entirely to avoid a confusing placeholder.
func (w *pdfWriter) transcript(data *port.ExportData) {
	switch {
	case len(data.Utterances) > 0:
		w.sectionHeading("Transcript")
		for _, u := range data.Utterances {
			w.utterance(u, data.SpeakerMap)
		}
	case data.FullTranscript != "":
		w.sectionHeading("Transcript")
		for _, block := range textBlocks(data.FullTranscript) {
			w.paragraph(strings.Join(block, "\n"), pdfBodySize, pdfBodyLine, pdfInk)
			w.pdf.Ln(pdfParaGap)
		}
	default:
		if len(data.Summaries) > 0 {
			return
		}
		w.font("I", pdfBodySize, pdfMuted)
		w.pdf.MultiCell(w.width(), pdfBodyLine, w.tr("No transcript content."), "", "L", false)
	}
}

// utterance draws one speaker turn: a bold name with a muted timestamp on the
// same line, then the flowed text. The name line is kept with the first line
// of text so it never ends a page on its own.
func (w *pdfWriter) utterance(u port.ExportUtterance, speakerMap map[string]string) {
	pdf := w.pdf
	// Room for a name that wraps once, plus the first line of text.
	w.keepTogether(2*pdfSpeakerLine + pdfBodyLine)

	w.font("B", pdfSpeakerSize, pdfDark)
	pdf.Write(pdfSpeakerLine, w.tr(speakerName(u.Speaker, speakerMap)))
	w.font("", pdfStampSize, pdfMuted)
	stamp := fmt.Sprintf("  [%s - %s]", formatTimestamp(u.Start), formatTimestamp(u.End))
	pdf.Write(pdfSpeakerLine, w.tr(stamp))
	pdf.Ln(pdfSpeakerLine)

	if text := u.ResolvedText(); text != "" {
		w.paragraph(text, pdfBodySize, pdfBodyLine, pdfInk)
	}
	pdf.Ln(pdfTurnGap)
}

func (w *pdfWriter) summaries(data *port.ExportData) {
	for _, s := range data.Summaries {
		label := summaryTypeLabels[s.Type]
		if label == "" {
			label = s.Type
		}
		w.sectionHeading(label)
		for _, block := range textBlocks(s.Content) {
			w.summaryBlock(block)
		}
	}
}

// summaryBlock draws one blank-line-delimited block of summary text. List
// lines become hanging-indent items; the other lines are drawn one per line,
// which keeps a "Q:" / "A:" pair together and lets a leading label be bold.
func (w *pdfWriter) summaryBlock(lines []string) {
	for _, line := range lines {
		if marker, body, ok := listItem(line); ok {
			w.listItem(marker, body)
			continue
		}
		w.summaryLine(line)
	}
	w.pdf.Ln(pdfParaGap)
}

// summaryLine draws a line with its leading label in bold. Evidence lines are
// supporting material, so they are set smaller and muted.
func (w *pdfWriter) summaryLine(line string) {
	label, rest := splitLabel(line)
	size, lh, color := pdfBodySize, pdfBodyLine, pdfInk
	if strings.EqualFold(label, "Evidence") {
		size, lh, color = pdfEvidenceSize, pdfEvidenceLine, pdfMuted
	}

	pdf := w.pdf
	left, _, _, _ := pdf.GetMargins()
	pdf.SetX(left)
	if label != "" {
		w.font("B", size, color)
		pdf.Write(lh, w.tr(label+": "))
	}
	w.font("", size, color)
	pdf.Write(lh, w.tr(rest))
	pdf.Ln(lh)
}

// listItem draws a marker in the gutter and the item lines with a hanging
// indent, so wrapped lines align with the text rather than the marker.
func (w *pdfWriter) listItem(marker, body string) {
	pdf := w.pdf
	w.keepTogether(2 * pdfBodyLine)

	pdf.SetLeftMargin(pdfMargin)
	pdf.SetX(pdfMargin)
	w.font("", pdfBodySize, pdfInk)
	pdf.CellFormat(pdfListIndent, pdfBodyLine, w.tr(marker), "", 0, "L", false, 0, "")

	pdf.SetLeftMargin(pdfMargin + pdfListIndent)
	pdf.SetX(pdfMargin + pdfListIndent)
	for _, line := range itemLines(body) {
		w.summaryLine(line)
	}
	pdf.SetLeftMargin(pdfMargin)
	pdf.Ln(pdfItemGap)
}

// textBlocks splits text into paragraphs on blank lines. Each block holds its
// trimmed, non-empty lines.
func textBlocks(text string) [][]string {
	var blocks [][]string
	var current []string
	for _, raw := range strings.Split(normalizeNewlines(text), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

func normalizeNewlines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// listItem recognizes a bulleted or numbered line and returns its marker and body.
func listItem(line string) (marker, body string, ok bool) {
	if m := bulletRE.FindStringSubmatch(line); m != nil {
		return "•", m[1], true
	}
	if m := numberRE.FindStringSubmatch(line); m != nil {
		return m[1] + ".", m[2], true
	}
	return "", "", false
}

// splitLabel separates a short leading label from the rest of the line.
func splitLabel(line string) (label, rest string) {
	m := labelRE.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", line
	}
	return m[1], m[2]
}

// itemLines splits a rendered list item into display lines. A key point ends
// with an " Evidence: " tail, and the action-item contract packs "Label: value"
// fields separated by "; " (values carry no semicolons). Both read better as a
// small record than as one run-on sentence. The tail is peeled first so a
// semicolon inside key-point prose cannot be mistaken for a field separator.
func itemLines(body string) []string {
	lines := []string{body}
	if i := strings.LastIndex(body, " Evidence: "); i > 0 {
		lines = []string{body[:i], strings.TrimSpace(body[i:])}
	}
	head := strings.TrimSuffix(lines[0], ";")
	if fields := strings.Split(head, "; "); len(fields) > 1 && allLabeled(fields) {
		lines = append(fields, lines[1:]...)
	}
	return lines
}

func allLabeled(fields []string) bool {
	for _, field := range fields {
		if label, _ := splitLabel(field); label == "" {
			return false
		}
	}
	return true
}
