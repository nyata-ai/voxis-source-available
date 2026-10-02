package main

import (
	"testing"

	"github.com/riverqueue/river"
	"github.com/voxis/backend/internal/worker"
)

func TestOSSQueueRoutingIncludesEveryRegisteredWorkerQueue(t *testing.T) {
	cfg := newRiverConfig(nil, nil)
	jobs := []interface{ InsertOpts() river.InsertOpts }{
		worker.TranscribeJobArgs{},
		worker.PollSpeechmaticsJobArgs{},
		worker.CleanSpeechmaticsJobArgs{},
		worker.SpeechmaticsWaitJobArgs{},
		worker.SummarizeJobArgs{},
		worker.StitchRecordingJobArgs{},
		worker.CleanupRecordingJobArgs{},
		worker.ScanMediaJobArgs{},
	}
	for _, job := range jobs {
		queue := job.InsertOpts().Queue
		if _, ok := cfg.Queues[queue]; !ok {
			t.Fatalf("%T routes to unconfigured queue %q", job, queue)
		}
	}
	if got := cfg.Queues["summarization"].MaxWorkers; got != 1 {
		t.Fatalf("summarization workers = %d, want 1 for the tested Gemma profile", got)
	}
}
