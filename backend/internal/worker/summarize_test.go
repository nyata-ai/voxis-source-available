package worker

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

func TestOSSSummarizeWorkerUsesConfiguredJobTimeout(t *testing.T) {
	configured := NewSummarizeWorker(nil, nil, nil, nil, nil, nil, WithSummaryJobTimeout(time.Hour))
	assert.Equal(t, time.Hour, configured.Timeout(nil))

	defaultWorker := NewSummarizeWorker(nil, nil, nil, nil, nil, nil)
	assert.Equal(t, 10*time.Minute, defaultWorker.Timeout(nil))
}

func TestOSSMergeStructuredSourceChunkResultsPreservesItemBudgets(t *testing.T) {
	source := ossFiveChunkSummarySource()
	cases := []struct {
		name        string
		summaryType string
		payload     func(testing.TB, string, int) string
	}{
		{name: "action items", summaryType: domain.SummaryTypeActionItems, payload: ossActionItemPayload},
		{name: "questions and answers", summaryType: domain.SummaryTypeQAndA, payload: ossQuestionAnswerPayload},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			results := make([]*port.SummaryResult, 0, len(source.Chunks))
			for index := range source.Chunks {
				results = append(results, &port.SummaryResult{Content: testCase.payload(t, source.Segments[index].ID, 6)})
			}

			merged, err := mergeStructuredSourceChunkResults(testCase.summaryType, source, results)
			require.NoError(t, err)
			itemIDs := ossMergedItemIDs(t, testCase.summaryType, merged.StructuredContent)
			assert.Len(t, itemIDs, 30)
			assert.Equal(t, "c1-i1", itemIDs[0])
			assert.Equal(t, "c5-i6", itemIDs[29])
		})
	}
}

func ossFiveChunkSummarySource() *service.SummarySource {
	segmentIDs := []string{"u000001", "u000002", "u000003", "u000004", "u000005"}
	source := &service.SummarySource{Version: service.SummarySourceVersion}
	for index, segmentID := range segmentIDs {
		source.Segments = append(source.Segments, service.SummarySourceSegment{ID: segmentID, Text: "Source evidence."})
		source.Chunks = append(source.Chunks, service.SummarySourceChunk{Index: index, SegmentIDs: []string{segmentID}})
	}
	return source
}

func ossActionItemPayload(t testing.TB, citationID string, count int) string {
	t.Helper()
	items := make([]domain.StructuredSummaryActionItem, 0, count)
	for index := 0; index < count; index++ {
		items = append(items, domain.StructuredSummaryActionItem{
			ID:          fmt.Sprintf("item-%d", index+1),
			Kind:        domain.SummaryActionKindActionItem,
			Text:        fmt.Sprintf("Action %d.", index+1),
			CitationIDs: []string{citationID},
		})
	}
	encoded, err := json.Marshal(domain.StructuredActionItemsSummary{
		SchemaVersion:  domain.StructuredSummarySchemaVersion,
		MatrixLanguage: "en",
		Items:          items,
	})
	require.NoError(t, err)
	return string(encoded)
}

func ossQuestionAnswerPayload(t testing.TB, citationID string, count int) string {
	t.Helper()
	items := make([]domain.StructuredSummaryQuestionAnswer, 0, count)
	for index := 0; index < count; index++ {
		items = append(items, domain.StructuredSummaryQuestionAnswer{
			ID:           fmt.Sprintf("item-%d", index+1),
			Question:     fmt.Sprintf("Question %d?", index+1),
			Answer:       fmt.Sprintf("Answer %d.", index+1),
			AnswerStatus: domain.SummaryAnswerStatusAnswered,
			CitationIDs:  []string{citationID},
		})
	}
	encoded, err := json.Marshal(domain.StructuredQuestionAndAnswerSummary{
		SchemaVersion:  domain.StructuredSummarySchemaVersion,
		MatrixLanguage: "en",
		Items:          items,
	})
	require.NoError(t, err)
	return string(encoded)
}

func ossMergedItemIDs(t testing.TB, summaryType string, raw []byte) []string {
	t.Helper()
	parsed, err := service.ParseStructuredSummary(summaryType, raw)
	require.NoError(t, err)
	if parsed.ActionItems != nil {
		itemIDs := make([]string, 0, len(parsed.ActionItems.Items))
		for _, item := range parsed.ActionItems.Items {
			itemIDs = append(itemIDs, item.ID)
		}
		return itemIDs
	}
	require.NotNil(t, parsed.QAndA)
	itemIDs := make([]string, 0, len(parsed.QAndA.Items))
	for _, item := range parsed.QAndA.Items {
		itemIDs = append(itemIDs, item.ID)
	}
	return itemIDs
}
