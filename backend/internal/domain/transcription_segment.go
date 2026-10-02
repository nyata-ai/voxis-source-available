package domain

import (
	"fmt"
	"time"
)

// Transcription segment statuses model each bounded Speechmatics job.
const (
	TranscriptionSegmentStatusPending   = "pending"
	TranscriptionSegmentStatusSubmitted = "submitted"
	TranscriptionSegmentStatusCompleted = "completed"
	TranscriptionSegmentStatusFailed    = "failed"
)

// TranscriptionSegment tracks one Speechmatics job for a bounded media chunk.
type TranscriptionSegment struct {
	ID                    string
	TranscriptionID       string
	SegmentIndex          int
	StartOffsetSec        float64
	EndOffsetSec          float64
	Status                string
	ContentEncrypted      []byte
	ContentNonce          []byte
	WrappedDEK            []byte
	WrappingNonce         []byte
	FullTranscript        string
	Utterances            []byte
	SpeakerCount          int
	WordCount             int
	DurationSeconds       float64
	ErrorMessage          string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CompletedAt           *time.Time
	SpeechmaticsJobID     string
	SpeechmaticsDeletedAt *time.Time
}

// NewTranscriptionSegment creates one bounded media segment in a pending state.
func NewTranscriptionSegment(transcriptionID string, segmentIndex int, startOffsetSec, endOffsetSec float64) (*TranscriptionSegment, error) {
	if transcriptionID == "" || segmentIndex < 0 || startOffsetSec < 0 || endOffsetSec <= startOffsetSec {
		return nil, fmt.Errorf("invalid transcription segment: %w", ErrInvalidInput)
	}
	return &TranscriptionSegment{TranscriptionID: transcriptionID, SegmentIndex: segmentIndex, StartOffsetSec: startOffsetSec, EndOffsetSec: endOffsetSec, Status: TranscriptionSegmentStatusPending}, nil
}

// SetSubmittedSpeechmatics records this segment's provider job identifier.
func (s *TranscriptionSegment) SetSubmittedSpeechmatics(jobID string) {
	s.SpeechmaticsJobID = jobID
	s.Status = TranscriptionSegmentStatusSubmitted
	s.ErrorMessage = ""
	s.UpdatedAt = time.Now()
}

// SetCompleted records this segment's completed provider result.
func (s *TranscriptionSegment) SetCompleted(fullTranscript string, utterances []byte, speakerCount, wordCount int, durationSec float64) {
	s.FullTranscript = fullTranscript
	s.Utterances = cloneBytes(utterances)
	s.SpeakerCount = speakerCount
	s.WordCount = wordCount
	s.DurationSeconds = durationSec
	s.Status = TranscriptionSegmentStatusCompleted
	s.ErrorMessage = ""
	now := time.Now()
	s.CompletedAt = &now
	s.UpdatedAt = now
}

// SetFailed records the sanitized provider failure for this segment.
func (s *TranscriptionSegment) SetFailed(message string) {
	s.ErrorMessage = message
	s.Status = TranscriptionSegmentStatusFailed
	s.UpdatedAt = time.Now()
}

// SetSpeechmaticsDeleted records confirmed deletion of this provider job.
func (s *TranscriptionSegment) SetSpeechmaticsDeleted() {
	now := time.Now()
	s.SpeechmaticsDeletedAt = &now
	s.UpdatedAt = now
}

// Clone returns an independent copy of mutable segment fields.
func (s *TranscriptionSegment) Clone() *TranscriptionSegment {
	cloned := *s
	cloned.ContentEncrypted = cloneBytes(s.ContentEncrypted)
	cloned.ContentNonce = cloneBytes(s.ContentNonce)
	cloned.WrappedDEK = cloneBytes(s.WrappedDEK)
	cloned.WrappingNonce = cloneBytes(s.WrappingNonce)
	cloned.Utterances = cloneBytes(s.Utterances)
	cloned.CompletedAt = cloneTimePtr(s.CompletedAt)
	cloned.SpeechmaticsDeletedAt = cloneTimePtr(s.SpeechmaticsDeletedAt)
	return &cloned
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
