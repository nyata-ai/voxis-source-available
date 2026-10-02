package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/service"
)

func TestConfigureAPIKeyOwnerCheck(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	newCfg := func(environment, clientID string) *config.OSSConfig {
		cfg := &config.OSSConfig{}
		cfg.Environment = environment
		cfg.Auth = config.AuthConfig{
			KeycloakURL: "https://voxis.example.invalid/auth", KeycloakFetchURL: "http://keycloak:8080/auth",
			Realm: "voxis-oss", UserLookupClientID: clientID, UserLookupClientSecret: "lookup-secret",
		}
		return cfg
	}

	require.NoError(t, configureAPIKeyOwnerCheck(newCfg("development", ""), service.NewAPIKeyService(nil, logger), logger),
		"development may run without the lookup client")
	require.Error(t, configureAPIKeyOwnerCheck(newCfg("production", ""), service.NewAPIKeyService(nil, logger), logger),
		"production refuses to start without the lookup client")
	require.NoError(t, configureAPIKeyOwnerCheck(newCfg("production", "voxis-oss-api-lookup"), service.NewAPIKeyService(nil, logger), logger))
}
