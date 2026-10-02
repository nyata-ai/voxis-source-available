package export

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/gomutex/godocx/docx"
	"github.com/voxis/backend/internal/port"
)

// bapCorePropsPath is the OOXML core-properties part. godocx's default template
// already ships it, already relationship-linked from _rels/.rels and already
// declared in [Content_Types].xml, so the formatter only overwrites its bytes —
// no part registration is needed.
const bapCorePropsPath = "docProps/core.xml"

// bapGenerator names the tool in the document metadata.
const bapGenerator = "Voxis"

// bapMetadataTitle labels the document in file properties. It is metadata only:
// nothing here is rendered on a printed page.
const bapMetadataTitle = "Draf Berita Acara Pemeriksaan"

// bapIndonesianDays is indexed by time.Weekday (Sunday = 0).
var bapIndonesianDays = [7]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// bapIndonesianMonths is indexed by time.Month - 1.
var bapIndonesianMonths = [12]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// bapOpeningFormula pre-fills the pembukaan date parts when a date is known.
// The clock time always stays blank: the recording's timestamp is not the
// examination-room local time, and guessing it would be a factual claim.
func bapOpeningFormula(recordingDate time.Time) string {
	if recordingDate.IsZero() {
		return "Pada hari ini, ____ tanggal ____ bulan ____ tahun ____, sekira pukul ____ WIB, saya:"
	}
	return fmt.Sprintf(
		"Pada hari ini, %s tanggal %d bulan %s tahun %d, sekira pukul ____ WIB, saya:",
		bapIndonesianDays[int(recordingDate.Weekday())],
		recordingDate.Day(),
		bapIndonesianMonths[int(recordingDate.Month())-1],
		recordingDate.Year(),
	)
}

// bapDateText renders a date as "30 Agustus 2026".
func bapDateText(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d %s %d", value.Day(), bapIndonesianMonths[int(value.Month())-1], value.Year())
}

// bapProvenanceDescription records what produced the draft, from which audio,
// and when. Unavailable parts are omitted rather than filled with a placeholder,
// so the metadata never asserts something it does not know.
func bapProvenanceDescription(data *port.BAPExportData) string {
	parts := make([]string, 0, 4)
	parts = append(parts, "Dihasilkan oleh "+bapGenerator)
	if id := strings.TrimSpace(data.TranscriptionID); id != "" {
		parts = append(parts, "ID transkripsi: "+id)
	}
	if hash := strings.TrimSpace(data.AudioSHA256); hash != "" {
		parts = append(parts, "SHA-256 audio: "+hash)
	}
	if !data.GeneratedAt.IsZero() {
		parts = append(parts, "Dibuat: "+data.GeneratedAt.UTC().Format("2006-01-02 15:04:05")+" UTC")
	}
	return strings.Join(parts, " | ")
}

// attachBAPCoreProperties overwrites docProps/core.xml so the draft carries its
// provenance in the file's document properties. This is deliberately invisible:
// the owner's review removed every on-page marker, and the metadata is what is
// left to tie a generated file back to its source recording.
func attachBAPCoreProperties(doc *docx.RootDoc, data *port.BAPExportData) error {
	content, err := bapCorePropsXML(data)
	if err != nil {
		return err
	}
	doc.FileMap.Store(bapCorePropsPath, content)
	return nil
}

// bapCorePropsXML builds the core-properties part. Every timestamp comes from
// data.GeneratedAt — the formatter never reads a clock, so output stays
// byte-for-byte reproducible.
func bapCorePropsXML(data *port.BAPExportData) ([]byte, error) {
	stamp := ""
	if !data.GeneratedAt.IsZero() {
		stamp = data.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	buf.WriteString(`<cp:coreProperties` +
		` xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:dcterms="http://purl.org/dc/terms/"` +
		` xmlns:dcmitype="http://purl.org/dc/dcmitype/"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`)

	// Element order follows the CT_CoreProperties sequence.
	if err := writeBAPPropElement(&buf, "dc:title", bapMetadataTitle); err != nil {
		return nil, err
	}
	if err := writeBAPPropElement(&buf, "dc:creator", bapGenerator); err != nil {
		return nil, err
	}
	if err := writeBAPPropElement(&buf, "dc:description", bapProvenanceDescription(data)); err != nil {
		return nil, err
	}
	if err := writeBAPPropElement(&buf, "dc:identifier", strings.TrimSpace(data.TranscriptionID)); err != nil {
		return nil, err
	}
	if err := writeBAPPropElement(&buf, "cp:lastModifiedBy", bapGenerator); err != nil {
		return nil, err
	}
	if stamp != "" {
		buf.WriteString(`<dcterms:created xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:created>`)
		buf.WriteString(`<dcterms:modified xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:modified>`)
	}
	buf.WriteString(`</cp:coreProperties>`)
	return buf.Bytes(), nil
}

// writeBAPPropElement writes one escaped property element, skipping empties.
func writeBAPPropElement(buf *bytes.Buffer, tag, value string) error {
	if value == "" {
		return nil
	}
	buf.WriteString("<" + tag + ">")
	if err := xml.EscapeText(buf, []byte(value)); err != nil {
		return fmt.Errorf("BAP: escaping %s: %w", tag, err)
	}
	buf.WriteString("</" + tag + ">")
	return nil
}
