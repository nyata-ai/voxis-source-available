package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	compensationDeleteTimeout = 30 * time.Second
	speechmaticsWebhookPath   = "/api/v1/webhooks/speechmatics/"
)

func (w *TranscribeWorker) deleteForCompensation(ctx context.Context, jobID string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), compensationDeleteTimeout)
	defer cancel()
	return w.provider.Delete(ctx, jobID)
}

func (w *TranscribeWorker) applySpeechmaticsCallback(req *port.TranscriptionRequest) {
	if !w.speechmaticsWebhook || !req.CallbackEnabled {
		req.CallbackEnabled = false
		req.CallbackURL = ""
		return
	}
	req.CallbackURL = w.callbackBaseURL + speechmaticsWebhookPath + w.webhookSecret
}

func (w *TranscribeWorker) segmentSubmitted(seg *domain.TranscriptionSegment) bool {
	return seg.SpeechmaticsJobID != ""
}

func (w *TranscribeWorker) persistChunkedParentSubmission(ctx context.Context, transcription *domain.Transcription) error {
	transcription.SetSubmittedSpeechmatics("")
	return w.smRepo.MarkSubmittedToSpeechmatics(ctx, transcription.ID, "", transcription.PreprocessorUsed)
}

func (w *TranscribeWorker) persistParentSubmission(ctx context.Context, transcription *domain.Transcription, jobID string) error {
	transcription.SetSubmittedSpeechmatics(jobID)
	if err := w.smRepo.MarkSubmittedToSpeechmatics(ctx, transcription.ID, jobID, transcription.PreprocessorUsed); err != nil {
		persisted, getErr := w.transRepo.GetByID(ctx, transcription.ID)
		if getErr == nil && persisted.Status == domain.TranscriptionStatusSubmitted && persisted.SpeechmaticsJobID == jobID {
			return nil
		}
		if deleteErr := w.deleteForCompensation(ctx, jobID); deleteErr != nil {
			return fmt.Errorf("persist submitted transcription: %w; remove provider job: %w", err, deleteErr)
		}
		return fmt.Errorf("persist submitted transcription: %w", err)
	}
	w.enqueueSpeechmaticsWait(ctx, SpeechmaticsWaitJobArgs{TranscriptionID: transcription.ID, JobID: jobID})
	return nil
}

func (w *TranscribeWorker) persistSegmentSubmission(ctx context.Context, segment *domain.TranscriptionSegment, jobID string) error {
	segment.SetSubmittedSpeechmatics(jobID)
	if err := w.smSegRepo.MarkSegmentSubmittedToSpeechmatics(ctx, segment.ID, jobID); err != nil {
		persisted, getErr := w.smSegRepo.GetSegmentBySpeechmaticsJobID(ctx, jobID)
		if getErr == nil && persisted.ID == segment.ID && persisted.Status == domain.TranscriptionSegmentStatusSubmitted {
			return nil
		}
		if deleteErr := w.deleteForCompensation(ctx, jobID); deleteErr != nil {
			return fmt.Errorf("persist submitted segment: %w; remove provider job: %w", err, deleteErr)
		}
		return fmt.Errorf("persist submitted segment: %w", err)
	}
	w.enqueueSpeechmaticsWait(ctx, SpeechmaticsWaitJobArgs{SegmentID: segment.ID, JobID: jobID})
	return nil
}

func (w *TranscribeWorker) enqueueSpeechmaticsWait(ctx context.Context, args SpeechmaticsWaitJobArgs) {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil || client == nil {
		return
	}
	if _, err := client.Insert(ctx, args, nil); err != nil {
		w.logger.Warn("failed to enqueue Speechmatics wait job", "speechmatics_job_id", args.JobID, "error", err)
	}
}
