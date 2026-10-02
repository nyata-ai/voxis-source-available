package port

import "time"

// StreamTokenPayload describes a signed token for media streaming.
type StreamTokenPayload struct {
	MediaID string    `json:"media_id"`
	OrgID   string    `json:"org_id"`
	Sub     string    `json:"sub"`
	Exp     time.Time `json:"exp"`
}

// StreamTokenSigner creates and validates stream tokens.
type StreamTokenSigner interface {
	Sign(payload StreamTokenPayload) (string, error)
	Verify(token string) (*StreamTokenPayload, error)
}
