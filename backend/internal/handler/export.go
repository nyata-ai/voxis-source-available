package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// maxExportTranscriptBytes bounds exports built in memory in the request
// goroutine. ~8h of speech is well under 1 MB, so this only stops pathological rows.
const maxExportTranscriptBytes = 10 << 20

// ExportHandler handles export HTTP requests.
type ExportHandler struct {
	transSvc     *service.TranscriptionService
	summarySvc   *service.SummaryService
	orgService   *service.OrganizationService
	mediaSvc     *service.MediaService
	formatter    port.DocumentFormatter          // nil-safe: PDF/DOCX return 501 when nil
	bapFormatter port.ExaminationRecordFormatter // nil-safe: template=bap returns 501 when nil
	privilegeSvc *service.PrivilegeService       // optional; nil if privilege not wired
	logger       *slog.Logger
}

// SetPrivilegeService wires the PrivilegeService for direct-access guards.
// Defense-in-depth: even if TranscriptionService.GetByID's guard is misconfigured
// (e.g., privilegeSvc never wired), the handler-level guard blocks export.
func (h *ExportHandler) SetPrivilegeService(p *service.PrivilegeService) {
	h.privilegeSvc = p
}

// NewExportHandler creates a new ExportHandler. mediaSvc supplies the audio
// SHA-256 and forensic recording date the BAP template needs; formatter and
// bapFormatter are both nil-safe (their routes answer 501 when unavailable).
func NewExportHandler(
	transSvc *service.TranscriptionService,
	summarySvc *service.SummaryService,
	orgService *service.OrganizationService,
	mediaSvc *service.MediaService,
	formatter port.DocumentFormatter,
	bapFormatter port.ExaminationRecordFormatter,
	logger *slog.Logger,
) *ExportHandler {
	if transSvc == nil {
		panic("export handler: transcription service cannot be nil")
	}
	if summarySvc == nil {
		panic("export handler: summary service cannot be nil")
	}
	if orgService == nil {
		panic("export handler: organization service cannot be nil")
	}
	if mediaSvc == nil {
		panic("export handler: media service cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ExportHandler{
		transSvc:     transSvc,
		summarySvc:   summarySvc,
		orgService:   orgService,
		mediaSvc:     mediaSvc,
		formatter:    formatter,
		bapFormatter: bapFormatter,
		logger:       logger,
	}
}

// handleError maps domain errors to HTTP status codes.
func (h *ExportHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "not found"})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "access denied"})
	case errors.Is(err, domain.ErrInvalidInput):
		msg := err.Error()
		sentinel := domain.ErrInvalidInput.Error()
		if i := strings.LastIndex(msg, ": "+sentinel); i >= 0 {
			msg = msg[:i]
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_input", "message": msg})
	default:
		h.logger.Error("export error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "internal server error"})
	}
}

// JSON response types with snake_case tags.
type jsonExportResponse struct {
	Title          string                `json:"title"`
	Filename       string                `json:"filename"`
	Languages      []string              `json:"languages"`
	Duration       float64               `json:"duration"`
	SpeakerCount   int                   `json:"speaker_count"`
	WordCount      int                   `json:"word_count"`
	CreatedAt      time.Time             `json:"created_at"`
	CompletedAt    *time.Time            `json:"completed_at"`
	FullTranscript string                `json:"full_transcript,omitempty"`
	Utterances     []jsonExportUtterance `json:"utterances,omitempty"`
	SpeakerMap     map[string]string     `json:"speaker_map,omitempty"`
	Summaries      []jsonExportSummary   `json:"summaries,omitempty"`
}

