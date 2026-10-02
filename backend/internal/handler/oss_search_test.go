package handler

import (
	"fmt"
	"testing"

	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

func TestOSSSearchResponseDoesNotDropTranscriptContinuation(t *testing.T) {
	metadata := make([]port.SearchAllResult, 50)
	for index := range metadata {
		metadata[index] = port.SearchAllResult{ID: fmt.Sprintf("metadata-%d", index), EntityType: "media"}
	}
	matches := make([]port.TranscriptSegmentMatch, 25)
	for index := range matches {
		matches[index] = port.TranscriptSegmentMatch{MediaID: fmt.Sprintf("transcript-%d", index), TranscriptionID: fmt.Sprintf("transcription-%d", index), DisplayName: "meeting.wav"}
	}

	response := ossSearchResponseFromPages(
		&port.SearchAllPage{Items: metadata, Total: 50, SearchMode: "lexical"},
		&service.TranscriptSearchPage{Matches: matches, NextCursor: "next-cursor", Complete: false},
		"market update",
	)
	if len(response.Items) != 50 || response.Total != 50 {
		t.Fatalf("metadata response = %#v, want all 50 metadata results", response)
	}
	if len(response.TranscriptItems) != 25 || response.NextTranscriptCursor != "next-cursor" || response.TranscriptComplete {
		t.Fatalf("transcript response = %#v, want independent continuation", response)
	}
}
