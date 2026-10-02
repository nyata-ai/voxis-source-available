package port

import (
	"context"
	"time"
)

// EmailOutcome is the terminal disposition of one attempted send.
//
// The set exists because the durable record cannot express it. Every branch of
// SendEmailWorker.Work that stops trying returns nil, so River finalizes a
// dropped message as `completed` exactly like a delivered one — a relay that
// rejects our credentials and silently destroys every outgoing message is
// indistinguishable, in river_job, from perfect delivery. These counters are the
// only place that difference is visible.
type EmailOutcome string

const (
	// EmailSent is a message the relay accepted.
	EmailSent EmailOutcome = "sent"
	// EmailRetried is a transient failure the job will be retried after.
	EmailRetried EmailOutcome = "retried"
	// EmailDroppedUndeliverable is a permanent rejection of this message or its
	// recipient — a dead mailbox. Bounded blast radius.
	EmailDroppedUndeliverable EmailOutcome = "dropped_undeliverable"
	// EmailDroppedMisconfigured is a rejection of our credentials or envelope
	// sender. It condemns every message we will send until a human intervenes,
	// which makes it the loudest number on this surface.
	EmailDroppedMisconfigured EmailOutcome = "dropped_misconfigured"
	// EmailDroppedUnrenderable is a message that never reached the relay: an
	// unknown kind, an undecodable payload, or an empty recipient.
	EmailDroppedUnrenderable EmailOutcome = "dropped_unrenderable"
)

// EmailMetricsRecorder counts one send attempt's outcome, keyed by email kind.
//
// Implementations must be safe for concurrent use: the email queue runs several
// workers at once.
type EmailMetricsRecorder interface {
	RecordEmail(kind string, outcome EmailOutcome)
}

// SMTPMetricsRecorder counts protocol-level events the send outcome cannot
// express. Kept separate from EmailMetricsRecorder so the SMTP adapter depends
// only on the one method it uses.
type SMTPMetricsRecorder interface {
	// RecordSMTPQuitFailure counts a QUIT that failed AFTER the relay accepted
	// the message. The send succeeded and no error counter moves, so without
	// this a relay that systematically fails QUIT is invisible.
	RecordSMTPQuitFailure()
}

// EmailKindMetrics is one email kind's counters.
type EmailKindMetrics struct {
	Kind                 string `json:"kind"`
	Sent                 int64  `json:"sent"`
	Retried              int64  `json:"retried"`
	DroppedUndeliverable int64  `json:"dropped_undeliverable"`
	DroppedMisconfigured int64  `json:"dropped_misconfigured"`
	DroppedUnrenderable  int64  `json:"dropped_unrenderable"`
}

// EmailMetricsSnapshot is a consistent read of the email counters.
//
// The counters live in process memory and reset when the process does, which is
// why ObservedSince is part of the snapshot: "0 sent" means something entirely
// different an hour after a deploy than it does after a fortnight of uptime.
type EmailMetricsSnapshot struct {
	ObservedSince time.Time          `json:"observed_since"`
	Kinds         []EmailKindMetrics `json:"kinds"`
	QuitFailures  int64              `json:"quit_failures"`
}

// EmailMetricsSource reads the process-local email counters.
type EmailMetricsSource interface {
	EmailMetrics() EmailMetricsSnapshot
}

// Sweeper kinds. These MUST equal the River job kind of the corresponding
// worker — worker tests pin the equality — because the durable history is keyed
// by the same string River uses. Renaming one orphans every historical row.
const (
	SweepKindExpireCredits       = "expire_credits"
	SweepKindWarnExpiringCredits = "warn_expiring_credits"
)

// ExpectedSweepKinds lists the sweepers the observability surface reports on,
// whether or not they have ever run. Without it a sweeper that never started
// would simply be absent from the payload — the one failure mode hardest to
// notice. Returns a fresh slice so no caller can edit the list.
func ExpectedSweepKinds() []string {
	return []string{SweepKindExpireCredits, SweepKindWarnExpiringCredits}
}

// SweepRun is one completed execution of a periodic sweeper.
type SweepRun struct {
	Kind       string
	StartedAt  time.Time
	FinishedAt time.Time
	// Accounts is what the run acted on: balances zeroed, or organizations
	// warned.
	Accounts int
	// Credits is what the run confiscated. Zero for sweepers that move no
	// balance.
	Credits int64
	// Failure is empty when the run succeeded. Recorded because both credit
	// sweepers swallow their errors, so River reports a failed run as completed.
	Failure string
}

// SweepRunRecorder persists one sweep run.
//
// It is called after the work it describes has already committed, so callers
// must treat a failure as a lost history row and never as a reason to fail —
// still less to retry — a money-touching sweep.
type SweepRunRecorder interface {
	RecordSweepRun(ctx context.Context, run SweepRun) error
}

// SweepRunSummary is one sweeper kind's history: its newest run, plus totals
// over the window the reader was asked for.
//
// LastFinishedAt is the newest run of that kind whenever it happened, NOT the
// newest within the window — a sweeper silent for six weeks must report the real
// date rather than look identical to one that never ran at all.
type SweepRunSummary struct {
	Kind           string
	LastStartedAt  time.Time
	LastFinishedAt time.Time
	LastAccounts   int
	LastCredits    int64
	LastFailure    string
	Runs           int64
	Failures       int64
	Accounts       int64
	Credits        int64
}

// SweepRunReader reads sweep history for the admin observability surface.
type SweepRunReader interface {
	SummarizeSweepRuns(ctx context.Context, since time.Time) ([]SweepRunSummary, error)
}
