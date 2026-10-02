package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// OrganizationService handles user and organization lifecycle.
type OrganizationService struct {
	orgRepo                      port.OrganizationRepository
	userRepo                     port.UserRepository
	vault                        port.VaultClient
	regenerateMissingExistingKEK bool
	logger                       *slog.Logger
}

// OrganizationServiceOption configures an organization lifecycle safeguard.
type OrganizationServiceOption func(*OrganizationService)

// WithMissingExistingKEKRecovery controls the legacy development recovery
// behavior for a missing key of an existing organization. Persistent editions
// must leave this false: creating a replacement key would make retained
// ciphertext unrecoverable while appearing to repair the installation.
func WithMissingExistingKEKRecovery(enabled bool) OrganizationServiceOption {
	return func(s *OrganizationService) {
		s.regenerateMissingExistingKEK = enabled
	}
}

// NewOrganizationService creates a new OrganizationService.
// vault can be nil for development mode (KEK generation will be skipped).
func NewOrganizationService(
	orgRepo port.OrganizationRepository,
	userRepo port.UserRepository,
	vault port.VaultClient,
	logger *slog.Logger,
	opts ...OrganizationServiceOption,
) *OrganizationService {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &OrganizationService{
		orgRepo:  orgRepo,
		userRepo: userRepo,
		vault:    vault,
		// Preserve the commercial development path until its deployment policy
		// explicitly opts into fail-closed recovery. The OSS composition passes
		// WithMissingExistingKEKRecovery(false).
		regenerateMissingExistingKEK: true,
		logger:                       logger,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

// CreateUserWithOrg creates a new user with their personal organization.
// This is called on first login. If the user already exists, returns them.
// Returns the user and their organization to avoid redundant DB lookups.
func (s *OrganizationService) CreateUserWithOrg(
	ctx context.Context,
	claims *port.TokenClaims,
) (*domain.User, *domain.Organization, error) {
	if err := s.validateClaims(claims); err != nil {
		return nil, nil, err
	}

	// Check if user already exists (optimistic fast path)
	exists, err := s.userRepo.Exists(ctx, claims.Subject)
	if err != nil {
		return nil, nil, fmt.Errorf("check user exists: %w", err)
	}

	if exists {
		return s.handleExistingUser(ctx, claims)
	}

	return s.createNewUserWithOrg(ctx, claims)
}

func (s *OrganizationService) validateClaims(claims *port.TokenClaims) error {
	if claims.Subject == "" {
		return fmt.Errorf("missing keycloak subject in claims")
	}
	if claims.Email == "" {
		return fmt.Errorf("missing email in claims")
	}
	return nil
}

func (s *OrganizationService) handleExistingUser(ctx context.Context, claims *port.TokenClaims) (*domain.User, *domain.Organization, error) {
	user, org, err := s.userRepo.GetWithOrganization(ctx, claims.Subject)
	if err != nil {
		return nil, nil, fmt.Errorf("get existing user: %w", err)
	}

	// Sync profile from claims if changed
	if user.UpdateFromClaims(claims.Email, claims.Name) {
		if updateErr := s.userRepo.Update(ctx, user); updateErr != nil {
			s.logger.Warn("failed to sync user profile", "error", updateErr, "user_id", user.ID)
		}
	}

	// Ensure encryption key exists in vault (may be lost after restart
	// with in-memory vault). Previously encrypted data is already
	// inaccessible in that case; regenerating allows new operations.
	if err := s.ensureEncryptionKey(ctx, org); err != nil {
		return nil, nil, err
	}

	return user, org, nil
}

func (s *OrganizationService) createNewUserWithOrg(ctx context.Context, claims *port.TokenClaims) (*domain.User, *domain.Organization, error) {
	// Create organization
	org, err := domain.NewPersonalOrganization(claims.Email)
	if err != nil {
		return nil, nil, fmt.Errorf("create organization model: %w", err)
	}

	if err = s.orgRepo.Create(ctx, org); err != nil {
		return nil, nil, fmt.Errorf("persist organization: %w", err)
	}

	// The slug is derived from the email local part, so only the ID is logged.
	s.logger.Info("created organization", "org_id", org.ID)

	// Generate KEK in Vault (if configured)
	if setupErr := s.setupEncryptionKey(ctx, org); setupErr != nil {
		return nil, nil, setupErr
	}

	// Create user
	user, err := domain.NewUser(claims.Subject, org.ID, claims.Email, claims.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("create user model: %w", err)
	}

	if err = s.userRepo.Create(ctx, user); err != nil {
		// Race condition: another request created this user concurrently.
		// Fall back to the existing user path.
		if errors.Is(err, domain.ErrConflict) {
			s.logger.Info("concurrent user creation detected, falling back to existing user",
				"user_id", claims.Subject)
			return s.handleExistingUser(ctx, claims)
		}
		return nil, nil, fmt.Errorf("persist user: %w", err)
	}

	s.logger.Info("created user on first login", "user_id", user.ID, "org_id", org.ID)
	return user, org, nil
}

func (s *OrganizationService) setupEncryptionKey(ctx context.Context, org *domain.Organization) error {
	if s.vault == nil {
		s.logger.Warn("vault not configured, skipping KEK generation", "org_id", org.ID)
		return nil
	}

	keyID, err := s.vault.GenerateKEK(ctx, org.ID)
	if err != nil {
		s.logger.Error("failed to generate KEK for new org", "error", err, "org_id", org.ID)
		return fmt.Errorf("generate encryption key: %w", err)
	}

	if err = s.orgRepo.UpdateEncryptionKeyID(ctx, org.ID, keyID); err != nil {
		s.logger.Error("failed to update org encryption key ID", "error", err, "org_id", org.ID, "key_id", keyID)
		return fmt.Errorf("update encryption key reference: %w", err)
	}

	org.EncryptionKeyID = keyID
	return nil
}

// ensureEncryptionKey verifies the org's KEK exists in vault. A composition
// that enables legacy recovery may regenerate a missing key for its in-memory
// development vault. Persistent editions fail closed instead: a replacement
// key cannot decrypt media already stored for that organization.
func (s *OrganizationService) ensureEncryptionKey(ctx context.Context, org *domain.Organization) error {
	if s.vault == nil {
		return nil
	}

	_, err := s.vault.GetKEK(ctx, org.ID)
	if err == nil {
		return nil // KEK exists
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("check encryption key: %w", err)
	}
	if !s.regenerateMissingExistingKEK {
		return fmt.Errorf("existing organization encryption key is unavailable; restore the original key material before resuming writes: %w", err)
	}

	s.logger.Warn("encryption key missing from vault, regenerating",
		"org_id", org.ID,
		"hint", "previously encrypted data may be inaccessible")

	return s.setupEncryptionKey(ctx, org)
}

// GetUserWithOrg retrieves a user and their organization.
func (s *OrganizationService) GetUserWithOrg(
	ctx context.Context,
	userID string,
) (*domain.User, *domain.Organization, error) {
	return s.userRepo.GetWithOrganization(ctx, userID)
}
