package main

import (
	"context"

	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type ossSearchBackend interface {
	SearchAllPage(context.Context, string, string, int, int) (*port.SearchAllPage, error)
	SearchTranscriptMediaPage(context.Context, string, string, string) (*service.TranscriptSearchPage, error)
}

// ossSearchService keeps metadata paging independent from the bounded,
// organization-scoped decrypted transcript search. It never creates a
// plaintext index or fabricates a combined total.
type ossSearchService struct {
	backend ossSearchBackend
}

func newOSSSearchService(backend ossSearchBackend) *ossSearchService {
	return &ossSearchService{backend: backend}
}

func (s *ossSearchService) SearchOSSPages(
	ctx context.Context,
	orgID, search string,
	limit, offset int,
	transcriptCursor string,
) (*port.SearchAllPage, *service.TranscriptSearchPage, error) {
	metadata, err := s.backend.SearchAllPage(ctx, orgID, search, limit, offset)
	if err != nil {
		return nil, nil, err
	}
	transcripts, err := s.backend.SearchTranscriptMediaPage(ctx, orgID, search, transcriptCursor)
	if err != nil {
		return nil, nil, err
	}
	return metadata, transcripts, nil
}
