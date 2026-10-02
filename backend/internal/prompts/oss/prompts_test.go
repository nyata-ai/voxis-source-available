package oss

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

func TestGrammarSafeSchemaBounds(t *testing.T) {
	schemas := map[string]string{
		"key points":          keyPointsSummarySchema,
		"action items":        actionItemsSummarySchema,
		"question and answer": questionAnswerSummarySchema,
		"extraction":          extractionSchema,
		"speaker":             speakerSchema,
	}

	for name, schema := range schemas {
		if strings.Contains(schema, `"maxLength":2000`) {
			t.Errorf("%s uses llama.cpp's unsupported maxLength 2000", name)
		}
		if !strings.Contains(schema, `"maxLength":1999`) {
			t.Errorf("%s does not retain the bounded 1999-character field", name)
		}
	}
}

func TestGeneralSummarySchemaLimitsEachGeneratedChunk(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			MaxItems int `json:"maxItems"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(generalSummarySchema), &schema); err != nil {
		t.Fatalf("parse general summary schema: %v", err)
	}
	if got := schema.Properties["paragraphs"].MaxItems; got != 5 {
		t.Fatalf("general paragraph maxItems = %d, want 5 per source chunk", got)
	}
}

func TestItemSchemasShareGlobalBudgetAcrossSourceChunks(t *testing.T) {
	tests := []struct {
		summaryType string
		total       int
	}{
		{domain.SummaryTypeKeyPoints, 20},
		{domain.SummaryTypeActionItems, 30},
		{domain.SummaryTypeQAndA, 30},
	}

	for _, test := range tests {
		t.Run(test.summaryType, func(t *testing.T) {
			for chunkCount := 1; chunkCount <= maxSummarySourceChunks; chunkCount++ {
				standardTotal := 0
				anchoredTotal := 0
				for chunkIndex := 0; chunkIndex < chunkCount; chunkIndex++ {
					standard, err := Summary(port.SummaryRequest{
						SummaryType: test.summaryType, SourceChunkIndex: chunkIndex, SourceChunkCount: chunkCount,
					})
					if err != nil {
						t.Fatalf("Summary(chunk=%d/%d) error = %v", chunkIndex, chunkCount, err)
					}
					anchored, err := AnchoredSummary(port.AnchoredSummaryRequest{
						SummaryType: test.summaryType, SourceChunkIndex: chunkIndex, SourceChunkCount: chunkCount,
					})
					if err != nil {
						t.Fatalf("AnchoredSummary(chunk=%d/%d) error = %v", chunkIndex, chunkCount, err)
					}
					standardTotal += schemaItemsLimit(t, standard.Schema)
					anchoredTotal += schemaItemsLimit(t, anchored.Schema)
				}
				if standardTotal != test.total || anchoredTotal != test.total {
					t.Fatalf("chunk count %d budgets = standard %d, anchored %d; want %d", chunkCount, standardTotal, anchoredTotal, test.total)
				}
			}
		})
	}
	if _, err := Summary(port.SummaryRequest{
		SummaryType: domain.SummaryTypeKeyPoints, SourceChunkIndex: maxSummarySourceChunks, SourceChunkCount: maxSummarySourceChunks + 1,
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Summary() error = %v, want invalid chunk metadata", err)
	}
}

func TestSummaryChunkMetadataNormalizesOrRejectsBounds(t *testing.T) {
	zeroValue, err := Summary(port.SummaryRequest{SummaryType: domain.SummaryTypeKeyPoints})
	if err != nil {
		t.Fatalf("Summary() zero-value chunk metadata error = %v", err)
	}
	if got := schemaItemsLimit(t, zeroValue.Schema); got != 20 {
		t.Fatalf("Summary() zero-value chunk maxItems = %d, want 20", got)
	}

	invalid := []struct {
		name       string
		chunkIndex int
		chunkCount int
	}{
		{name: "negative count", chunkIndex: 0, chunkCount: -1},
		{name: "negative index", chunkIndex: -1, chunkCount: 1},
		{name: "index outside count", chunkIndex: 1, chunkCount: 1},
		{name: "too many chunks", chunkIndex: maxSummarySourceChunks, chunkCount: maxSummarySourceChunks + 1},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			_, standardErr := Summary(port.SummaryRequest{
				SummaryType: domain.SummaryTypeGeneral, SourceChunkIndex: test.chunkIndex, SourceChunkCount: test.chunkCount,
			})
			_, anchoredErr := AnchoredSummary(port.AnchoredSummaryRequest{
				SummaryType: domain.SummaryTypeGeneral, SourceChunkIndex: test.chunkIndex, SourceChunkCount: test.chunkCount,
			})
			for _, err := range []error{standardErr, anchoredErr} {
				if !errors.Is(err, domain.ErrInvalidInput) || !strings.Contains(err.Error(), "invalid source chunk metadata") {
					t.Fatalf("chunk metadata error = %v, want explicit invalid-input error", err)
				}
			}
		})
	}
}

func schemaItemsLimit(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			MaxItems int `json:"maxItems"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	return schema.Properties["items"].MaxItems
}

