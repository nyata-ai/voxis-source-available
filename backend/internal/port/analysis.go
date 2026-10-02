package port

import "context"

// TranscriptEvidence is a bounded source segment sent to an answer model.
type TranscriptEvidence struct {
	ID              string
	TranscriptionID string
	MediaID         string
	SourceType      string
	DisplayName     string
	Speaker         string
	StartSeconds    float64
	EndSeconds      float64
	Text            string
}

// TranscriptAnswer is the model output for grounded transcript Q&A.
type TranscriptAnswer struct {
	Answer      string
	CitationIDs []string
}

// TranscriptAnswerer generates an answer from provided transcript evidence.
type TranscriptAnswerer interface {
	AnswerTranscriptQuestion(ctx context.Context, question string, evidence []TranscriptEvidence) (*TranscriptAnswer, error)
}

// AskTranscriptRequest configures transcript-grounded Q&A.
type AskTranscriptRequest struct {
	TranscriptionID string
	Question        string
	MaxSegments     int
}

// AskTranscriptResult is a grounded Q&A response.
type AskTranscriptResult struct {
	Answer        string
	Citations     []TranscriptEvidence
	EvidenceCount int
	Refusal       bool
}

// AskCollectionRequest configures collection-grounded Q&A.
type AskCollectionRequest struct {
	CollectionID string
	Question     string
	MaxSegments  int
}

// RoleBriefRequest configures role-specific transcript briefing.
type RoleBriefRequest struct {
	Role            string
	TranscriptionID string
	CollectionID    string
	MaxSegments     int
}
