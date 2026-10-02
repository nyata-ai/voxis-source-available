package main

import (
	"fmt"
	"log/slog"

	"github.com/voxis/backend/internal/adapter/keycloak"
	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/handler/middleware"
	"github.com/voxis/backend/internal/service"
)

// configureAPIKeyOwnerCheck makes every API key authentication confirm, via
// Keycloak, that the key's creator is still enabled and still holds the
// voxis-user realm role. Configuration loading already requires the lookup
// client in production; development may run without it.
func configureAPIKeyOwnerCheck(cfg *config.OSSConfig, apiKeys *service.APIKeyService, logger *slog.Logger) error {
	if cfg.Auth.UserLookupClientID == "" {
		if cfg.Environment == "production" {
			return fmt.Errorf("KEYCLOAK_USER_LOOKUP_CLIENT_ID is required in production")
		}
		logger.Warn("KEYCLOAK_USER_LOOKUP_CLIENT_ID is not set: API keys keep working after their owner is disabled or loses the " +
			middleware.UserRole + " role (development only)")
		return nil
	}
	lookup, err := keycloak.NewUserLookup(keycloak.Config{
		BaseURL:      cfg.Auth.FetchBaseURL(),
		Realm:        cfg.Auth.Realm,
		ClientID:     cfg.Auth.UserLookupClientID,
		ClientSecret: cfg.Auth.UserLookupClientSecret,
		RequiredRole: middleware.UserRole,
	})
	if err != nil {
		return fmt.Errorf("create Keycloak user lookup: %w", err)
	}
	apiKeys.SetOwnerVerifier(lookup)
	return nil
}
