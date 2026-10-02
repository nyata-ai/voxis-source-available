package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

type fakeDataPurgeRepo struct {
	org         port.PurgeUserOrganization
	mediaIDs    []string
	pending     int64
	deleteCalls []bool
	events      *[]string
	// onDeleteOrganization simulates work that lands while rows are removed.
	onDeleteOrganization func()
}

func (r *fakeDataPurgeRepo) GetUserOrganization(_ context.Context, userID string) (port.PurgeUserOrganization, error) {
	if userID != r.org.UserID {
		return port.PurgeUserOrganization{}, domain.ErrNotFound
	}
	return r.org, nil
}

func (r *fakeDataPurgeRepo) CountOrganization(context.Context, string) (port.PurgeCounts, error) {
	return port.PurgeCounts{Media: int64(len(r.mediaIDs)), PendingProviderDeletions: r.pending}, nil
}

func (r *fakeDataPurgeRepo) ListMediaIDsAfter(_ context.Context, _, afterID string, limit int) ([]string, error) {
	start := 0
	if afterID != "" {
		start = slices.Index(r.mediaIDs, afterID) + 1
	}
	return r.mediaIDs[start:min(start+limit, len(r.mediaIDs))], nil
}

func (r *fakeDataPurgeRepo) DeleteAPIKeys(context.Context, string) error {
	*r.events = append(*r.events, "delete-api-keys")
	return nil
}

func (r *fakeDataPurgeRepo) DeleteOrganizationData(_ context.Context, _ string, deleteOrganization bool) error {
	r.deleteCalls = append(r.deleteCalls, deleteOrganization)
	*r.events = append(*r.events, "delete-organization-data")
	if r.onDeleteOrganization != nil {
		r.onDeleteOrganization()
	}
	return nil
}

func newDataPurgeFixture(mediaCount int) (*DataPurgeService, *fakeDataPurgeRepo, *purgeStorage, *[]string) {
	events := &[]string{}
	mediaRepo := &purgeMediaRepo{media: map[string]*domain.Media{}, events: events}
	storage := &purgeStorage{objects: map[string]bool{
		"orgs/org-1/recordings/orphan/chunk-000.bin.enc": true,
		"orgs/org-2/media/other/encrypted.bin":           true,
	}, events: events}
	repo := &fakeDataPurgeRepo{org: port.PurgeUserOrganization{UserID: "sub-1", OrganizationID: "org-1", MemberCount: 1}, events: events}
	for i := 0; i < mediaCount; i++ {
		id := fmt.Sprintf("m%03d", i)
		repo.mediaIDs = append(repo.mediaIDs, id)
		key := "orgs/org-1/media/" + id + "/encrypted.bin"
		mediaRepo.media[id] = &domain.Media{ID: id, OrganizationID: "org-1", Status: domain.MediaStatusReady, StorageKey: key}
		storage.objects[key] = true
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	media := NewMediaService(mediaRepo, nil, nil, storage, nil, nil, logger)
	return NewDataPurgeService(repo, media, storage, logger), repo, storage, events
}

func TestDataPurgeRemovesEveryMediaAcrossPagesAndStrayObjects(t *testing.T) {
	svc, repo, storage, _ := newDataPurgeFixture(purgeMediaPageSize + 3)

	report, err := svc.Purge(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if report.MediaPurged != purgeMediaPageSize+3 || !report.OrganizationDeleted {
		t.Fatalf("report = %+v", report)
	}
	if !slices.Equal(repo.deleteCalls, []bool{true}) {
		t.Fatalf("DeleteOrganizationData calls = %v, want one with the organization", repo.deleteCalls)
	}
	for key := range storage.objects {
		if key != "orgs/org-2/media/other/encrypted.bin" {
			t.Fatalf("object %q survived the purge", key)
		}
	}
}

func TestDataPurgeRevokesAPIKeysFirstAndSweepsLateObjects(t *testing.T) {
	svc, repo, storage, events := newDataPurgeFixture(1)
	const late = "orgs/org-1/media/late/encrypted.bin"
	repo.onDeleteOrganization = func() { storage.objects[late] = true }

	if _, err := svc.Purge(context.Background(), "sub-1"); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if len(*events) == 0 || (*events)[0] != "delete-api-keys" {
		t.Fatalf("events = %v, want API keys deleted before anything else", *events)
	}
	if storage.objects[late] {
		t.Fatalf("object %q stored during the row delete survived the purge", late)
	}
	last := (*events)[len(*events)-1]
	if last != "storage-delete:"+late {
		t.Fatalf("last event = %q, want the late object swept after the row delete", last)
	}
}

func TestDataPurgeKeepsOrganizationWhileProviderDeletionIsPending(t *testing.T) {
	svc, repo, _, _ := newDataPurgeFixture(1)
	repo.pending = 2

	report, err := svc.Purge(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if report.OrganizationDeleted || report.PendingProviderDeletions != 2 {
		t.Fatalf("report = %+v, want organization kept", report)
	}
	if !slices.Equal(repo.deleteCalls, []bool{false}) {
		t.Fatalf("DeleteOrganizationData calls = %v, want one scrub-only call", repo.deleteCalls)
	}
}

func TestDataPurgeRefusesSharedOrganizationAndDryRunChangesNothing(t *testing.T) {
	svc, repo, storage, events := newDataPurgeFixture(2)
	plan, err := svc.Plan(context.Background(), "sub-1")
	if err != nil || plan.Counts.Media != 2 || len(*events) != 0 || len(storage.objects) != 4 {
		t.Fatalf("Plan() = %+v, %v; events %v", plan, err, *events)
	}

	repo.org.MemberCount = 2
	if _, err := svc.Purge(context.Background(), "sub-1"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Purge() of a shared organization error = %v, want ErrConflict", err)
	}
	if len(*events) != 0 || len(repo.deleteCalls) != 0 {
		t.Fatalf("shared organization was modified: %v", *events)
	}
	if _, err := svc.Plan(context.Background(), "unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Plan() of an unknown subject error = %v, want ErrNotFound", err)
	}
}
