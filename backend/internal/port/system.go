package port

import (
	"context"
	"time"
)

// SystemCPU holds load averages and core count.
type SystemCPU struct {
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
	Cores  int     `json:"cores"`
}

// SystemMemory holds host memory in bytes. AvailableBytes is free memory —
// distinct from a section's `Available` health flag.
type SystemMemory struct {
	UsedBytes      int64 `json:"used_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	TotalBytes     int64 `json:"total_bytes"`
}

// SystemUptime holds host and process uptime in seconds.
type SystemUptime struct {
	HostSeconds    int64 `json:"host_seconds"`
	ProcessSeconds int64 `json:"process_seconds"`
}

// SystemHostStats holds host vitals.
type SystemHostStats struct {
	Available  bool         `json:"available"`
	Reason     string       `json:"reason,omitempty"`
	IP         string       `json:"ip"`
	ExternalIP string       `json:"external_ip,omitempty"`
	CPU        SystemCPU    `json:"cpu"`
	Memory     SystemMemory `json:"memory"`
	Uptime     SystemUptime `json:"uptime"`
}

// SystemBuildStats holds backend build info from ldflags.
type SystemBuildStats struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// SystemDiskStats holds VM filesystem usage for one mount.
type SystemDiskStats struct {
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	Mount      string `json:"mount"`
	UsedBytes  int64  `json:"used_bytes"`
	FreeBytes  int64  `json:"free_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

// SystemGCSStats holds GCS bucket usage from Cloud Monitoring. HasData=false
// means no metric point yet (e.g. empty bucket / not sampled) — not a failure.
type SystemGCSStats struct {
	Bucket      string    `json:"bucket"`
	HasData     bool      `json:"has_data"`
	TotalBytes  int64     `json:"total_bytes"`
	ObjectCount int64     `json:"object_count"`
	AsOf        time.Time `json:"as_of"`
}

// SystemStorageStats describes the media storage backend.
type SystemStorageStats struct {
	Available bool            `json:"available"`
	Reason    string          `json:"reason,omitempty"`
	Backend   string          `json:"backend"` // "gcs" | "memory"
	GCS       *SystemGCSStats `json:"gcs,omitempty"`
}

// SystemDBPool holds pgxpool connection counts.
type SystemDBPool struct {
	Acquired int32 `json:"acquired"`
	Idle     int32 `json:"idle"`
	Max      int32 `json:"max"`
	Total    int32 `json:"total"`
}

// SystemDatabaseStats holds Postgres size and pool stats.
type SystemDatabaseStats struct {
	Available bool         `json:"available"`
	Reason    string       `json:"reason,omitempty"`
	SizeBytes int64        `json:"size_bytes"`
	Pool      SystemDBPool `json:"pool"`
}

// SystemUsersStats holds user/org counts.
type SystemUsersStats struct {
	Available     bool   `json:"available"`
	Reason        string `json:"reason,omitempty"`
	Total         int64  `json:"total"`
	Organizations int64  `json:"organizations"`
	New7d         int64  `json:"new_7d"`
	New30d        int64  `json:"new_30d"`
}

// SystemDependency holds one dependency's health.
type SystemDependency struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // "up" | "down"
	LatencyMS int64  `json:"latency_ms"`
	Reason    string `json:"reason,omitempty"`
}

// SystemSweepStats is one periodic sweeper's health.
//
// "Never ran" is a state of its own, not a zero: LastRunAt and HoursSinceRun are
// both nil and NeverRan is true. Reporting an age of zero instead would make the
// two natural alarm predicates — `stale == true` and `hours_since_run > N` —
// both read HEALTHY for a sweeper that has never started, which is the failure
// hardest to notice and the one this section exists to catch.
type SystemSweepStats struct {
	Kind      string     `json:"kind"`
	LastRunAt *time.Time `json:"last_run_at"`
	// NeverRan means the durable history holds no run for this kind at all.
	// A boolean alarm rule can key on it directly; a nil-aware rule reads the
	// null hours_since_run instead. Both work, deliberately.
	NeverRan bool `json:"never_ran"`
	// HoursSinceRun is nil when NeverRan — never omitted, so the JSON carries an
	// explicit null rather than silently dropping the field.
	HoursSinceRun *float64 `json:"hours_since_run"`
	// Stale is the alarm: no successful-or-otherwise run for longer than
	// StaleAfterHours. Best-effort daily scheduling means an alert must key on
	// staleness, never on an assumed cadence. For a sweeper that has never run
	// it is gated on process uptime so a fresh deploy does not cry wolf — which
	// is precisely why NeverRan has to be readable on its own.
	Stale          bool   `json:"stale"`
	LastDurationMS int64  `json:"last_duration_ms"`
	LastAccounts   int    `json:"last_accounts"`
	LastCredits    int64  `json:"last_credits"`
	LastFailure    string `json:"last_failure,omitempty"`
	Runs           int64  `json:"runs"`
	Failures       int64  `json:"failures"`
	Accounts       int64  `json:"accounts"`
	Credits        int64  `json:"credits"`
}

