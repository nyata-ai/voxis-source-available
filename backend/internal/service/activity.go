package service

import (
	"context"
	"strings"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// ActivityService assembles the read-only activity feed.
type ActivityService struct {
	recordingRepo port.RecordingRepository
	transRepo     port.TranscriptionRepository
	summaryRepo   port.SummaryRepository
}

// NewActivityService creates an ActivityService.
func NewActivityService(
	recordingRepo port.RecordingRepository,
	transRepo port.TranscriptionRepository,
	summaryRepo port.SummaryRepository,
) *ActivityService {
	return &ActivityService{
		recordingRepo: recordingRepo,
		transRepo:     transRepo,
		summaryRepo:   summaryRepo,
	}
}

// GetActivity returns active recording and transcription activity for a user/org.
func (s *ActivityService) GetActivity(ctx context.Context, orgID, userID string) ([]domain.ActivityItem, error) {
	rows, err := s.recordingRepo.ListActiveActivity(ctx, userID)
	if err != nil {
		return nil, err
	}
	trans, err := s.transRepo.ListActiveActivity(ctx, orgID, 25)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(trans))
	for _, t := range trans {
		ids = append(ids, t.ID)
	}
	statuses, err := s.summaryRepo.ListSummaryStatusesByTranscriptionIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]domain.ActivityItem, 0, len(rows)+len(trans))
	for i := range rows {
		items = append(items, recordingToActivity(rows[i]))
	}
	for _, t := range trans {
		items = append(items, transcriptionToActivity(t, statuses[t.ID]))
	}
	return items, nil
}

func recordingToActivity(row domain.RecordingActivityRow) domain.ActivityItem {
	item := domain.ActivityItem{
		Kind:      domain.ActivityKindRecording,
		RefID:     row.SessionID,
		Title:     recordingActivityTitle(row),
		Stage:     domain.ActivityStageStitching,
		Status:    domain.ActivityStatusInProgress,
		StartedAt: row.StartedAt,
	}
	switch row.Status {
	case domain.RecordingStatusCompleted:
		switch {
		case row.MediaStatus == domain.MediaStatusFailed || isFailedScanStatus(row.MediaScanStatus):
			item.Status = domain.ActivityStatusFailed
		case isMediaStreamable(row.MediaStatus, row.MediaScanStatus):
			item.Stage = domain.ActivityStageReady
			item.Status = domain.ActivityStatusCompleted
			if row.MediaID != "" {
				item.Link = "/media/" + row.MediaID
			}
		default:
			// Session finalized, but the media is not yet streamable (still
			// encrypting, or malware scan still pending). Leave the card
			// in-progress — showing "Ready" + a View link here would 409 on
			// playback until the scan clears.
		}
	case domain.RecordingStatusFailed:
		item.Status = domain.ActivityStatusFailed
	}
	return item
}

func transcriptionToActivity(t *domain.Transcription, summaryStatuses []domain.SummaryStatusInfo) domain.ActivityItem {
	link := "/transcriptions/" + t.ID
	item := domain.ActivityItem{
		Kind:      domain.ActivityKindTranscription,
		RefID:     t.ID,
		Link:      link,
		Title:     transcriptionActivityTitle(t),
		Stage:     domain.ActivityStageTranscribing,
		Status:    domain.ActivityStatusInProgress,
		StartedAt: t.CreatedAt,
	}
	switch t.Status {
	case domain.TranscriptionStatusCompleted:
		item.Stage = domain.ActivityStageReady
		item.Status = domain.ActivityStatusCompleted
		detail := summaryActivityDetail(summaryStatuses)
		if detail != nil {
			item.Stage = domain.ActivityStageSummarizing
			item.Status = domain.ActivityStatusInProgress
			item.Detail = detail
		}
	case domain.TranscriptionStatusFailed:
		item.Status = domain.ActivityStatusFailed
		item.ErrorMessage = safeActivityFailureMessage(t.ErrorMessage)
	}
	return item
}

// safeActivityFailureMessage exposes only stable categories in the activity
// feed. Provider response text can contain request details and remains in
// server logs for an administrator to inspect.
func safeActivityFailureMessage(message string) string {
	switch {
	case message == "media not found", message == "media not ready", message == "decryption failed", message == "provider authentication failed":
		return message
	case strings.HasPrefix(message, "media scan status:"):
		return "media failed the security scan"
	case strings.HasPrefix(message, "submission failed after"):
		return "transcription provider was unavailable after its retry limit"
	case message == "Speechmatics job expired", message == "Speechmatics job rejected":
		return message
	default:
		return "transcription failed; inspect provider configuration and retry"
	}
}

func recordingActivityTitle(row domain.RecordingActivityRow) string {
	if row.MediaTitle != "" {
		return row.MediaTitle
	}
	if row.MediaFilename != "" {
		return row.MediaFilename
	}
	return row.MicrophoneLabel
}

func transcriptionActivityTitle(t *domain.Transcription) string {
	if t.MediaTitle != "" {
		return t.MediaTitle
	}
	if t.MediaFilename != "" {
		return t.MediaFilename
	}
	return t.ID
}

func isFailedScanStatus(status string) bool {
	return status == domain.ScanStatusInfected || status == domain.ScanStatusError
}

// isMediaStreamable reports whether a recording's media can actually be opened:
// fully encrypted (status ready) and past malware scanning (clean or skipped).
func isMediaStreamable(status, scan string) bool {
	return status == domain.MediaStatusReady &&
		(scan == domain.ScanStatusClean || scan == domain.ScanStatusSkipped)
}

func summaryActivityDetail(summaryStatuses []domain.SummaryStatusInfo) *domain.ActivityDetail {
	if len(summaryStatuses) == 0 {
		return nil
	}
	terminal := 0
	for _, status := range summaryStatuses {
		switch status.Status {
		case domain.SummaryStatusCompleted, domain.SummaryStatusFailed:
			terminal++
		}
	}
	if terminal == len(summaryStatuses) {
		return nil
	}
	return &domain.ActivityDetail{Terminal: terminal, Total: len(summaryStatuses)}
}
