package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/domain"
)

type ownerCheckKeyRepo struct {
	key *domain.APIKey
}

func (r *ownerCheckKeyRepo) Create(context.Context, *domain.APIKey) error { return nil }
func (r *ownerCheckKeyRepo) GetByPrefix(_ context.Context, prefix string) (*domain.APIKey, error) {
	if r.key == nil || r.key.KeyPrefix != prefix {
		return nil, domain.ErrNotFound
	}
	copied := *r.key
	return &copied, nil
}
func (r *ownerCheckKeyRepo) ListByOrg(context.Context, string) ([]*domain.APIKey, error) {
	return nil, nil
}
func (r *ownerCheckKeyRepo) CountByOrg(context.Context, string) (int, error) { return 0, nil }
func (r *ownerCheckKeyRepo) Revoke(context.Context, string, string) error    { return nil }
func (r *ownerCheckKeyRepo) UpdateLastUsed(context.Context, string) error    { return nil }

type stubOwnerVerifier struct {
	err      error
	subjects []string
}

func (v *stubOwnerVerifier) VerifyOwner(_ context.Context, subject string) error {
	v.subjects = append(v.subjects, subject)
	return v.err
}

func newOwnerCheckService(t *testing.T) (svc *APIKeyService, fullKey string) {
	t.Helper()
	fullKey, prefix, hash, err := domain.GenerateAPIKey("live")
	require.NoError(t, err)
	repo := &ownerCheckKeyRepo{key: &domain.APIKey{
		ID: "key-1", OrganizationID: "org-1", CreatedBy: "kc-subject-1",
		KeyPrefix: prefix, KeyHash: hash, Scopes: []string{"media:read"}, CreatedAt: time.Now(),
	}}
	return NewAPIKeyService(repo, nil), fullKey
}

func TestAuthenticate_ChecksOwnerBySubject(t *testing.T) {
	svc, fullKey := newOwnerCheckService(t)
	verifier := &stubOwnerVerifier{}
	svc.SetOwnerVerifier(verifier)

	key, err := svc.Authenticate(context.Background(), fullKey)

	require.NoError(t, err)
	assert.Equal(t, "key-1", key.ID)
	assert.Equal(t, []string{"kc-subject-1"}, verifier.subjects)
}

func TestAuthenticate_RefusesKeyOfDisabledOwner(t *testing.T) {
	svc, fullKey := newOwnerCheckService(t)
	svc.SetOwnerVerifier(&stubOwnerVerifier{err: fmt.Errorf("account disabled: %w", domain.ErrUnauthorized)})

	key, err := svc.Authenticate(context.Background(), fullKey)

	assert.Nil(t, key)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthenticate_FailsClosedWhenOwnerCheckErrors(t *testing.T) {
	svc, fullKey := newOwnerCheckService(t)
	lookupErr := errors.New("identity provider unreachable")
	svc.SetOwnerVerifier(&stubOwnerVerifier{err: lookupErr})

	key, err := svc.Authenticate(context.Background(), fullKey)

	assert.Nil(t, key)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	require.ErrorIs(t, err, lookupErr)
}

func TestAuthenticate_WithoutOwnerVerifierKeepsKeyWorking(t *testing.T) {
	svc, fullKey := newOwnerCheckService(t)

	key, err := svc.Authenticate(context.Background(), fullKey)

	require.NoError(t, err)
	assert.Equal(t, "kc-subject-1", key.CreatedBy)
}
