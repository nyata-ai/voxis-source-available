package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

var errStorageDown = errors.New("storage unavailable")

// purgeMediaRepo records the order of the database purge relative to the
// storage deletes, which share the events log.
type purgeMediaRepo struct {
	port.MediaRepository
	media     map[string]*domain.Media
	chunkKeys []string
	events    *[]string
	marked    []string
}

func (r *purgeMediaRepo) GetByID(_ context.Context, id string) (*domain.Media, error) {
	media, ok := r.media[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := *media
	return &copied, nil
}

func (r *purgeMediaRepo) CascadeDelete(_ context.Context, id string) error {
	*r.events = append(*r.events, "db-purge:"+id)
	r.media[id].Status = domain.MediaStatusDeleted
	r.media[id].StorageKey = ""
	return nil
}

func (r *purgeMediaRepo) ListRecordingChunkKeysForPurge(context.Context, string) ([]string, error) {
	return r.chunkKeys, nil
}

func (r *purgeMediaRepo) MarkAudioDeleted(_ context.Context, mediaID, storageKey string, _ time.Time) (*domain.Media, error) {
	*r.events = append(*r.events, "mark-audio-deleted:"+storageKey)
	r.marked = append(r.marked, mediaID)
	return r.media[mediaID], nil
}

type purgeStorage struct {
	port.StorageClient
	objects map[string]bool
	failKey string
	events  *[]string
}

func (s *purgeStorage) Delete(_ context.Context, key string) error {
	if key == s.failKey {
		return errStorageDown
	}
	if !s.objects[key] {
		return domain.ErrNotFound
	}
	delete(s.objects, key)
	*s.events = append(*s.events, "storage-delete:"+key)
	return nil
}

func (s *purgeStorage) ListByPrefix(_ context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys, nil
}

func newPurgeFixture() (*MediaService, *purgeMediaRepo, *purgeStorage, *[]string) {
	events := &[]string{}
	repo := &purgeMediaRepo{
		media: map[string]*domain.Media{"m1": {
			ID: "m1", OrganizationID: "org-1", Status: domain.MediaStatusReady,
			StorageKey: "orgs/org-1/media/m1/encrypted.bin",
		}},
		chunkKeys: []string{"orgs/org-1/recordings/s1/chunk-000.bin.enc", "orgs/org-1/recordings/s1/chunk-001.bin.enc"},
		events:    events,
	}
	storage := &purgeStorage{objects: map[string]bool{
		"orgs/org-1/media/m1/encrypted.bin":          true,
		"orgs/org-1/recordings/s1/chunk-000.bin.enc": true,
		"orgs/org-2/media/m9/encrypted.bin":          true,
	}, events: events}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewMediaService(repo, nil, nil, storage, nil, nil, logger), repo, storage, events
}

func TestMediaDeleteRemovesStoredObjectsBeforeDatabasePurge(t *testing.T) {
	svc, _, storage, events := newPurgeFixture()

	if err := svc.Delete(context.Background(), "org-1", "m1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	want := []string{
		"storage-delete:orgs/org-1/media/m1/encrypted.bin",
		"storage-delete:orgs/org-1/recordings/s1/chunk-000.bin.enc",
		"db-purge:m1",
	}
	if !slices.Equal(*events, want) {
		t.Fatalf("events = %v, want %v (missing chunk-001 counts as already deleted)", *events, want)
	}
	if !storage.objects["orgs/org-2/media/m9/encrypted.bin"] {
		t.Fatal("another organization's object was deleted")
	}
}

func TestMediaDeleteKeepsRowsWhenStorageDeleteFails(t *testing.T) {
	svc, _, storage, events := newPurgeFixture()
	storage.failKey = "orgs/org-1/recordings/s1/chunk-000.bin.enc"

	if err := svc.Delete(context.Background(), "org-1", "m1"); !errors.Is(err, errStorageDown) {
		t.Fatalf("Delete() error = %v, want storage error", err)
	}
	if slices.Contains(*events, "db-purge:m1") {
		t.Fatalf("database purge ran after a storage failure: %v", *events)
	}

	storage.failKey = ""
	if err := svc.Delete(context.Background(), "org-1", "m1"); err != nil {
		t.Fatalf("retry Delete() error = %v", err)
	}
	if (*events)[len(*events)-1] != "db-purge:m1" {
		t.Fatalf("retry did not finish with the database purge: %v", *events)
	}
}

func TestMediaDeleteRejectsOtherOrganizationAndRepurgesDeletedMedia(t *testing.T) {
	svc, repo, _, events := newPurgeFixture()
	if err := svc.Delete(context.Background(), "org-2", "m1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-org Delete() error = %v, want ErrNotFound", err)
	}
	if len(*events) != 0 {
		t.Fatalf("cross-org delete touched data: %v", *events)
	}

	repo.media["m1"].Status = domain.MediaStatusDeleted
	repo.media["m1"].StorageKey = ""
	repo.chunkKeys = nil
	if err := svc.Delete(context.Background(), "org-1", "m1"); err != nil {
		t.Fatalf("Delete() of soft-deleted media error = %v", err)
	}
	if !slices.Equal(*events, []string{"db-purge:m1"}) {
		t.Fatalf("events = %v, want the purge to re-run for a soft-deleted row", *events)
	}
}

