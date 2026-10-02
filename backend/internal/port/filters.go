package port

import "time"

// MediaListFilter holds filter parameters for ListByOrganizationFiltered.
type MediaListFilter struct {
	DateFrom    *time.Time
	DateTo      *time.Time
	MinDuration *float64
	MaxDuration *float64
	Status      string
	Search      string
	SortBy      string // created_at, duration, size
	SortOrder   string // asc, desc
	Limit       int
	Offset      int
}

// TranscriptionListFilter holds filter parameters for ListByOrganizationFiltered.
type TranscriptionListFilter struct {
	DateFrom    *time.Time
	DateTo      *time.Time
	MinDuration *float64
	MaxDuration *float64
	Languages   []string
	MinSpeakers *int
	MaxSpeakers *int
	Status      string
	Search      string
	SortBy      string // created_at, duration_seconds, word_count, speaker_count
	SortOrder   string // asc, desc
	Limit       int
	Offset      int
}

// SummaryListFilter holds filter parameters for ListByOrganizationFiltered.
type SummaryListFilter struct {
	DateFrom    *time.Time
	DateTo      *time.Time
	Status      string
	SummaryType string
	Search      string
	SortBy      string // created_at, word_count
	SortOrder   string // asc, desc
	Limit       int
	Offset      int
}