func TestActionItemPromptsRequireExplicitSourceSupport(t *testing.T) {
	summary, err := Summary(port.SummaryRequest{
		SummaryType:    domain.SummaryTypeActionItems,
		SummaryProfile: domain.SummaryProfileInvestmentAnalysis,
		Language:       "de",
	})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	anchored, err := AnchoredSummary(port.AnchoredSummaryRequest{
		SummaryType:    domain.SummaryTypeActionItems,
		SummaryProfile: domain.SummaryProfileInvestmentAnalysis,
		Language:       "de",
	})
	if err != nil {
		t.Fatalf("AnchoredSummary() error = %v", err)
	}
	extraction := Extraction(port.SummaryExtractionRequest{
		SummaryProfile: domain.SummaryProfileInvestmentAnalysis,
		Language:       "de",
	})
	if !strings.Contains(extraction.System, dateGranularityInstruction) {
		t.Error("extraction prompt does not preserve source date granularity")
	}

	for name, system := range map[string]string{
		"summary":  summary.System,
		"anchored": anchored.System,
	} {
		for _, instruction := range []string{
			dateGranularityInstruction,
			"Create an open issue only when the source explicitly calls it open",
			"Mark an item as a decision only when the source explicitly records a choice, approval, or agreement",
			"Set an owner only when the source assigns or accepts responsibility; otherwise use null",
			"The action array need not include every kind; omit unsupported categories.",
		} {
			if !strings.Contains(system, instruction) {
				t.Errorf("%s prompt does not contain %q", name, instruction)
			}
		}
	}
	if !strings.Contains(summary.System, "diligence questions that the source explicitly leaves open") {
		t.Error("investment profile prompt does not limit diligence questions to source-supported issues")
	}
}

