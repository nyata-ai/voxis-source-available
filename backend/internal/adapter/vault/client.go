package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Config holds Vault client configuration.
type Config struct {
	Address   string        // Vault address (e.g., https://vault.example.com:8200)
	Token     string        // Vault token (direct auth)
	RoleID    string        // AppRole role ID (alternative to Token)
	SecretID  string        // AppRole secret ID (alternative to Token)
	MountPath string        // Transit secrets engine mount path (default: transit)
	Timeout   time.Duration // Request timeout (default: 30s)
	CacheTTL  time.Duration // KEK cache lifetime (default: 5m; KEK_CACHE_TTL)
}

// Client is a HashiCorp Vault client implementing port.VaultClient.
type Client struct {
	client    *api.Client
	mountPath string
	logger    *slog.Logger

	// AppRole credentials for re-authentication after max TTL expiry.
	roleID   string
	secretID string

	// Goroutine lifecycle for token renewal.
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Simple in-memory cache for KEKs
	mu       sync.RWMutex
	cache    map[string]cacheEntry
	cacheTTL time.Duration
}

type cacheEntry struct {
	key       []byte
	expiresAt time.Time
}

const (
	defaultMountPath = "transit"
	defaultTimeout   = 30 * time.Second
	cacheExpiry      = 5 * time.Minute
)

// NewClient creates a new Vault client.
//
//nolint:gocyclo // factory function with auth strategy selection
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("vault address is required")
	}

	// Validate auth: either Token or (RoleID+SecretID) must be provided.
	hasToken := cfg.Token != ""
	hasRoleID := cfg.RoleID != ""
	hasSecretID := cfg.SecretID != ""

	switch {
	case hasToken && (hasRoleID || hasSecretID):
		return nil, fmt.Errorf("ambiguous auth: set either Token or (RoleID + SecretID), not both")
	case !hasToken && !hasRoleID && !hasSecretID:
		return nil, fmt.Errorf("no auth method configured: set Token or (RoleID + SecretID)")
	case !hasToken && hasRoleID && !hasSecretID:
		return nil, fmt.Errorf("incomplete AppRole config: role_id set but secret_id missing")
	case !hasToken && !hasRoleID && hasSecretID:
		return nil, fmt.Errorf("incomplete AppRole config: secret_id set but role_id missing")
	}

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = defaultMountPath
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = cacheExpiry
	}

	// api.DefaultConfig() reads VAULT_CACERT, VAULT_CLIENT_CERT, etc. natively.
	vaultCfg := api.DefaultConfig()
	vaultCfg.Address = cfg.Address
	vaultCfg.Timeout = timeout

	client, err := api.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Client{
		client:    client,
		mountPath: mountPath,
		logger:    logger,
		roleID:    cfg.RoleID,
		secretID:  cfg.SecretID,
		cancel:    cancel,
		cache:     make(map[string]cacheEntry),
		cacheTTL:  cacheTTL,
	}

	if hasToken {
		// Direct token auth — set and done.
		client.SetToken(cfg.Token)
	} else {
		// AppRole auth — perform initial login.
		secret, loginErr := c.loginAppRole(ctx)
		if loginErr != nil {
			cancel()
			return nil, fmt.Errorf("AppRole initial login: %w", loginErr)
		}
		c.startTokenRenewal(ctx, secret)
	}

	return c, nil
}

// loginAppRole authenticates via AppRole and sets the client token.
func (c *Client) loginAppRole(ctx context.Context) (*api.Secret, error) {
	secret, err := c.client.Logical().WriteWithContext(ctx, "auth/approle/login", map[string]interface{}{
		"role_id":   c.roleID,
		"secret_id": c.secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("approle login: %w", err)
	}
	if secret == nil || secret.Auth == nil {
		return nil, fmt.Errorf("approle login returned no auth data")
	}

	c.client.SetToken(secret.Auth.ClientToken)
	c.logger.Info("AppRole login successful",
		"token_ttl", time.Duration(secret.Auth.LeaseDuration)*time.Second,
		"renewable", secret.Auth.Renewable,
	)
	return secret, nil
}

// startTokenRenewal runs a background goroutine that renews the token at 50% TTL
// and re-authenticates when renewal fails (max TTL reached).
func (c *Client) startTokenRenewal(ctx context.Context, initialSecret *api.Secret) {
	ttl := time.Duration(initialSecret.Auth.LeaseDuration) * time.Second
	if ttl <= 0 {
		c.logger.Warn("AppRole token has zero TTL, skipping renewal")
		return
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		renewAt := ttl / 2
		backoff := time.Second

		timer := time.NewTimer(renewAt)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				// Try to renew the existing token.
				renewed, err := c.client.Auth().Token().RenewSelfWithContext(ctx, 0)
				if err == nil && renewed != nil && renewed.Auth != nil {
					newTTL := time.Duration(renewed.Auth.LeaseDuration) * time.Second
					c.logger.Debug("token renewed", "new_ttl", newTTL)
					renewAt = newTTL / 2
					backoff = time.Second
					timer.Reset(renewAt)
					continue
				}

				// Renewal failed (likely max TTL reached) — re-authenticate.
				c.logger.Warn("token renewal failed, re-authenticating via AppRole", "error", err)
				secret, loginErr := c.loginAppRole(ctx)
				if loginErr != nil {
					c.logger.Error("AppRole re-authentication failed", "error", loginErr, "retry_in", backoff)
					timer.Reset(backoff)
					backoff = min(backoff*2, 5*time.Minute)
					continue
				}

				newTTL := time.Duration(secret.Auth.LeaseDuration) * time.Second
				renewAt = newTTL / 2
				backoff = time.Second
				timer.Reset(renewAt)
			}
		}
	}()
}