func TestCheckDurationEnforcesMediaLimit(t *testing.T) {
	svc := NewMediaService(nil, nil, nil, nil, nil, nil, nil, WithMaxMediaDuration(time.Hour))
	if err := svc.CheckDuration(3600); err != nil {
		t.Fatalf("CheckDuration(limit) error = %v", err)
	}
	if err := svc.CheckDuration(0); err != nil {
		t.Fatalf("CheckDuration(unknown) error = %v", err)
	}
	if err := svc.CheckDuration(3601); !errors.Is(err, domain.ErrMediaTooLong) {
		t.Fatalf("CheckDuration(over) error = %v, want ErrMediaTooLong", err)
	}
	if err := svc.CheckStitchedDuration(3600 + 60); err != nil {
		t.Fatalf("CheckStitchedDuration(within grace) error = %v", err)
	}
	over := (time.Hour + domain.MediaDurationGrace).Seconds() + 1
	if err := svc.CheckStitchedDuration(over); !errors.Is(err, domain.ErrMediaTooLong) {
		t.Fatalf("CheckStitchedDuration(over grace) error = %v, want ErrMediaTooLong", err)
	}
	if err := NewMediaService(nil, nil, nil, nil, nil, nil, nil).CheckDuration(domain.DefaultMaxMediaDuration.Seconds()); err != nil {
		t.Fatalf("default limit rejected a recording of exactly %s: %v", domain.DefaultMaxMediaDuration, err)
	}
}

// retentionRecordingRepo returns one due live-recording candidate.
type retentionRecordingRepo struct {
	port.RecordingRepository
	candidates []port.LiveRecordingAudioRetentionCandidate
}

func (r *retentionRecordingRepo) ListLiveRecordingAudioRetentionCandidates(context.Context, time.Time, int) ([]port.LiveRecordingAudioRetentionCandidate, error) {
	return r.candidates, nil
}

// TestRetentionDeletesAudioFileThenClearsKeys pins the retention semantics:
// only the audio object goes (then the key references), transcripts stay.
func TestRetentionDeletesAudioFileThenClearsKeys(t *testing.T) {
	media, repo, storage, events := newPurgeFixture()
	recordings := &retentionRecordingRepo{candidates: []port.LiveRecordingAudioRetentionCandidate{{
		SessionID: "s1", MediaID: "m1", OrganizationID: "org-1",
		StorageKey: "orgs/org-1/media/m1/encrypted.bin", CompletedAt: time.Now().Add(-8 * 24 * time.Hour),
	}}}
	svc := NewRecordingService(recordings, nil, media, storage, nil, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	result, err := svc.CleanupRetainedAudio(context.Background())
	if err != nil || result.Deleted != 1 {
		t.Fatalf("CleanupRetainedAudio() = %+v, %v", result, err)
	}
	want := []string{
		"storage-delete:orgs/org-1/media/m1/encrypted.bin",
		"mark-audio-deleted:orgs/org-1/media/m1/encrypted.bin",
	}
	if !slices.Equal(*events, want) {
		t.Fatalf("events = %v, want %v", *events, want)
	}
	if slices.Contains(*events, "db-purge:m1") || len(repo.marked) != 1 {
		t.Fatalf("retention must clear audio only, not purge transcripts: %v", *events)
	}
}
