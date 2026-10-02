package riverjob

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/worker"
)

// Inserter inserts retained OSS jobs.
type Inserter struct{ client *river.Client[pgx.Tx] }

// NewInserter creates a River-backed job inserter.
func NewInserter(client *river.Client[pgx.Tx]) *Inserter { return &Inserter{client: client} }

// InsertTranscribeJob queues transcription for one media object.
func (i *Inserter) InsertTranscribeJob(ctx context.Context, transcriptionID, mediaID, orgID string) error {
	_, err := i.client.Insert(ctx, worker.TranscribeJobArgs{TranscriptionID: transcriptionID, MediaID: mediaID, OrgID: orgID}, nil)
	if err != nil {
		return fmt.Errorf("insert transcription job: %w", err)
	}
	return nil
}

// InsertSpeechmaticsWaitJob queues a bounded Speechmatics completion wait.
func (i *Inserter) InsertSpeechmaticsWaitJob(ctx context.Context, transcriptionID, segmentID, jobID string) error {
	_, err := i.client.Insert(ctx, worker.SpeechmaticsWaitJobArgs{TranscriptionID: transcriptionID, SegmentID: segmentID, JobID: jobID}, nil)
	if err != nil {
		return fmt.Errorf("insert Speechmatics wait job: %w", err)
	}
	return nil
}

// InsertSummarizeJob queues structured summary generation.
func (i *Inserter) InsertSummarizeJob(ctx context.Context, summaryID, transcriptionID, orgID string) error {
	_, err := i.client.Insert(ctx, worker.SummarizeJobArgs{SummaryID: summaryID, TranscriptionID: transcriptionID, OrgID: orgID}, nil)
	if err != nil {
		return fmt.Errorf("insert summary job: %w", err)
	}
	return nil
}

// InsertStitchJob queues standard recording assembly.
func (i *Inserter) InsertStitchJob(ctx context.Context, sessionID string) error {
	_, err := i.client.Insert(ctx, worker.StitchRecordingJobArgs{SessionID: sessionID}, nil)
	if err != nil {
		return fmt.Errorf("insert recording stitch job: %w", err)
	}
	return nil
}

// InsertScanJob queues malware scanning for an encrypted media object.
func (i *Inserter) InsertScanJob(ctx context.Context, mediaID, orgID string) error {
	_, err := i.client.Insert(ctx, worker.ScanMediaJobArgs{MediaID: mediaID, OrgID: orgID}, nil)
	return err
}

var _ port.TranscriptionJobInserter = (*Inserter)(nil)
var _ port.SpeechmaticsWaitJobInserter = (*Inserter)(nil)
var _ port.SummaryJobInserter = (*Inserter)(nil)
var _ port.RecordingJobInserter = (*Inserter)(nil)
var _ port.ScanJobInserter = (*Inserter)(nil)
