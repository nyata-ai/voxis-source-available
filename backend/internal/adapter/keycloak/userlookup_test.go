package keycloak

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/domain"
)

const (
	testRealm   = "voxis-oss"
	testSubject = "3f1c2b8e-0000-4000-8000-000000000001"
)

// fakeKeycloak serves the token endpoint and the two admin endpoints used by
// UserLookup, and counts calls to each.
type fakeKeycloak struct {
	mu            sync.Mutex
	tokenCalls    int
	userCalls     int
	roleCalls     int
	tokenStatus   int
	userStatus    int
	roleStatus    int
	enabled       bool
	roles         []string
	lastGrant     string
	lastClientID  string
	lastSecret    string
	lastAuthToken string
}

func newFakeKeycloak() *fakeKeycloak {
	return &fakeKeycloak{tokenStatus: http.StatusOK, userStatus: http.StatusOK, roleStatus: http.StatusOK,
		enabled: true, roles: []string{"offline_access", "voxis-user"}}
}

func (f *fakeKeycloak) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	userPath := "/auth/admin/realms/" + testRealm + "/users/" + testSubject
	switch r.URL.Path {
	case "/auth/realms/" + testRealm + "/protocol/openid-connect/token":
		f.tokenCalls++
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.lastGrant, f.lastClientID, f.lastSecret = r.PostForm.Get("grant_type"), r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
		writeJSON(w, f.tokenStatus, map[string]any{"access_token": "svc-token-" + strconv.Itoa(f.tokenCalls), "expires_in": 300})
	case userPath:
		f.userCalls++
		f.lastAuthToken = r.Header.Get("Authorization")
		writeJSON(w, f.userStatus, map[string]any{"id": testSubject, "enabled": f.enabled})
	case userPath + "/role-mappings/realm/composite":
		f.roleCalls++
		roles := make([]map[string]string, 0, len(f.roles))
		for _, name := range f.roles {
			roles = append(roles, map[string]string{"name": name})
		}
		writeJSON(w, f.roleStatus, roles)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status == http.StatusOK {
		_ = json.NewEncoder(w).Encode(body) //nolint:errcheck // test server
	}
}

func (f *fakeKeycloak) counts() (token, user, role int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls, f.userCalls, f.roleCalls
}

func (f *fakeKeycloak) set(update func(*fakeKeycloak)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(f)
}

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time          { return c.now }
func (c *testClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestLookup(t *testing.T, fake *fakeKeycloak) (*UserLookup, *testClock) {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	lookup, err := NewUserLookup(Config{
		BaseURL: server.URL + "/auth/", Realm: testRealm, ClientID: "voxis-oss-api-lookup",
		ClientSecret: "lookup-secret", RequiredRole: "voxis-user", HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	clock := &testClock{now: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)}
	lookup.now = clock.Now
	return lookup, clock
}

func TestVerifyOwner_AllowsEnabledUserWithRoleAndCaches(t *testing.T) {
	fake := newFakeKeycloak()
	lookup, _ := newTestLookup(t, fake)

	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject))
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject))

	token, user, role := fake.counts()
	assert.Equal(t, 1, token, "service token is reused")
	assert.Equal(t, 1, user, "second call is served from cache")
	assert.Equal(t, 1, role)
	var grant, clientID, secret, authorization string
	fake.set(func(f *fakeKeycloak) {
		grant, clientID, secret, authorization = f.lastGrant, f.lastClientID, f.lastSecret, f.lastAuthToken
	})
	assert.Equal(t, "client_credentials", grant)
	assert.Equal(t, "voxis-oss-api-lookup", clientID)
	assert.Equal(t, "lookup-secret", secret)
	assert.Equal(t, "Bearer svc-token-1", authorization)
}

func TestVerifyOwner_DeniesDisabledMissingOrRolelessUser(t *testing.T) {
	cases := map[string]func(*fakeKeycloak){
		"disabled":     func(f *fakeKeycloak) { f.enabled = false },
		"deleted":      func(f *fakeKeycloak) { f.userStatus = http.StatusNotFound },
		"role removed": func(f *fakeKeycloak) { f.roles = []string{"offline_access"} },
		"admin only":   func(f *fakeKeycloak) { f.roles = []string{"voxis-admin"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeKeycloak()
			fake.set(mutate)
			lookup, _ := newTestLookup(t, fake)

			err := lookup.VerifyOwner(context.Background(), testSubject)

			require.ErrorIs(t, err, domain.ErrUnauthorized)
		})
	}
}

