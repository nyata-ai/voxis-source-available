package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

const (
	bapMediaOnlyMessage = "Berita Acara Pemeriksaan export requires an uploaded or recorded audio file; " +
		"URL-source transcriptions are not eligible"
	bapSummaryRequiredMessage = "generate a Questions & Answers summary for this transcription first"
	bapRegenerateMessage      = "the Questions & Answers summary no longer matches the transcript " +
		"(speakers were renamed after it was generated); regenerate the Q&A summary and try again"
	bapFilenameSuffix = "-BAP-draf.docx"
)

// bapRecordingDateLayouts are the timestamp shapes ffprobe tags carry.
var bapRecordingDateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// resolveExportTemplate validates the optional `template` query parameter.
// Returns false when it has already written an error response.
func (h *ExportHandler) resolveExportTemplate(c *gin.Context, format port.ExportFormat) (port.ExportTemplate, bool) {
	raw := strings.TrimSpace(c.Query("template"))
	if raw == "" {
		return port.ExportTemplateStandard, true
	}

	template := port.ExportTemplate(raw)
	if !port.ValidExportTemplates[template] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": fmt.Sprintf("unsupported export template: %s", raw),
		})
		return "", false
	}
	if template == port.ExportTemplateBAP && format != port.ExportFormatDOCX {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"message": "template=bap is available for format=docx only",
		})
		return "", false
	}
	return template, true
}

// writeBAPExport renders the draft Berita Acara Pemeriksaan for a completed,
// media-source transcription. Callers have already run the privilege guard and
// the completion check.
func (h *ExportHandler) writeBAPExport(c *gin.Context, orgID string, trans *domain.Transcription) {
	if h.bapFormatter == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "Berita Acara Pemeriksaan export not available",
		})
		return
	}
	if trans.MediaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_input", "message": bapMediaOnlyMessage})
		return
	}

	ctx := c.Request.Context()
	media, err := h.mediaSvc.GetByID(ctx, orgID, trans.MediaID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	items, err := h.summarySvc.ExaminationQuestionAnswers(ctx, orgID, trans)
	if err != nil {
		h.handleBAPError(c, err)
		return
	}

	docBytes, err := h.bapFormatter.FormatBAP(h.buildBAPExportData(ctx, orgID, trans, media, items))
	if err != nil {
		h.logger.Error("failed to generate BAP document", "error", err, "transcription_id", trans.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "internal server error"})
		return
	}

	filename := sanitizeBAPFilename(trans.MediaFilename, media.Filename)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, port.ExportMIMETypes[port.ExportFormatDOCX], docBytes)
}

// buildBAPExportData assembles the render input. Every timestamp is resolved
// here so the formatter never reads a clock.
func (h *ExportHandler) buildBAPExportData(
	ctx context.Context,
	orgID string,
	trans *domain.Transcription,
	media *domain.Media,
	items []service.ExaminationQuestionAnswer,
) *port.BAPExportData {
	return &port.BAPExportData{
		Title:           firstNonEmptyExportValue(media.Title, media.Filename, trans.MediaFilename),
		MediaFilename:   firstNonEmptyExportValue(media.Filename, trans.MediaFilename),
		AudioSHA256:     media.FileHash,
		RecordingDate:   h.resolveBAPRecordingDate(ctx, orgID, media, trans.CreatedAt),
		Languages:       trans.Languages,
		DurationSeconds: trans.DurationSeconds,
		TranscriptionID: trans.ID,
		CreatedAt:       trans.CreatedAt,
		CompletedAt:     trans.CompletedAt,
		GeneratedAt:     time.Now().UTC(),
		Items:           buildBAPItems(items),
	}
}

// resolveBAPRecordingDate prefers the forensic recording date carried in the
// audio container tags and falls back to the transcription's creation time.
func (h *ExportHandler) resolveBAPRecordingDate(
	ctx context.Context,
	orgID string,
	media *domain.Media,
	fallback time.Time,
) time.Time {
	analysis, err := h.mediaSvc.GetAudioAnalysis(ctx, orgID, media)
	if err != nil {
		h.logger.Warn("failed to load audio analysis for BAP export", "error", err, "media_id", media.ID)
		return fallback
	}
	if analysis == nil || strings.TrimSpace(analysis.RecordingDate) == "" {
		return fallback
	}
	for _, layout := range bapRecordingDateLayouts {
		parsed, parseErr := time.Parse(layout, strings.TrimSpace(analysis.RecordingDate))
		if parseErr == nil {
			return parsed
		}
	}
	// The tag value itself is uploader-controlled content; log the fact, not the value.
	h.logger.Warn("unparsable forensic recording date for BAP export", "media_id", media.ID)
	return fallback
}

// handleBAPError maps the BAP precondition errors to 409 and defers the rest
// to the shared export error mapping.
func (h *ExportHandler) handleBAPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrExaminationSummaryMissing),
		errors.Is(err, domain.ErrExaminationSummaryUnstructured):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": bapSummaryRequiredMessage})
	case errors.Is(err, domain.ErrExaminationSourceMismatch):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": bapRegenerateMessage})
	default:
		h.handleError(c, err)
	}
}

// buildBAPItems numbers the items and applies the fidelity rule to each answer.
func buildBAPItems(items []service.ExaminationQuestionAnswer) []port.BAPQuestionAnswer {
	rendered := make([]port.BAPQuestionAnswer, 0, len(items))
	for i, item := range items {
		answer, answerSource := resolveBAPAnswer(item)
		rendered = append(rendered, port.BAPQuestionAnswer{
			Number:          i + 1,
			Question:        item.Question,
			QuestionSpeaker: item.QuestionSpeaker,
			Answer:          answer,
			AnswerSpeaker:   item.AnswerSpeaker,
			AnswerSource:    answerSource,
		})
	}
	return rendered
}

// resolveBAPAnswer implements the KUHAP Pasal 117(2) fidelity rule: prefer the
// model's exact quote, then the cited source utterances, and only then the
// summarized answer — which the renderer marks as a summary.
func resolveBAPAnswer(item service.ExaminationQuestionAnswer) (string, port.BAPAnswerSource) {
	if item.AnswerExactQuote != "" {
		return item.AnswerExactQuote, port.BAPAnswerExactQuote
	}
	if cited := citedAnswerText(item); cited != "" {
		return cited, port.BAPAnswerCitation
	}
	return item.Answer, port.BAPAnswerSummary
}

// citedAnswerText joins the cited utterances. When the item names an answering
// speaker, that speaker's utterances are preferred so a cited question turn
// does not end up inside the answer.
func citedAnswerText(item service.ExaminationQuestionAnswer) string {
	if item.AnswerSpeaker != "" {
		if text := joinCitedSegments(item.CitedSegments, item.AnswerSpeaker); text != "" {
			return text
		}
	}
	return joinCitedSegments(item.CitedSegments, "")
}

func joinCitedSegments(segments []service.ExaminationCitedSegment, speaker string) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.Text == "" {
			continue
		}
		if speaker != "" && segment.Speaker != speaker {
			continue
		}
		parts = append(parts, segment.Text)
	}
	return strings.Join(parts, " ")
}

// sanitizeBAPFilename mirrors sanitizeExportFilename and appends the BAP suffix.
func sanitizeBAPFilename(candidates ...string) string {
	name := firstNonEmptyExportValue(candidates...)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		name = "export"
	}
	return unsafeFilenameChars.ReplaceAllString(name, "_") + bapFilenameSuffix
}

func firstNonEmptyExportValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
