package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// lastUsedWriteInterval coalesces last_used_at writes: an API key hammered on
// every request should not fire a DB write per request.
const lastUsedWriteInterval = time.Minute

// maxLastUsedTrackedKeys bounds lastUsedAt. Keys are capped per org
// (domain.MaxAPIKeysPerOrg), so this is only a safety net.
const maxLastUsedTrackedKeys = 10_000

// APIKeyService handles API key lifecycle operations.
type APIKeyService struct {
	repo   port.APIKeyRepository
	logger *slog.Logger
	// ownerVerifier, when set, makes every Authenticate confirm that the
	// key's creator can still use Voxis. Nil only in development.
	ownerVerifier port.APIKeyOwnerVerifier

	lastUsedMu sync.Mutex
	lastUsedAt map[string]time.Time
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(repo port.APIKeyRepository, logger *slog.Logger) *APIKeyService {
	if logger == nil {
		logger = slog.Default()
	}
	return &APIKeyService{
		repo:       repo,
		logger:     logger,
		lastUsedAt: make(map[string]time.Time),
	}
}

// SetOwnerVerifier installs the account check applied on every API key
// authentication. Call it during startup, before serving requests.
func (s *APIKeyService) SetOwnerVerifier(verifier port.APIKeyOwnerVerifier) {
	s.ownerVerifier = verifier
}

// Create generates a new API key for the organization.
// Returns the full key (only shown once), the stored key metadata, and any error.
// Retries up to 3 times on key_prefix collision (ErrConflict from repo).
func (s *APIKeyService) Create(
	ctx context.Context,
	orgID, createdBy, name string,
	scopes []string,
	expiresAt *time.Time,
) (string, *domain.APIKey, error) {
	// Validate inputs
	if name == "" {
		return "", nil, fmt.Errorf("name is required: %w", domain.ErrInvalidInput)
	}
	if len(name) > 100 {
		return "", nil, fmt.Errorf("name must be at most 100 characters: %w", domain.ErrInvalidInput)
	}
	if err := domain.ValidateScopes(scopes); err != nil {
		return "", nil, fmt.Errorf("%s: %w", err.Error(), domain.ErrInvalidInput)
	}

	// Check org key count limit
	count, err := s.repo.CountByOrg(ctx, orgID)
	if err != nil {
		return "", nil, fmt.Errorf("count org keys: %w", err)
	}
	if count >= domain.MaxAPIKeysPerOrg {
		return "", nil, fmt.Errorf("organization has reached the maximum of %d API keys: %w",
			domain.MaxAPIKeysPerOrg, domain.ErrConflict)
	}

	// Generate and store with retry on prefix collision
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		fullKey, prefix, hash, err := domain.GenerateAPIKey("live")
		if err != nil {
			return "", nil, fmt.Errorf("generate key: %w", err)
		}

		key := &domain.APIKey{
			ID:             uuid.New().String(),
			OrganizationID: orgID,
			CreatedBy:      createdBy,
			Name:           name,
			KeyPrefix:      prefix,
			KeyHash:        hash,
			Scopes:         scopes,
			ExpiresAt:      expiresAt,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if err := s.repo.Create(ctx, key); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				s.logger.Warn("API key prefix collision, retrying",
					"attempt", attempt+1, "org_id", orgID)
				continue
			}
			return "", nil, fmt.Errorf("store key: %w", err)
		}

		s.logger.Info("API key created",
			"key_id", key.ID, "org_id", orgID, "prefix", prefix)
		return fullKey, key, nil
	}

	return "", nil, fmt.Errorf("failed to generate unique key after %d attempts: %w",
		maxRetries, domain.ErrConflict)
}

// Authenticate validates an API key and returns the associated key metadata.
// Returns ErrUnauthorized for any authentication failure (invalid format, wrong key,
// revoked, expired, or not found).
func (s *APIKeyService) Authenticate(ctx context.Context, fullKey string) (*domain.APIKey, error) {
	prefix, err := domain.ParseKeyPrefix(fullKey)
	if err != nil {
		return nil, fmt.Errorf("invalid key format: %w", domain.ErrUnauthorized)
	}

	key, err := s.repo.GetByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("key not found: %w", domain.ErrUnauthorized)
		}
		return nil, fmt.Errorf("lookup key: %w", err)
	}

	if !domain.VerifyAPIKey(fullKey, key.KeyHash) {
		return nil, fmt.Errorf("key mismatch: %w", domain.ErrUnauthorized)
	}

	if key.IsExpired() {
		return nil, fmt.Errorf("key expired: %w", domain.ErrUnauthorized)
	}

	if key.IsRevoked() {
		return nil, fmt.Errorf("key revoked: %w", domain.ErrUnauthorized)
	}

	if err := s.verifyOwner(ctx, key); err != nil {
		return nil, err
	}

	// Fire-and-forget: update last_used_at, coalesced to at most once per
	// lastUsedWriteInterval per key (bounded timeout for graceful shutdown).
	if s.shouldWriteLastUsed(key.ID) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.repo.UpdateLastUsed(ctx, key.ID); err != nil {
				s.logger.Warn("failed to update API key last_used_at",
					"key_id", key.ID, "error", err)
			}
		}()
	}

	return key, nil
}

// verifyOwner fails closed: a key whose owner is disabled, deleted, or no
// longer holds the realm role is refused, and so is a key whose owner could
// not be checked (for example while the identity provider is unreachable).
func (s *APIKeyService) verifyOwner(ctx context.Context, key *domain.APIKey) error {
	if s.ownerVerifier == nil {
		return nil
	}
	err := s.ownerVerifier.VerifyOwner(ctx, key.CreatedBy)
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrUnauthorized) {
		return fmt.Errorf("key owner may not use Voxis: %w", err)
	}
	s.logger.Warn("API key owner check failed; refusing the key",
		"key_id", key.ID, "org_id", key.OrganizationID, "error", err)
	return fmt.Errorf("verify key owner: %w: %w", err, domain.ErrUnauthorized)
}

// shouldWriteLastUsed reports whether enough time has passed since the last
// recorded write for keyID to warrant another one, and records the attempt
// time before returning true so concurrent Authenticate calls don't both fire.
func (s *APIKeyService) shouldWriteLastUsed(keyID string) bool {
	now := time.Now()

	s.lastUsedMu.Lock()
	defer s.lastUsedMu.Unlock()

	if last, ok := s.lastUsedAt[keyID]; ok && now.Sub(last) < lastUsedWriteInterval {
		return false
	}
	if len(s.lastUsedAt) >= maxLastUsedTrackedKeys {
		s.lastUsedAt = make(map[string]time.Time)
	}
	s.lastUsedAt[keyID] = now
	return true
}

// List returns active API keys for an organization with hashes cleared.
func (s *APIKeyService) List(ctx context.Context, orgID string) ([]*domain.APIKey, error) {
	keys, err := s.repo.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	// Clear hashes from the response
	for _, k := range keys {
		k.KeyHash = ""
	}

	return keys, nil
}

// Revoke marks an API key as revoked.
func (s *APIKeyService) Revoke(ctx context.Context, id, orgID string) error {
	if err := s.repo.Revoke(ctx, id, orgID); err != nil {
		return err
	}

	s.logger.Info("API key revoked", "key_id", id, "org_id", orgID)
	return nil
}
