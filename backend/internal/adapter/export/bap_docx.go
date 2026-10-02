package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gomutex/godocx"
	"github.com/gomutex/godocx/docx"
	"github.com/gomutex/godocx/wml/stypes"
	"github.com/voxis/backend/internal/port"
)

// ---------------------------------------------------------------------------
// PROVISIONAL WORDING — the Indonesian boilerplate below (pembukaan and
// penutup formulas) is a working draft. The owner, and ideally a practitioner
// reviewer, must finalize every string here before this export is enabled for
// any external user. Structure follows KUHAP Pasal 117(2): statements in the
// examinee's own words. The output is deliberately lean — it exists to save
// typing, and a penyidik reviews and completes it before signing. Provenance
// lives in the file's document properties (see attachBAPCoreProperties), never
// on a printed page.
// ---------------------------------------------------------------------------

const (
	bapProJustitia   = "PRO JUSTITIA"
	bapDocumentTitle = "BERITA ACARA PEMERIKSAAN"
	bapBodyHeading   = "PERTANYAAN DAN JAWABAN"
	bapIdentityHead  = "IDENTITAS YANG DIPERIKSA"

	bapClosingFormula = "Demikian Berita Acara Pemeriksaan ini dibuat dengan sebenarnya atas kekuatan " +
		"sumpah jabatan, kemudian ditutup dan ditandatangani di ________ pada hari dan tanggal tersebut di atas."
	bapParafNote     = "Setiap halaman diparaf oleh yang diperiksa."
	bapSummaryMarker = "(ringkasan — periksa rekaman)"
	bapNoItems       = "Tidak ada pertanyaan dan jawaban yang terekam pada ringkasan Tanya Jawab."

	// bapRule is the visible ruling an examiner writes on.
	bapRule = "______________________________"
)

// FormatBAP renders a draft Berita Acara Pemeriksaan as a DOCX document.
// Every timestamp comes from data — the formatter never reads a clock.
func (f *Formatter) FormatBAP(data *port.BAPExportData) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("BAP export data is required")
	}

	doc, err := godocx.NewDocument()
	if err != nil {
		return nil, fmt.Errorf("creating BAP document: %w", err)
	}

	if err := attachBAPCoreProperties(doc, data); err != nil {
		return nil, err
	}

	if err := addBAPHead(doc, data); err != nil {
		return nil, err
	}
	if err := addBAPIdentity(doc); err != nil {
		return nil, err
	}
	addBAPRecordingNote(doc, data)
	if err := addBAPQuestions(doc, data.Items); err != nil {
		return nil, err
	}
	addBAPClosing(doc)

	var buf bytes.Buffer
	if err := doc.Write(&buf); err != nil {
		return nil, fmt.Errorf("writing BAP document: %w", err)
	}
	return buf.Bytes(), nil
}

// Compile-time check.
var _ port.ExaminationRecordFormatter = (*Formatter)(nil)

// addBAPHead writes the PRO JUSTITIA kop, the document number placeholders,
// the title, the pembukaan formula, and the examiner placeholders.
func addBAPHead(doc *docx.RootDoc, data *port.BAPExportData) error {
	kop := doc.AddEmptyParagraph()
	kop.Justification(stypes.JustificationCenter)
	kop.AddText(bapProJustitia).Bold(true)
	doc.AddEmptyParagraph()

	addBAPRuledLines(doc, []string{"Instansi", "Nomor BAP", "Nomor Laporan Polisi"})
	doc.AddEmptyParagraph()

	if _, err := doc.AddHeading(bapDocumentTitle, 1); err != nil {
		return fmt.Errorf("adding BAP title: %w", err)
	}

	doc.AddParagraph(bapOpeningFormula(data.RecordingDate))
	addBAPRuledLines(doc, []string{"Nama", "Pangkat/Golongan", "NRP/NIP", "Jabatan"})
	doc.AddParagraph("selaku penyidik/penyidik pembantu pada instansi tersebut di atas, telah melakukan " +
		"pemeriksaan terhadap seseorang yang identitasnya tersebut di bawah ini.")

	note := doc.AddEmptyParagraph()
	note.AddText("[Periksa dan sesuaikan hari, tanggal, dan waktu pemeriksaan sebelum dokumen disahkan.]").Italic(true)
	doc.AddEmptyParagraph()
	return nil
}