func TestQuestionAnswerAndSpeakerPromptsRequireSourceGrounding(t *testing.T) {
	standard, err := Summary(port.SummaryRequest{SummaryType: domain.SummaryTypeQAndA})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	anchored, err := AnchoredSummary(port.AnchoredSummaryRequest{SummaryType: domain.SummaryTypeQAndA})
	if err != nil {
		t.Fatalf("AnchoredSummary() error = %v", err)
	}
	for name, system := range map[string]string{
		"summary":  standard.System,
		"anchored": anchored.System,
	} {
		for _, instruction := range []string{
			summarySpeakerGroundingInstruction,
			"When every cited record has speaker_name null, return JSON null even if record text contains a person's name.",
			"Include a question only when a source speaker actually asks it.",
			"Each Q&A item needs a cited source record that directly asks that question.",
			"When a source reports a question indirectly, such as 'Ada asked whether X', write the actual question X.",
			"Do not ask whether Ada asked it.",
			"Do not create an item from a record that merely gives information, makes a commitment, records a decision, or gives an answer.",
			"Do not derive or infer a question from a statement, commitment, decision, answer, or other non-question.",
			"Do not create owner, status, or meta questions about the source.",
			"Use conflicting only when cited records give materially inconsistent answers",
		} {
			if !strings.Contains(system, instruction) {
				t.Errorf("%s prompt does not contain %q", name, instruction)
			}
		}
	}

	speakers := Speakers(port.SpeakerSuggestionRequest{}).System
	for _, instruction := range []string{
		"A name in a speaker's address names the addressee, not the speaker who says it.",
		"Name identifies another person and gives no name evidence for Speaker N.",
		"Never infer the speaking person's name from a name they use to address someone else.",
		"If no independent source supports the speaking person's name, omit that speaker.",
	} {
		if !strings.Contains(speakers, instruction) {
			t.Errorf("speaker prompt does not contain %q", instruction)
		}
	}
}

func TestSummarySchemasNullSpeakerFieldsWithoutSourceNames(t *testing.T) {
	withoutNames := "{\"v\":\"summary-source-v1\",\"lang\":\"en\"}\n[\"u000001\",null,null,null,null,\"Ada asked whether North Harbor should close.\"]"
	withNames := "{\"v\":\"summary-source-v1\",\"lang\":\"en\"}\n[\"u000001\",\"speaker-ada\",\"Ada\",null,null,\"Should North Harbor close?\"]"
	malformed := "{\"v\":\"summary-source-v1\",\"lang\":\"en\"}\nnot-json"
	builders := []struct {
		name  string
		build func(string, string) (Parts, error)
	}{
		{
			name: "standard",
			build: func(summaryType, source string) (Parts, error) {
				return Summary(port.SummaryRequest{SummaryType: summaryType, Text: source})
			},
		},
		{
			name: "anchored",
			build: func(summaryType, source string) (Parts, error) {
				return AnchoredSummary(port.AnchoredSummaryRequest{SummaryType: summaryType, Transcript: source})
			},
		},
	}
	inputs := []struct {
		name       string
		source     string
		nullFields bool
	}{
		{name: "unnamed", source: withoutNames, nullFields: true},
		{name: "named", source: withNames},
		{name: "empty fallback", source: ""},
		{name: "malformed fallback", source: malformed},
	}

	for _, summaryType := range []string{
		domain.SummaryTypeGeneral,
		domain.SummaryTypeKeyPoints,
		domain.SummaryTypeActionItems,
		domain.SummaryTypeQAndA,
	} {
		t.Run(summaryType, func(t *testing.T) {
			for _, builder := range builders {
				t.Run(builder.name, func(t *testing.T) {
					for _, input := range inputs {
						t.Run(input.name, func(t *testing.T) {
							parts, err := builder.build(summaryType, input.source)
							if err != nil {
								t.Fatalf("prompt build error = %v", err)
							}
							assertSpeakerSchemaFields(t, parts.Schema, summarySpeakerFields(summaryType), input.nullFields)
						})
					}
				})
			}
		})
	}
}

func assertSpeakerSchemaFields(t *testing.T, schema json.RawMessage, fields []string, nullFields bool) {
	t.Helper()
	for _, field := range fields {
		if nullFields {
			if !strings.Contains(string(schema), `"`+field+`":{"type":"null"}`) {
				t.Errorf("schema does not require null %s", field)
			}
			continue
		}
		if !strings.Contains(string(schema), `"`+field+`":{"type":["string","null"]`) {
			t.Errorf("schema does not allow a source-backed %s", field)
		}
	}
}

func summarySpeakerFields(summaryType string) []string {
	if summaryType == domain.SummaryTypeQAndA {
		return []string{"question_speaker", "answer_speaker"}
	}
	return []string{"speaker"}
}
