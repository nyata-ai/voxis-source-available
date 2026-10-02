package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type fakeOSSSearchBackend struct {
	page             *port.SearchAllPage
	transcriptPage   *service.TranscriptSearchPage
	orgID            string
	transcriptCursor string
}

func (f *fakeOSSSearchBackend) SearchAllPage(_ context.Context, orgID, _ string, _, _ int) (*port.SearchAllPage, error) {
	f.orgID = orgID
	return f.page, nil
}

func (f *fakeOSSSearchBackend) SearchTranscriptMediaPage(_ context.Context, orgID, _, cursor string) (*service.TranscriptSearchPage, error) {
	f.orgID = orgID
	f.transcriptCursor = cursor
	return f.transcriptPage, nil
}

func TestOSSSearchKeepsMetadataAndTranscriptPagesSeparate(t *testing.T) {
	metadata := make([]port.SearchAllResult, 50)
	for index := range metadata {
		metadata[index] = port.SearchAllResult{ID: fmt.Sprintf("metadata-%d", index), EntityType: "media"}
	}
	matches := make([]port.TranscriptSegmentMatch, 25)
	for index := range matches {
		matches[index] = port.TranscriptSegmentMatch{MediaID: fmt.Sprintf("transcript-%d", index), TranscriptionID: fmt.Sprintf("transcription-%d", index)}
	}
	backend := &fakeOSSSearchBackend{
		page:           &port.SearchAllPage{Items: metadata, Total: 50, SearchMode: "lexical"},
		transcriptPage: &service.TranscriptSearchPage{Matches: matches, NextCursor: "next-cursor", Complete: false},
	}

	metadataPage, transcriptPage, err := newOSSSearchService(backend).SearchOSSPages(context.Background(), "org-1", "market update", 50, 0, "cursor-1")
	if err != nil {
		t.Fatalf("SearchOSSPages() error = %v", err)
	}
	if backend.orgID != "org-1" || backend.transcriptCursor != "cursor-1" {
		t.Fatalf("search scope = org %q cursor %q", backend.orgID, backend.transcriptCursor)
	}
	if len(metadataPage.Items) != 50 || metadataPage.Total != 50 {
		t.Fatalf("metadata page = %#v, want all 50 metadata results", metadataPage)
	}
	if len(transcriptPage.Matches) != 25 || transcriptPage.NextCursor != "next-cursor" || transcriptPage.Complete {
		t.Fatalf("transcript page = %#v, want independent continuation", transcriptPage)
	}
}
