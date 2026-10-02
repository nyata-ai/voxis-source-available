package port

import (
	"context"
	"io"
)

// ScanResult holds the outcome of a malware scan.
type ScanResult struct {
	Clean      bool   // true if no threats detected
	ThreatName string // non-empty when Clean is false (e.g., "Win.Trojan.Agent-123")
}

// MalwareScanner scans file content for malware.
type MalwareScanner interface {
	// Scan reads from r and returns the scan result.
	// The caller is responsible for closing r.
	Scan(ctx context.Context, r io.Reader) (*ScanResult, error)

	// Available reports whether the scanner backend is reachable.
	// Takes context because this performs a network call (TCP dial + PING),
	// unlike AudioProber.Available() which only checks a local binary path.
	Available(ctx context.Context) bool
}

// ScanJobInserter enqueues malware scan background jobs.
type ScanJobInserter interface {
	InsertScanJob(ctx context.Context, mediaID, orgID string) error
}
