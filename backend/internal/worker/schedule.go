// Copyright © 2026 PT. Karya Nyata Teknologi (Nyata.AI).
//
// This file is part of Voxis Source-Available. Use, modification, and distribution are
// governed by the license accompanying this distribution.

package worker

import (
	"time"

	"github.com/riverqueue/river"
)

// DailyAtUTC runs a job once per day at a fixed UTC wall-clock time.
//
// River ships only PeriodicInterval, which anchors to whenever the client
// started: two 24-hour jobs registered in one boot fire together, and every
// restart shifts both. Money-touching sweeps deserve a deterministic hour —
// one that stays put across deploys and can be reasoned about from a log
// timestamp — so the credit sweeps use this instead.
//
// The zone is UTC, never time.Local: a local-time schedule would skip or
// repeat a run at a DST transition, and a machine's zone is not something a
// billing sweep should depend on.
//
// Hour and Minute are expected in range (0-23, 0-59). Out-of-range values are
// normalized by time.Date rather than rejected — Next has no error to return —
// so the schedule still advances, just at a shifted hour.
type DailyAtUTC struct{ Hour, Minute int }

// Compile-time proof that the schedule is usable by river.NewPeriodicJob.
var _ river.PeriodicSchedule = DailyAtUTC{}

// Next returns the next occurrence of the configured time strictly after
// current. On the target minute exactly it returns tomorrow, never current
// itself: a zero-length interval would make River insert a job every tick.
func (d DailyAtUTC) Next(current time.Time) time.Time {
	utc := current.UTC()
	next := time.Date(utc.Year(), utc.Month(), utc.Day(),
		d.Hour, d.Minute, 0, 0, time.UTC)
	if !next.After(utc) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
