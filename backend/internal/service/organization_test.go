package service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

type fakeOrgRepo struct {
	port.OrganizationRepository
	created *domain.Organization
}

func (r *fakeOrgRepo) Create(_ context.Context, org *domain.Organization) error {
	org.ID = "org-1"
	r.created = org
	return nil
}

type fakeFirstLoginUserRepo struct {
	port.UserRepository
	created *domain.User
}

func (r *fakeFirstLoginUserRepo) Exists(context.Context, string) (bool, error) { return false, nil }

func (r *fakeFirstLoginUserRepo) Create(_ context.Context, user *domain.User) error {
	r.created = user
	return nil
}

// First login must not write the email, or the slug derived from its local
// part, to the logs.
func TestCreateUserWithOrg_DoesNotLogEmailDerivedData(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	orgs := &fakeOrgRepo{}
	users := &fakeFirstLoginUserRepo{}
	svc := NewOrganizationService(orgs, users, nil, logger)

	claims := &port.TokenClaims{Subject: "sub-1", Email: "jane.q.private@example.test", Name: "Jane"}
	user, org, err := svc.CreateUserWithOrg(context.Background(), claims)
	if err != nil {
		t.Fatalf("CreateUserWithOrg: %v", err)
	}
	if user == nil || org == nil || orgs.created == nil || users.created == nil {
		t.Fatal("expected the user and organization to be created")
	}
	if !strings.Contains(org.Slug, "jane-q-private") {
		t.Fatalf("test premise: slug %q should derive from the email local part", org.Slug)
	}

	out := logs.String()
	if !strings.Contains(out, "created organization") || !strings.Contains(out, "org_id=org-1") {
		t.Fatalf("expected the organization creation to be logged by ID, got:\n%s", out)
	}
	for _, leaked := range []string{"jane", org.Slug, "example.test"} {
		if strings.Contains(strings.ToLower(out), leaked) {
			t.Errorf("log output contains email-derived %q:\n%s", leaked, out)
		}
	}
}
