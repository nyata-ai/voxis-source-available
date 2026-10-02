package gemma

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

func fillModelSlots(client *Client) {
	for i := 0; i < modelConcurrency; i++ {
		client.slots <- struct{}{}
	}
}

func TestInteractiveCallFailsWithModelBusyWhenSlotsStayFull(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCompletion(w, `{"answer":"Ada spoke.","citation_ids":["e1"]}`, "stop", 0, 0)
	}))
	defer server.Close()
	client := testClient(t, server.URL+"/v1")
	client.interactiveWait = 20 * time.Millisecond
	fillModelSlots(client)

	_, err := client.AnswerTranscriptQuestion(context.Background(), "Who spoke?", []port.TranscriptEvidence{{ID: "e1", Text: "Ada spoke."}})
	if !errors.Is(err, domain.ErrModelBusy) {
		t.Fatalf("error = %v, want ErrModelBusy", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model calls = %d, want 0 while busy", calls.Load())
	}
}

func TestInteractiveCallProceedsWhenSlotFreesWithinWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCompletion(w, `{"answer":"Ada spoke.","citation_ids":["e1"]}`, "stop", 0, 0)
	}))
	defer server.Close()
	client := testClient(t, server.URL+"/v1")
	client.interactiveWait = 5 * time.Second
	fillModelSlots(client)
	go func() {
		time.Sleep(20 * time.Millisecond)
		<-client.slots
	}()

	answer, err := client.AnswerTranscriptQuestion(context.Background(), "Who spoke?", []port.TranscriptEvidence{{ID: "e1", Text: "Ada spoke."}})
	if err != nil || answer.Answer != "Ada spoke." {
		t.Fatalf("AnswerTranscriptQuestion() = %#v, %v", answer, err)
	}
	if len(client.slots) != modelConcurrency-1 {
		t.Fatalf("slots in use = %d, want the call to release its slot", len(client.slots))
	}
}

func TestBackgroundSummaryWaitsForSlotInsteadOfModelBusy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCompletion(w, generalSummaryJSON, "stop", 0, 0)
	}))
	defer server.Close()
	client := testClient(t, server.URL+"/v1")
	client.interactiveWait = time.Millisecond
	fillModelSlots(client)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.GenerateSummary(ctx, summaryRequest())
	if errors.Is(err, domain.ErrModelBusy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want caller deadline while waiting", err)
	}

	<-client.slots
	result, err := client.GenerateSummary(context.Background(), summaryRequest())
	if err != nil || result.EndpointLocation != EndpointLabel {
		t.Fatalf("GenerateSummary() = %#v, %v", result, err)
	}
}