func TestVerifyOwner_ReflectsChangesAfterCacheTTL(t *testing.T) {
	fake := newFakeKeycloak()
	lookup, clock := newTestLookup(t, fake)
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject))

	fake.set(func(f *fakeKeycloak) { f.enabled = false })
	clock.Advance(resultCacheTTL - time.Second)
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject), "cached within the TTL")

	clock.Advance(2 * time.Second)
	require.ErrorIs(t, lookup.VerifyOwner(context.Background(), testSubject), domain.ErrUnauthorized)
}

func TestVerifyOwner_FailsClosedAndDoesNotCacheErrors(t *testing.T) {
	fake := newFakeKeycloak()
	fake.set(func(f *fakeKeycloak) { f.userStatus = http.StatusInternalServerError })
	lookup, _ := newTestLookup(t, fake)

	err := lookup.VerifyOwner(context.Background(), testSubject)
	require.Error(t, err)
	require.NotErrorIs(t, err, domain.ErrUnauthorized, "an outage is reported as an error, not a denial")

	fake.set(func(f *fakeKeycloak) { f.userStatus = http.StatusOK })
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject), "the failure was not cached")
}

func TestVerifyOwner_TokenEndpointFailureIsAnError(t *testing.T) {
	fake := newFakeKeycloak()
	fake.set(func(f *fakeKeycloak) { f.tokenStatus = http.StatusUnauthorized })
	lookup, _ := newTestLookup(t, fake)

	require.Error(t, lookup.VerifyOwner(context.Background(), testSubject))
	_, user, _ := fake.counts()
	assert.Zero(t, user, "no admin call without a service token")
}

func TestVerifyOwner_ForbiddenAdminCallDropsServiceToken(t *testing.T) {
	fake := newFakeKeycloak()
	fake.set(func(f *fakeKeycloak) { f.roleStatus = http.StatusForbidden })
	lookup, _ := newTestLookup(t, fake)

	require.Error(t, lookup.VerifyOwner(context.Background(), testSubject))
	fake.set(func(f *fakeKeycloak) { f.roleStatus = http.StatusOK })
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject))

	token, _, _ := fake.counts()
	assert.Equal(t, 2, token, "a 403 forces a fresh service token")
}

func TestVerifyOwner_RefreshesServiceTokenBeforeExpiry(t *testing.T) {
	fake := newFakeKeycloak()
	lookup, clock := newTestLookup(t, fake)
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject))

	clock.Advance(300*time.Second - tokenRefreshMargin)
	require.NoError(t, lookup.VerifyOwner(context.Background(), testSubject))

	token, user, _ := fake.counts()
	assert.Equal(t, 2, token)
	assert.Equal(t, 2, user)
}

func TestVerifyOwner_RejectsInvalidSubjectWithoutCallingKeycloak(t *testing.T) {
	fake := newFakeKeycloak()
	lookup, _ := newTestLookup(t, fake)

	require.ErrorIs(t, lookup.VerifyOwner(context.Background(), ""), domain.ErrUnauthorized)
	token, user, _ := fake.counts()
	assert.Zero(t, token+user)
}

func TestUserLookupCacheIsBounded(t *testing.T) {
	lookup, err := NewUserLookup(Config{BaseURL: "http://keycloak:8080/auth", Realm: testRealm,
		ClientID: "id", ClientSecret: "secret", RequiredRole: "voxis-user"})
	require.NoError(t, err)
	for i := range maxCachedSubjects + 5 {
		lookup.store("subject-"+strconv.Itoa(i), true)
	}
	assert.LessOrEqual(t, len(lookup.cache), maxCachedSubjects)
	_, ok := lookup.cached("subject-" + strconv.Itoa(maxCachedSubjects+4))
	assert.True(t, ok, "the newest entry survives eviction")
}

func TestNewUserLookupValidatesConfig(t *testing.T) {
	valid := Config{BaseURL: "http://keycloak:8080/auth", Realm: testRealm, ClientID: "id", ClientSecret: "secret", RequiredRole: "voxis-user"}
	_, err := NewUserLookup(valid)
	require.NoError(t, err)

	for name, mutate := range map[string]func(*Config){
		"relative URL":   func(c *Config) { c.BaseURL = "keycloak/auth" },
		"missing secret": func(c *Config) { c.ClientSecret = "" },
		"missing role":   func(c *Config) { c.RequiredRole = "" },
		"missing realm":  func(c *Config) { c.Realm = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			mutate(&cfg)
			_, err := NewUserLookup(cfg)
			require.Error(t, err)
		})
	}
}
