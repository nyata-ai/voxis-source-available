package service

import (
	"testing"

	"github.com/voxis/backend/internal/domain"
)

func TestOSSActivityFailureReasonIsSafe(t *testing.T) {
	item := transcriptionToActivity(&domain.Transcription{
		ID:           "failed-transcription",
		Status:       domain.TranscriptionStatusFailed,
		ErrorMessage: "provider said token=secret and request=https://private.example",
	}, nil)
	if item.ErrorMessage != "transcription failed; inspect provider configuration and retry" {
		t.Fatalf("error message = %q", item.ErrorMessage)
	}
}

func TestOSSActivityFinalRetryReasonIsActionable(t *testing.T) {
	item := transcriptionToActivity(&domain.Transcription{
		ID:           "failed-transcription",
		Status:       domain.TranscriptionStatusFailed,
		ErrorMessage: "submission failed after 3 attempts",
	}, nil)
	if item.ErrorMessage != "transcription provider was unavailable after its retry limit" {
		t.Fatalf("error message = %q", item.ErrorMessage)
	}
}