// addBAPIdentity writes the identity placeholder block and the examinee status.
func addBAPIdentity(doc *docx.RootDoc) error {
	if _, err := doc.AddHeading(bapIdentityHead, 1); err != nil {
		return fmt.Errorf("adding BAP identity heading: %w", err)
	}
	addBAPRuledLines(doc, []string{
		"Nama lengkap",
		"Tempat/tanggal lahir",
		"Umur",
		"Jenis kelamin",
		"Kewarganegaraan",
		"Agama",
		"Pekerjaan",
		"Alamat",
		"Pendidikan",
	})
	doc.AddParagraph("Diperiksa sebagai: [ Saksi / Ahli / Tersangka ] (coret yang tidak perlu)")
	doc.AddEmptyParagraph()
	return nil
}

// addBAPRecordingNote records which audio the questions and answers came from.
func addBAPRecordingNote(doc *docx.RootDoc, data *port.BAPExportData) {
	parts := make([]string, 0, 5)
	if name := firstNonEmptyString(data.MediaFilename, data.Title); name != "" {
		parts = append(parts, "Berkas: "+name)
	}
	if data.DurationSeconds > 0 {
		parts = append(parts, "Durasi: "+formatTimestamp(data.DurationSeconds))
	}
	if len(data.Languages) > 0 {
		parts = append(parts, "Bahasa: "+strings.Join(data.Languages, ", "))
	}
	if !data.RecordingDate.IsZero() {
		parts = append(parts, "Tanggal rekaman: "+bapDateText(data.RecordingDate))
	}
	if data.CompletedAt != nil {
		parts = append(parts, "Transkripsi selesai: "+data.CompletedAt.UTC().Format("2006-01-02 15:04")+" UTC")
	}
	if len(parts) == 0 {
		return
	}
	para := doc.AddEmptyParagraph()
	para.AddText("Keterangan rekaman: " + strings.Join(parts, " · ")).Italic(true)
	doc.AddEmptyParagraph()
}

// addBAPQuestions writes the numbered question/answer pairs in transcript order.
func addBAPQuestions(doc *docx.RootDoc, items []port.BAPQuestionAnswer) error {
	if _, err := doc.AddHeading(bapBodyHeading, 1); err != nil {
		return fmt.Errorf("adding BAP body heading: %w", err)
	}
	if len(items) == 0 {
		empty := doc.AddEmptyParagraph()
		empty.AddText(bapNoItems).Italic(true)
		doc.AddEmptyParagraph()
		return nil
	}

	for _, item := range items {
		question := doc.AddEmptyParagraph()
		question.AddText(fmt.Sprintf("%d. %s: ", item.Number, bapSpeakerLabel("Pertanyaan", item.QuestionSpeaker))).Bold(true)
		question.AddText(item.Question)

		answer := doc.AddEmptyParagraph()
		answer.AddText(bapSpeakerLabel("Jawaban", item.AnswerSpeaker) + ": ").Bold(true)
		answer.AddText(item.Answer)
		if item.AnswerSource == port.BAPAnswerSummary {
			answer.AddText(" " + bapSummaryMarker).Italic(true)
		}
		doc.AddEmptyParagraph()
	}
	return nil
}

// addBAPClosing writes the penutup formula, the signature blocks, and the
// paraf note.
func addBAPClosing(doc *docx.RootDoc) {
	doc.AddParagraph(bapClosingFormula)
	doc.AddEmptyParagraph()

	examinee := doc.AddEmptyParagraph()
	examinee.AddText("Yang diperiksa,").Bold(true)
	doc.AddEmptyParagraph()
	doc.AddParagraph(bapRule)
	doc.AddParagraph("(nama lengkap)")
	doc.AddEmptyParagraph()

	investigator := doc.AddEmptyParagraph()
	investigator.AddText("Penyidik/Penyidik Pembantu,").Bold(true)
	doc.AddEmptyParagraph()
	doc.AddParagraph(bapRule)
	doc.AddParagraph("(nama, pangkat, NRP/NIP)")
	doc.AddEmptyParagraph()

	paraf := doc.AddEmptyParagraph()
	paraf.AddText(bapParafNote).Italic(true)
}

// addBAPRuledLines writes one "Label: ______" placeholder per label.
func addBAPRuledLines(doc *docx.RootDoc, labels []string) {
	for _, label := range labels {
		doc.AddParagraph(label + ": " + bapRule)
	}
}

// bapSpeakerLabel appends the speaker attribution when the transcript has one.
func bapSpeakerLabel(label, speaker string) string {
	if speaker == "" {
		return label
	}
	return label + " (" + speaker + ")"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
