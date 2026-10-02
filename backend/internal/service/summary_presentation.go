package service

import (
	"encoding/json"

	"github.com/voxis/backend/internal/domain"
)

// SummaryCitation is the compact, source-addressable citation returned with a
// structured summary. Optional source metadata remains nil when unavailable.
type SummaryCitation struct {
	ID           string
	Speaker      *string
	StartSeconds *float64
	EndSeconds   *float64
}

// SummaryGenerationMetadata records how a structured summary was generated.
type SummaryGenerationMetadata struct {
	PromptVersion           string
	Model                   string
	EndpointLocation        string
	SourceVersion           string
	SourceHash              string
	StructuredSchemaVersion string
	DegradationCodes        []string
}

// SummaryStructuredPresentation holds additive fields for API and MCP
// responses. Canonical prose remains on domain.Summary.Content.
type SummaryStructuredPresentation struct {
	StructuredContent  json.RawMessage
	Citations          []SummaryCitation
	GenerationMetadata *SummaryGenerationMetadata
}

// BuildSummaryStructuredPresentation prepares structured response fields for
// one summary. Citations are exposed only when the current transcription
// rebuilds to the exact source version and hash recorded on the summary.
func BuildSummaryStructuredPresentation(summary *domain.Summary, transcription *domain.Transcription) SummaryStructuredPresentation {
	presentation := SummaryStructuredPresentation{
		GenerationMetadata: summaryGenerationMetadata(summary),
	}
	structured, ok := parsePresentationStructuredContent(summary)
	if !ok {
		return presentation
	}
	presentation.StructuredContent = append(json.RawMessage(nil), summary.StructuredContent...)

	source, ok := presentationSource(summary, transcription)
	if !ok {
		return presentation
	}
	presentation.Citations = presentationCitations(structured, source)
	return presentation
}

func summaryGenerationMetadata(summary *domain.Summary) *SummaryGenerationMetadata {
	metadata := &SummaryGenerationMetadata{
		PromptVersion:           summary.PromptVersion,
		Model:                   summary.Model,
		EndpointLocation:        summary.EndpointLocation,
		SourceVersion:           summary.SourceVersion,
		SourceHash:              summary.SourceHash,
		StructuredSchemaVersion: summary.StructuredSchemaVersion,
		DegradationCodes:        append([]string{}, summary.DegradationCodes...),
	}
	if metadata.PromptVersion == "" && metadata.Model == "" && metadata.EndpointLocation == "" &&
		metadata.SourceVersion == "" && metadata.SourceHash == "" && metadata.StructuredSchemaVersion == "" &&
		len(metadata.DegradationCodes) == 0 {
		return nil
	}
	return metadata
}

func parsePresentationStructuredContent(summary *domain.Summary) (*domain.StructuredSummary, bool) {
	if len(summary.StructuredContent) == 0 || !json.Valid(summary.StructuredContent) {
		return nil, false
	}
	structured, err := ParseStructuredSummary(summary.SummaryType, summary.StructuredContent)
	if err != nil {
		return nil, false
	}
	return structured, true
}

func presentationSource(summary *domain.Summary, transcription *domain.Transcription) (*SummarySource, bool) {
	if transcription == nil || summary.SourceVersion != SummarySourceVersion || summary.SourceHash == "" {
		return nil, false
	}
	content := &domain.TranscriptionContent{
		FullTranscript: transcription.FullTranscript,
		Utterances:     transcription.Utterances,
		SpeakerMap:     transcription.SpeakerMap,
	}
	source, err := BuildSummarySource(content, presentationSourceLanguage(transcription.Languages))
	if err != nil || source.Version != summary.SourceVersion || source.Hash != summary.SourceHash {
		return nil, false
	}
	return source, true
}

func presentationSourceLanguage(languages []string) string {
	if len(languages) == 1 && languages[0] != "auto" {
		return languages[0]
	}
	return ""
}

func presentationCitations(structured *domain.StructuredSummary, source *SummarySource) []SummaryCitation {
	byID := make(map[string]SummarySourceSegment, len(source.Segments))
	for _, segment := range source.Segments {
		byID[segment.ID] = segment
	}
	ids := structuredCitationIDs(structured)
	citations := make([]SummaryCitation, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		segment, exists := byID[id]
		if !exists {
			continue
		}
		seen[id] = struct{}{}
		citations = append(citations, SummaryCitation{
			ID:           segment.ID,
			Speaker:      segment.Speaker,
			StartSeconds: segment.StartSeconds,
			EndSeconds:   segment.EndSeconds,
		})
	}
	return citations
}

func structuredCitationIDs(summary *domain.StructuredSummary) []string {
	if summary == nil {
		return nil
	}
	ids := make([]string, 0)
	switch summary.SummaryType {
	case domain.SummaryTypeGeneral:
		for _, item := range summary.General.Paragraphs {
			ids = append(ids, item.CitationIDs...)
		}
	case domain.SummaryTypeKeyPoints:
		for _, item := range summary.KeyPoints.Items {
			ids = append(ids, item.CitationIDs...)
		}
	case domain.SummaryTypeActionItems:
		for _, item := range summary.ActionItems.Items {
			ids = append(ids, item.CitationIDs...)
		}
	case domain.SummaryTypeQAndA:
		for _, item := range summary.QAndA.Items {
			ids = append(ids, item.CitationIDs...)
		}
	}
	return ids
}
