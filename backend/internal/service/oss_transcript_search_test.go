package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/voxis/backend/internal/domain"
)

func TestTranscriptSearchCursorRoundTrip(t *testing.T) {
	transcription := &domain.Transcription{ID: uuid.NewString(), CreatedAt: time.Date(2026, 9, 13, 8, 30, 0, 0, time.UTC)}
	encoded, err := encodeTranscriptSearchCursor(transcription)
	if err != nil {
		t.Fatalf("encodeTranscriptSearchCursor() error = %v", err)
	}
	decoded, err := decodeTranscriptSearchCursor(encoded)
	if err != nil {
		t.Fatalf("decodeTranscriptSearchCursor() error = %v", err)
	}
	if decoded.ID != transcription.ID || !decoded.CreatedAt.Equal(transcription.CreatedAt) {
		t.Fatalf("decoded cursor = %#v, want transcription cursor", decoded)
	}
}

func TestDecodeTranscriptSearchCursorRejectsInvalidValue(t *testing.T) {
	if _, err := decodeTranscriptSearchCursor("not-a-cursor"); err == nil {
		t.Fatal("decodeTranscriptSearchCursor() error = nil, want invalid input")
	}
}
