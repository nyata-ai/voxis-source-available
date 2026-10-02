package domain

import "time"

// RecordingActivityRow is a recording session joined with its (nullable) media.
// Media* fields are empty when no media row exists yet (mid-stitch).
type RecordingActivityRow struct {
	SessionID       string
	Status          string // recording_sessions.status
	MediaID         string // "" until stitched
	MicrophoneLabel string
	MediaStatus     string // "" | pending | encrypting | ready | failed
	MediaScanStatus string // "" | scan_pending | scan_clean | scan_infected | ...
	MediaTitle      string
	MediaFilename   string
	StartedAt       time.Time // created_at
	UpdatedAt       time.Time
}

// Activity item kinds, stages, statuses (stable tokens the client maps to i18n).
const (
	ActivityKindRecording     = "recording"
	ActivityKindTranscription = "transcription"

	ActivityStageStitching    = "stitching"
	ActivityStageTranscribing = "transcribing"
	ActivityStageSummarizing  = "summarizing"
	ActivityStageReady        = "ready"

	ActivityStatusInProgress = "in_progress"
	ActivityStatusCompleted  = "completed"
	ActivityStatusFailed     = "failed"
)

// ActivityDetail carries a truthful sub-count (summaries only).
type ActivityDetail struct {
	Terminal int
	Total    int
}

// ActivityItem is one row in the activity feed.
type ActivityItem struct {
	Kind         string
	RefID        string
	Link         string
	Title        string
	Stage        string
	ErrorMessage string
	Status       string
	Detail       *ActivityDetail
	StartedAt    time.Time
}
