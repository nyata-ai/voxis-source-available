// Package keycloak checks, through the Keycloak admin REST API, that the
// account behind an API key still exists, is enabled, and still holds the
// realm role that grants access to Voxis.
package keycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/voxis/backend/internal/domain"
)

const (
	// resultCacheTTL bounds how long a disabled account or a removed role can
	// keep an API key working.
	resultCacheTTL = 60 * time.Second
	// maxCachedSubjects bounds the per-subject result cache.
	maxCachedSubjects = 10_000
	// requestTimeout applies to each call to Keycloak.
	requestTimeout = 5 * time.Second
	// tokenRefreshMargin refreshes the service token before it expires.
	tokenRefreshMargin = 30 * time.Second
	// maxResponseBytes bounds every response body read from Keycloak.
	maxResponseBytes = 1 << 20
	// maxSubjectLength matches the users.id column.
	maxSubjectLength = 255
)

// Config identifies the realm and the confidential service-account client
// used for lookups. The client needs the realm-management "view-users" role.
type Config struct {
	// BaseURL is the Keycloak origin plus any path prefix, without
	// "/realms/<realm>" (for example http://keycloak:8080/auth).
	BaseURL      string
	Realm        string
	ClientID     string
	ClientSecret string
	// RequiredRole is the realm role the account must hold.
	RequiredRole string
	// HTTPClient is optional; tests inject one.
	HTTPClient *http.Client
}

// UserLookup implements port.APIKeyOwnerVerifier against Keycloak.
type UserLookup struct {
	cfg    Config
	client *http.Client
	now    func() time.Time

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time

	cacheMu sync.Mutex
	cache   map[string]cachedResult
}

type cachedResult struct {
	allowed bool
	expires time.Time
}

// NewUserLookup validates cfg and returns a lookup client.
func NewUserLookup(cfg Config) (*UserLookup, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("keycloak user lookup: base URL must be an absolute http(s) URL")
	}
	if cfg.Realm == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RequiredRole == "" {
		return nil, errors.New("keycloak user lookup: realm, client ID, client secret, and required role are required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 2 * requestTimeout,
			// A redirect from the admin API is never expected; refuse it
			// rather than forward the service token elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &UserLookup{cfg: cfg, client: client, now: time.Now, cache: make(map[string]cachedResult)}, nil
}

// VerifyOwner reports whether subject may use Voxis. Results, allowed or
// denied, are cached per subject for resultCacheTTL; failures are not cached.
func (l *UserLookup) VerifyOwner(ctx context.Context, subject string) error {
	if subject == "" || len(subject) > maxSubjectLength {
		return fmt.Errorf("invalid key owner subject: %w", domain.ErrUnauthorized)
	}
	allowed, ok := l.cached(subject)
	if !ok {
		var err error
		allowed, err = l.lookup(ctx, subject)
		if err != nil {
			return err
		}
		l.store(subject, allowed)
	}
	if !allowed {
		return fmt.Errorf("key owner is missing, disabled, or lacks the %s role: %w", l.cfg.RequiredRole, domain.ErrUnauthorized)
	}
	return nil
}

func (l *UserLookup) lookup(ctx context.Context, subject string) (bool, error) {
	token, err := l.serviceToken(ctx)
	if err != nil {
		return false, err
	}
	userPath := "/admin/realms/" + url.PathEscape(l.cfg.Realm) + "/users/" + url.PathEscape(subject)

	var user struct {
		Enabled bool `json:"enabled"`
	}
	found, err := l.getJSON(ctx, token, userPath, &user)
	if err != nil || !found || !user.Enabled {
		return false, err
	}

	var roles []struct {
		Name string `json:"name"`
	}
	found, err = l.getJSON(ctx, token, userPath+"/role-mappings/realm/composite?briefRepresentation=true", &roles)
	if err != nil || !found {
		return false, err
	}
	for _, role := range roles {
		if role.Name == l.cfg.RequiredRole {
			return true, nil
		}
	}
	return false, nil
}

// getJSON reads one admin API resource into out. It reports found=false for
// 404 and an error for every other non-200 status.
func (l *UserLookup) getJSON(ctx context.Context, token, path string, out any) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.cfg.BaseURL+path, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("build keycloak user lookup request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("keycloak user lookup: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // body fully read or abandoned

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		l.dropServiceToken()
		return false, fmt.Errorf("keycloak user lookup: status %d (the lookup client needs realm-management view-users)", resp.StatusCode)
	default:
		return false, fmt.Errorf("keycloak user lookup: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return false, fmt.Errorf("decode keycloak user lookup response: %w", err)
	}
	return true, nil
}

// serviceToken returns a cached client-credentials token, fetching a new one
// shortly before the old one expires. Holding tokenMu across the fetch keeps
// concurrent callers from stampeding the token endpoint.
func (l *UserLookup) serviceToken(ctx context.Context) (string, error) {
	l.tokenMu.Lock()
	defer l.tokenMu.Unlock()
	if l.token != "" && l.now().Before(l.tokenExpiry) {
		return l.token, nil
	}
	token, lifetime, err := l.fetchServiceToken(ctx)
	if err != nil {
		return "", err
	}
	l.token = token
	l.tokenExpiry = l.now().Add(lifetime - tokenRefreshMargin)
	return token, nil
}

func (l *UserLookup) dropServiceToken() {
	l.tokenMu.Lock()
	defer l.tokenMu.Unlock()
	l.token = ""
}

func (l *UserLookup) fetchServiceToken(ctx context.Context) (string, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {l.cfg.ClientID},
		"client_secret": {l.cfg.ClientSecret},
	}
	tokenURL := l.cfg.BaseURL + "/realms/" + url.PathEscape(l.cfg.Realm) + "/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build keycloak token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("keycloak token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // body fully read or abandoned
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("keycloak token request: status %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return "", 0, fmt.Errorf("decode keycloak token response: %w", err)
	}
	if body.AccessToken == "" || body.ExpiresIn <= 0 {
		return "", 0, errors.New("keycloak token response has no usable access token")
	}
	return body.AccessToken, time.Duration(body.ExpiresIn) * time.Second, nil
}

func (l *UserLookup) cached(subject string) (allowed, ok bool) {
	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()
	entry, found := l.cache[subject]
	if !found || !l.now().Before(entry.expires) {
		return false, false
	}
	return entry.allowed, true
}

func (l *UserLookup) store(subject string, allowed bool) {
	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()
	now := l.now()
	if _, exists := l.cache[subject]; !exists && len(l.cache) >= maxCachedSubjects {
		l.evictLocked(now)
	}
	l.cache[subject] = cachedResult{allowed: allowed, expires: now.Add(resultCacheTTL)}
}

// evictLocked drops expired entries and, if the cache is still full, the
// oldest live entry. Both loops are bounded by maxCachedSubjects.
func (l *UserLookup) evictLocked(now time.Time) {
	for subject, entry := range l.cache {
		if !now.Before(entry.expires) {
			delete(l.cache, subject)
		}
	}
	if len(l.cache) < maxCachedSubjects {
		return
	}
	oldest, oldestExpiry, found := "", time.Time{}, false
	for subject, entry := range l.cache {
		if !found || entry.expires.Before(oldestExpiry) {
			oldest, oldestExpiry, found = subject, entry.expires, true
		}
	}
	delete(l.cache, oldest)
}