type jsonExportUtterance struct {
	Speaker int     `json:"speaker"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Text    string  `json:"text"`
	// Confidence is omitted entirely when the provider reports none
	// (Speechmatics Melia). Emitting a zero value would publish a fabricated
	// "the model was certain it was wrong" score. Gladia always supplies one,
	// so Gladia exports are unchanged.
	Confidence *float64 `json:"confidence,omitempty"`
}

type jsonExportSummary struct {
	Type        string     `json:"type"`
	Content     string     `json:"content"`
	WordCount   int        `json:"word_count"`
	CompletedAt *time.Time `json:"completed_at"`
}

// unsafeFilenameChars matches characters not allowed in filenames.
var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeExportFilename strips audio extension, replaces unsafe chars, and appends format extension.
func sanitizeExportFilename(mediaFilename string, format port.ExportFormat) string {
	name := strings.TrimSuffix(mediaFilename, filepath.Ext(mediaFilename))
	if name == "" {
		name = "export"
	}
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	return name + port.ExportFileExtensions[format]
}

// ExportTranscription handles
// GET /api/v1/transcriptions/:id/export?format=pdf|docx|json&template=standard|bap.
func (h *ExportHandler) ExportTranscription(c *gin.Context) {
	format := port.ExportFormat(c.Query("format"))
	if !port.ValidExportFormats[format] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": fmt.Sprintf("unsupported export format: %s", c.Query("format")),
		})
		return
	}

	template, ok := h.resolveExportTemplate(c, format)
	if !ok {
		return
	}

	orgID, _ := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if orgID == "" {
		return
	}

	transcription, err := h.transSvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Defense-in-depth privilege guard at the handler layer (Finding #1 of self-review).
	// TranscriptionService.GetByID also gates this, but this guard makes the
	// invariant local and explicit so misconfigured wiring can never expose
	// privilege transcript via export.
	if h.privilegeSvc != nil && transcription.MediaID != "" {
		if pErr := h.privilegeSvc.IsTranscriptReadAllowed(c.Request.Context(), orgID, transcription.MediaID, port.ReadContextRegular); pErr != nil {
			h.handleError(c, domain.ErrNotFound) // do not leak existence
			return
		}
	}

	if transcription.Status != domain.TranscriptionStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": "transcription is not completed",
		})
		return
	}

	if len(transcription.FullTranscript) > maxExportTranscriptBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": "transcript too large to export",
		})
		return
	}

	if template == port.ExportTemplateBAP {
		h.writeBAPExport(c, orgID, transcription)
		return
	}

	utterances := h.parseUtterances(transcription.ID, transcription.Utterances)
	exportSummaries := h.fetchExportSummaries(c.Request.Context(), orgID, transcription.ID)

	exportData := &port.ExportData{
		Title:          transcription.MediaFilename,
		Filename:       sanitizeExportFilename(transcription.MediaFilename, format),
		Languages:      transcription.Languages,
		Duration:       transcription.DurationSeconds,
		SpeakerCount:   transcription.SpeakerCount,
		WordCount:      transcription.WordCount,
		CreatedAt:      transcription.CreatedAt,
		CompletedAt:    transcription.CompletedAt,
		FullTranscript: transcription.FullTranscript,
		Utterances:     utterances,
		SpeakerMap:     transcription.SpeakerMap,
		Summaries:      exportSummaries,
	}

	h.writeExport(c, format, exportData)
}

// ExportSummary handles GET /api/v1/summaries/:id/export?format=pdf|docx|json.
func (h *ExportHandler) ExportSummary(c *gin.Context) {
	format := port.ExportFormat(c.Query("format"))
	if !port.ValidExportFormats[format] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": fmt.Sprintf("unsupported export format: %s", c.Query("format")),
		})
		return
	}

	orgID, _ := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if orgID == "" {
		return
	}

	summary, err := h.summarySvc.GetByID(c.Request.Context(), orgID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	if summary.Status != domain.SummaryStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": "summary is not completed",
		})
		return
	}

	// Load parent transcription for enrichment metadata (languages, duration, speakers).
	// Timestamps always come from the summary itself — not the parent transcription.
	var languages []string
	var duration float64
	var speakerCount int
	var mediaFilename string

	trans, transErr := h.transSvc.GetByID(c.Request.Context(), orgID, summary.TranscriptionID)
	if transErr != nil {
		h.logger.Warn("failed to load parent transcription for summary export", "error", transErr, "transcription_id", summary.TranscriptionID)
		mediaFilename = summary.TranscriptionMediaFilename
	} else {
		languages = trans.Languages
		duration = trans.DurationSeconds
		speakerCount = trans.SpeakerCount
		mediaFilename = trans.MediaFilename
	}

	if mediaFilename == "" {
		mediaFilename = "summary"
	}

	exportData := &port.ExportData{
		Title:        mediaFilename,
		Filename:     sanitizeExportFilename(mediaFilename, format),
		Languages:    languages,
		Duration:     duration,
		SpeakerCount: speakerCount,
		WordCount:    summary.WordCount,
		CreatedAt:    summary.CreatedAt,
		CompletedAt:  summary.CompletedAt,
		Summaries: []port.ExportSummary{
			{
				Type:        summary.SummaryType,
				Content:     summary.Content,
				WordCount:   summary.WordCount,
				CompletedAt: summary.CompletedAt,
			},
		},
	}

	h.writeExport(c, format, exportData)
}

// parseUtterances unmarshals raw JSON utterances into export types.
func (h *ExportHandler) parseUtterances(transcriptionID string, raw json.RawMessage) []port.ExportUtterance {
	if len(raw) == 0 {
		return nil
	}

	// Confidence decodes into a pointer so "key absent" stays distinguishable
	// from "key present with value 0" — Melia omits it, Gladia always sends it.
	var rawUtterances []struct {
		Speaker    int      `json:"speaker"`
		Start      float64  `json:"start"`
		End        float64  `json:"end"`
		Text       string   `json:"text"`
		Confidence *float64 `json:"confidence"`
		Words      []struct {
			Word       string   `json:"word"`
			Start      float64  `json:"start"`
			End        float64  `json:"end"`
			Confidence *float64 `json:"confidence"`
		} `json:"words"`
	}
	if err := json.Unmarshal(raw, &rawUtterances); err != nil {
		h.logger.Error("failed to parse utterances", "error", err, "transcription_id", transcriptionID)
		return nil
	}

	utterances := make([]port.ExportUtterance, len(rawUtterances))
	for i, ru := range rawUtterances {
		eu := port.ExportUtterance{
			Speaker:    ru.Speaker,
			Start:      ru.Start,
			End:        ru.End,
			Text:       ru.Text,
			Confidence: ru.Confidence,
		}
		if len(ru.Words) > 0 {
			eu.Words = make([]port.ExportWord, len(ru.Words))
			for j, w := range ru.Words {
				eu.Words[j] = port.ExportWord{
					Word:       w.Word,
					Start:      w.Start,
					End:        w.End,
					Confidence: w.Confidence,
				}
			}
		}
		utterances[i] = eu
	}
	return utterances
}

// fetchExportSummaries loads completed summaries with decrypted content for export.
// Note: This uses an N+1 pattern (list + per-summary GetByID) because GetByID
// decrypts summary content. Acceptable since N is bounded by the number of
// summary types (domain.DefaultSummaryTypes).
func (h *ExportHandler) fetchExportSummaries(ctx context.Context, orgID, transcriptionID string) []port.ExportSummary {
	summaryList, listErr := h.summarySvc.ListByTranscription(ctx, orgID, transcriptionID)
	if listErr != nil {
		h.logger.Warn("failed to list summaries for export", "error", listErr, "transcription_id", transcriptionID)
		return nil
	}

	var exportSummaries []port.ExportSummary
	for _, s := range summaryList {
		if s.Status != domain.SummaryStatusCompleted {
			continue
		}
		detail, detailErr := h.summarySvc.GetByID(ctx, orgID, s.ID)
		if detailErr != nil {
			h.logger.Warn("failed to get summary detail for export", "error", detailErr, "summary_id", s.ID)
			continue
		}
		exportSummaries = append(exportSummaries, port.ExportSummary{
			Type:        detail.SummaryType,
			Content:     detail.Content,
			WordCount:   detail.WordCount,
			CompletedAt: detail.CompletedAt,
		})
	}
	return exportSummaries
}

// writeExport writes the export response in the requested format.
func (h *ExportHandler) writeExport(c *gin.Context, format port.ExportFormat, data *port.ExportData) {
	switch format {
	case port.ExportFormatJSON:
		h.writeJSONExport(c, data)
	case port.ExportFormatPDF, port.ExportFormatDOCX:
		h.writeDocumentExport(c, format, data)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": fmt.Sprintf("unsupported export format: %s", string(format)),
		})
	}
}

// writeJSONExport writes the export as a JSON attachment.
func (h *ExportHandler) writeJSONExport(c *gin.Context, data *port.ExportData) {
	resp := jsonExportResponse{
		Title:          data.Title,
		Filename:       data.Filename,
		Languages:      data.Languages,
		Duration:       data.Duration,
		SpeakerCount:   data.SpeakerCount,
		WordCount:      data.WordCount,
		CreatedAt:      data.CreatedAt,
		CompletedAt:    data.CompletedAt,
		FullTranscript: data.FullTranscript,
		SpeakerMap:     data.SpeakerMap,
	}

	if len(data.Utterances) > 0 {
		resp.Utterances = make([]jsonExportUtterance, len(data.Utterances))
		for i, u := range data.Utterances {
			resp.Utterances[i] = jsonExportUtterance{
				Speaker:    u.Speaker,
				Start:      u.Start,
				End:        u.End,
				Text:       u.ResolvedText(),
				Confidence: u.Confidence,
			}
		}
	}

	if len(data.Summaries) > 0 {
		resp.Summaries = make([]jsonExportSummary, len(data.Summaries))
		for i, s := range data.Summaries {
			resp.Summaries[i] = jsonExportSummary{
				Type:        s.Type,
				Content:     s.Content,
				WordCount:   s.WordCount,
				CompletedAt: s.CompletedAt,
			}
		}
	}

	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		h.logger.Error("failed to marshal JSON export", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "internal server error"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", data.Filename))
	c.Data(http.StatusOK, port.ExportMIMETypes[port.ExportFormatJSON], jsonBytes)
}

// writeDocumentExport generates and writes a PDF or DOCX document.
func (h *ExportHandler) writeDocumentExport(c *gin.Context, format port.ExportFormat, data *port.ExportData) {
	if h.formatter == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "PDF/DOCX export not available",
		})
		return
	}

	var (
		docBytes []byte
		err      error
	)

	switch format {
	case port.ExportFormatPDF:
		docBytes, err = h.formatter.FormatPDF(data)
	case port.ExportFormatDOCX:
		docBytes, err = h.formatter.FormatDOCX(data)
	}

	if err != nil {
		h.logger.Error("failed to generate document", "format", format, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "internal server error"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", data.Filename))
	c.Data(http.StatusOK, port.ExportMIMETypes[format], docBytes)
}
