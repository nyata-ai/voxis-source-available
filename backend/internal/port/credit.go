package port

import (
	"context"
	"encoding/json"
)

// CreditService is the narrow reservation contract retained by the standard
// transcription worker. Voxis-OSS wires its unlimited local implementation;
// no account, payment, or billing API is part of this edition.
type CreditService interface {
	Reserve(ctx context.Context, orgID string, durationSec float64, referenceID string) (string, error)
	Commit(ctx context.Context, reservationID string) error
	Release(ctx context.Context, reservationID string) error
	ChargeUsage(ctx context.Context, orgID string, durationSec float64, referenceID string, metadata json.RawMessage) error
	HasCredit(ctx context.Context, orgID string) (bool, error)
}