// Ping checks if Vault is reachable and healthy (server-level status).
// It does NOT validate the current token — only that the Vault process is
// initialized and unsealed.
func (c *Client) Ping(ctx context.Context) error {
	health, err := c.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("vault health check failed: %w", err)
	}
	if !health.Initialized {
		return fmt.Errorf("vault is not initialized")
	}
	if health.Sealed {
		return fmt.Errorf("vault is sealed")
	}
	return nil
}

// GenerateKEK creates a new Key Encryption Key for an organization.
// Uses Vault's transit secrets engine to create a named encryption key.
func (c *Client) GenerateKEK(ctx context.Context, orgID string) (string, error) {
	if orgID == "" {
		return "", domain.ErrInvalidInput
	}

	keyName := "org-" + orgID
	keyPath := fmt.Sprintf("%s/keys/%s", c.mountPath, keyName)

	// Create a new encryption key in transit engine
	_, err := c.client.Logical().WriteWithContext(ctx, keyPath, map[string]interface{}{
		"type":       "aes256-gcm96",
		"exportable": true, // We need to export keys for client-side encryption
	})
	if err != nil {
		return "", fmt.Errorf("create transit key: %w", err)
	}

	c.logger.Info("generated KEK in Vault", "org_id", orgID, "key_path", keyPath)
	return keyPath, nil
}

// GetKEK retrieves an organization's KEK from Vault.
func (c *Client) GetKEK(ctx context.Context, orgID string) ([]byte, error) {
	if orgID == "" {
		return nil, domain.ErrInvalidInput
	}

	// Check cache first
	c.mu.RLock()
	entry, ok := c.cache[orgID]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		// Return a copy to prevent callers from corrupting the cache
		keyCopy := make([]byte, len(entry.key))
		copy(keyCopy, entry.key)
		return keyCopy, nil
	}

	// Export key from Vault
	keyName := "org-" + orgID
	exportPath := fmt.Sprintf("%s/export/encryption-key/%s", c.mountPath, keyName)

	secret, err := c.client.Logical().ReadWithContext(ctx, exportPath)
	if err != nil {
		return nil, fmt.Errorf("export transit key: %w", err)
	}
	if secret == nil || secret.Data == nil {
		return nil, domain.ErrNotFound
	}

	// Get the latest key version
	keys, ok := secret.Data["keys"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected key format from Vault")
	}

	// Find the highest version by parsing numeric keys
	var latestKey string
	highestVersion := -1
	for k, v := range keys {
		ver, parseErr := strconv.Atoi(k)
		if parseErr != nil {
			continue
		}
		if ver > highestVersion {
			if keyStr, ok := v.(string); ok {
				highestVersion = ver
				latestKey = keyStr
			}
		}
	}

	if latestKey == "" {
		return nil, fmt.Errorf("no key found in Vault response")
	}

	// Decode base64 key
	keyBytes, err := base64.StdEncoding.DecodeString(latestKey)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}

	// Cache a copy of the key
	cachedCopy := make([]byte, len(keyBytes))
	copy(cachedCopy, keyBytes)
	c.mu.Lock()
	c.cache[orgID] = cacheEntry{
		key:       cachedCopy,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.mu.Unlock()

	return keyBytes, nil
}

// GetKEKVersions retrieves all KEK versions for an organization from Vault,
// ordered newest-first. This bypasses the cache because it must return all
// versions, not just the latest.
func (c *Client) GetKEKVersions(ctx context.Context, orgID string) ([][]byte, error) {
	if orgID == "" {
		return nil, domain.ErrInvalidInput
	}

	keyName := "org-" + orgID
	exportPath := fmt.Sprintf("%s/export/encryption-key/%s", c.mountPath, keyName)

	secret, err := c.client.Logical().ReadWithContext(ctx, exportPath)
	if err != nil {
		return nil, fmt.Errorf("export transit key: %w", err)
	}
	if secret == nil || secret.Data == nil {
		return nil, domain.ErrNotFound
	}

	keys, ok := secret.Data["keys"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected key format from Vault")
	}

	// Collect all versions with their numeric keys for sorting.
	type versionedKey struct {
		version int
		key     []byte
	}
	var versions []versionedKey
	for k, v := range keys {
		ver, parseErr := strconv.Atoi(k)
		if parseErr != nil {
			continue
		}
		keyStr, ok := v.(string)
		if !ok {
			continue
		}
		keyBytes, decErr := base64.StdEncoding.DecodeString(keyStr)
		if decErr != nil {
			continue
		}
		versions = append(versions, versionedKey{version: ver, key: keyBytes})
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no keys found in Vault response")
	}

	// Sort newest-first (descending by version number).
	for i := 0; i < len(versions); i++ {
		for j := i + 1; j < len(versions); j++ {
			if versions[j].version > versions[i].version {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}

	result := make([][]byte, len(versions))
	for i, v := range versions {
		result[i] = v.key
	}

	return result, nil
}

// Close cleanly shuts down the client, stopping token renewal and zeroizing
// all cached KEK material.
func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()

	c.mu.Lock()
	for orgID, entry := range c.cache {
		clear(entry.key)
		delete(c.cache, orgID)
	}
	c.mu.Unlock()

	c.roleID = ""
	c.secretID = ""
	c.client.ClearToken()
	return nil
}

var _ port.VaultClient = (*Client)(nil)
