package port

import "context"

// MediaPurgeChunkLister is implemented by media repositories that can list
// the chunk objects still recorded for the session a media row was stitched
// from. MediaService.Delete removes them before the database purge. Chunk
// cleanup deletes objects before their rows, so rows are a complete list.
type MediaPurgeChunkLister interface {
	ListRecordingChunkKeysForPurge(ctx context.Context, mediaID string) ([]string, error)
}

// PurgeUserOrganization is the personal organization that owns a person's data.
type PurgeUserOrganization struct {
	UserID         string
	OrganizationID string
	MemberCount    int64
}

// PurgeCounts is the dry-run inventory of one organization's stored data.
type PurgeCounts struct {
	Media                    int64
	Transcriptions           int64
	Summaries                int64
	RecordingSessions        int64
	RecordingChunks          int64
	Collections              int64
	APIKeys                  int64
	PendingProviderDeletions int64
}

// DataPurgeRepository supports the operator purge of one person's data.
// Listing methods page by ascending ID after afterID ("" starts at the top).
type DataPurgeRepository interface {
	GetUserOrganization(ctx context.Context, userID string) (PurgeUserOrganization, error)
	CountOrganization(ctx context.Context, orgID string) (PurgeCounts, error)
	ListMediaIDsAfter(ctx context.Context, orgID, afterID string, limit int) ([]string, error)
	// DeleteAPIKeys deletes every API key of the organization, so none can
	// be used while the rest of the purge runs.
	DeleteAPIKeys(ctx context.Context, orgID string) error
	// DeleteOrganizationData removes recording sessions (and chunk rows),
	// collections, API keys, and storage allocations. When deleteOrganization
	// is true it also deletes the organization, which cascades to its users
	// and the remaining content tombstones; otherwise it scrubs the names and
	// email addresses on the organization and user rows.
	DeleteOrganizationData(ctx context.Context, orgID string, deleteOrganization bool) error
}
