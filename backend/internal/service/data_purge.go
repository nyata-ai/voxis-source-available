package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	purgeMediaPageSize = 100
	// maxPurgeMediaPages bounds the media walk (Power-of-10 rule 2): one
	// million media rows is far beyond any single person's library.
	maxPurgeMediaPages = 10_000
)

// DataPurgeService removes everything one person stored: their personal
// organization's media and files, transcripts, summaries, recordings,
// collections, and API keys. It serves the operator's purge-user command.
type DataPurgeService struct {
	repo    port.DataPurgeRepository
	media   *MediaService
	storage port.StorageClient
	logger  *slog.Logger
}

// PurgePlan is what a purge would remove, reported by the dry run.
type PurgePlan struct {
	UserID         string
	OrganizationID string
	Counts         port.PurgeCounts
}

// PurgeReport is what a confirmed purge removed.
type PurgeReport struct {
	PurgePlan
	MediaPurged int
	// OrganizationDeleted is false while the transcription provider still has
	// to confirm deletion of submitted jobs; the organization and its
	// content-free tombstones then stay (with names and emails scrubbed) until
	// a later run, after the provider cleanup has finished.
	OrganizationDeleted      bool
	PendingProviderDeletions int64
}

// NewDataPurgeService creates the service behind the purge-user command.
func NewDataPurgeService(repo port.DataPurgeRepository, media *MediaService, storage port.StorageClient, logger *slog.Logger) *DataPurgeService {
	if repo == nil || media == nil || storage == nil {
		panic("service: data purge requires a repository, the media service, and storage")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DataPurgeService{repo: repo, media: media, storage: storage, logger: logger}
}

// Plan reports what Purge would remove, without changing anything. It refuses
// an organization shared with other people: purge-user only removes a
// personal organization.
func (s *DataPurgeService) Plan(ctx context.Context, userID string) (PurgePlan, error) {
	org, err := s.repo.GetUserOrganization(ctx, userID)
	if err != nil {
		return PurgePlan{}, fmt.Errorf("find user organization: %w", err)
	}
	if org.MemberCount != 1 {
		return PurgePlan{}, fmt.Errorf("organization %s has %d members; purge-user only removes a personal organization: %w",
			org.OrganizationID, org.MemberCount, domain.ErrConflict)
	}
	counts, err := s.repo.CountOrganization(ctx, org.OrganizationID)
	if err != nil {
		return PurgePlan{}, fmt.Errorf("count organization data: %w", err)
	}
	return PurgePlan{UserID: org.UserID, OrganizationID: org.OrganizationID, Counts: counts}, nil
}

// Purge first deletes the organization's API keys, so no key can add content
// while it runs. It then removes the person's data with the same primitive as
// a user's media delete, every remaining stored object under the
// organization, and the organization's rows, and finally sweeps the storage
// prefix again for objects written while the rows were being removed. It does
// not sign the person out: the operator disables the Keycloak account first.
// It is safe to re-run after a partial failure.
func (s *DataPurgeService) Purge(ctx context.Context, userID string) (PurgeReport, error) {
	plan, err := s.Plan(ctx, userID)
	if err != nil {
		return PurgeReport{}, err
	}
	report := PurgeReport{PurgePlan: plan}
	orgID := plan.OrganizationID
	if keyErr := s.repo.DeleteAPIKeys(ctx, orgID); keyErr != nil {
		return report, fmt.Errorf("delete API keys: %w", keyErr)
	}
	if report.MediaPurged, err = s.purgeAllMedia(ctx, orgID); err != nil {
		return report, err
	}
	if deleteErr := DeleteStorageByPrefix(ctx, s.storage, domain.OrganizationStoragePrefix(orgID)); deleteErr != nil {
		return report, fmt.Errorf("delete remaining stored objects: %w", deleteErr)
	}
	after, err := s.repo.CountOrganization(ctx, orgID)
	if err != nil {
		return report, fmt.Errorf("recount organization data: %w", err)
	}
	report.PendingProviderDeletions = after.PendingProviderDeletions
	report.OrganizationDeleted = after.PendingProviderDeletions == 0
	if deleteErr := s.repo.DeleteOrganizationData(ctx, orgID, report.OrganizationDeleted); deleteErr != nil {
		return report, fmt.Errorf("delete organization data: %w", deleteErr)
	}
	// An upload that finished after the first sweep has no row left to point
	// at its file; remove any such object now.
	if deleteErr := DeleteStorageByPrefix(ctx, s.storage, domain.OrganizationStoragePrefix(orgID)); deleteErr != nil {
		return report, fmt.Errorf("delete objects stored during the purge: %w", deleteErr)
	}
	s.logger.Info("user data purged", "org_id", orgID, "media_purged", report.MediaPurged,
		"organization_deleted", report.OrganizationDeleted)
	return report, nil
}

// purgeAllMedia runs MediaService.Delete on every media row of the
// organization, including rows an older release only soft-deleted.
func (s *DataPurgeService) purgeAllMedia(ctx context.Context, orgID string) (int, error) {
	purged := 0
	after := ""
	for page := 0; page < maxPurgeMediaPages; page++ {
		ids, err := s.repo.ListMediaIDsAfter(ctx, orgID, after, purgeMediaPageSize)
		if err != nil {
			return purged, fmt.Errorf("list media: %w", err)
		}
		for _, id := range ids {
			if err := s.media.Delete(ctx, orgID, id); err != nil {
				return purged, fmt.Errorf("purge media %s: %w", id, err)
			}
			purged++
		}
		if len(ids) < purgeMediaPageSize {
			return purged, nil
		}
		after = ids[len(ids)-1]
	}
	return purged, fmt.Errorf("media listing exceeded %d pages: %w", maxPurgeMediaPages, domain.ErrInternal)
}
