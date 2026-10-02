package port

import (
	"context"
	"io"
)

// TranscriptionRequest is the bounded Speechmatics batch-job request.
type TranscriptionRequest struct {
	AudioURL         string
	Diarization      bool
	ExpectedSpeakers int
	Languages        []string
	Reference        string
	CallbackEnabled  bool
	CallbackURL      string
	Vocabulary       []VocabTerm
}

// VocabTerm describes a custom vocabulary term when a provider supports it.
type VocabTerm struct {
	Value          string
	Pronunciations []string
	Intensity      float64
	Language       string
}

// TranscriptionResult is the sanitized provider result returned to workers.
type TranscriptionResult struct {
	Status         string
	FullTranscript string
	Utterances     []byte
	Languages      []string
	SpeakerCount   int
	WordCount      int
	AudioDuration  float64
	ErrorCode      int
	ErrorMessage   string
}

// TranscriptionProvider owns the complete upload, submit, poll, and deletion lifecycle.
type TranscriptionProvider interface {
	Upload(ctx context.Context, src io.Reader, filename, contentType string) (string, error)
	Submit(ctx context.Context, req TranscriptionRequest) (string, error)
	GetStatus(ctx context.Context, jobID string) (*TranscriptionResult, error)
	Delete(ctx context.Context, jobID string) error
	ConfirmDeleted(ctx context.Context, jobID string) error
}