// SystemCreditBacklog is the work the expiry sweeper still owes. A figure that
// stays non-zero well past the sweep hour means the sweeper is failing or is
// capped by its batch size.
type SystemCreditBacklog struct {
	Accounts int64 `json:"accounts"`
	Credits  int64 `json:"credits"`
}

// SystemExpiredCredits is what expiry took over the reported window, split by
// how it was taken. Sweep entries are whole balances zeroed by a sweep; hold
// entries are single in-flight reservations confiscated on release because the
// balance lapsed while their job ran — which happens outside any sweep.
type SystemExpiredCredits struct {
	SweepEntries int64 `json:"sweep_entries"`
	SweepCredits int64 `json:"sweep_credits"`
	HoldEntries  int64 `json:"hold_entries"`
	HoldCredits  int64 `json:"hold_credits"`
}

// SystemCreditExpiryStats is the credit expiry observability section.
type SystemCreditExpiryStats struct {
	Available       bool                 `json:"available"`
	Reason          string               `json:"reason,omitempty"`
	WindowDays      int                  `json:"window_days"`
	StaleAfterHours int                  `json:"stale_after_hours"`
	Backlog         SystemCreditBacklog  `json:"backlog"`
	Expired         SystemExpiredCredits `json:"expired"`
	Sweeps          []SystemSweepStats   `json:"sweeps"`
}

// SystemEmailQueueStats is the durable half of email health, read from River's
// job table. Pending and Retryable are live state and survive a restart;
// Discarded covers River's 7-day discarded-job retention.
type SystemEmailQueueStats struct {
	Pending     int64 `json:"pending"`
	Retryable   int64 `json:"retryable"`
	Discarded7d int64 `json:"discarded_7d"`
}

// SystemEmailStats is the transactional email section: process-local delivery
// counters plus, when readable, the durable queue depth.
//
// The counters reset with the process — ObservedSince says when they started —
// because the distinction they carry exists nowhere else. A dropped message and
// a delivered one are the same `completed` river_job row.
type SystemEmailStats struct {
	Available            bool                   `json:"available"`
	Reason               string                 `json:"reason,omitempty"`
	ObservedSince        time.Time              `json:"observed_since"`
	Sent                 int64                  `json:"sent"`
	Retried              int64                  `json:"retried"`
	DroppedUndeliverable int64                  `json:"dropped_undeliverable"`
	DroppedMisconfigured int64                  `json:"dropped_misconfigured"`
	DroppedUnrenderable  int64                  `json:"dropped_unrenderable"`
	QuitFailures         int64                  `json:"quit_failures"`
	Kinds                []EmailKindMetrics     `json:"kinds"`
	Queue                *SystemEmailQueueStats `json:"queue,omitempty"`
}

// SystemStats is the full admin system observability payload.
type SystemStats struct {
	GeneratedAt  time.Time               `json:"generated_at"`
	Host         SystemHostStats         `json:"host"`
	Build        SystemBuildStats        `json:"build"`
	Disk         SystemDiskStats         `json:"disk"`
	Storage      SystemStorageStats      `json:"storage"`
	Database     SystemDatabaseStats     `json:"database"`
	Users        SystemUsersStats        `json:"users"`
	CreditExpiry SystemCreditExpiryStats `json:"credit_expiry"`
	Email        SystemEmailStats        `json:"email"`
	Dependencies []SystemDependency      `json:"dependencies"`
}

// SystemStatsCollector assembles infrastructure observability stats.
// The returned error is non-nil ONLY for programmer/configuration faults or
// context cancellation. Expected probe failures degrade to section-level
// Available=false + Reason and still return a partial *SystemStats with nil error.
type SystemStatsCollector interface {
	GetSystemStats(ctx context.Context) (*SystemStats, error)
}
