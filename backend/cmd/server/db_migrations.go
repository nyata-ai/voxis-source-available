package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver for database/sql.
	dbmigrations "github.com/voxis/backend/db/migrations"
	"github.com/voxis/backend/internal/config"
)

type appMigrator interface {
	Up() error
	Close() error
}

type migrationWrapper struct {
	inner *migrate.Migrate
}

func (m *migrationWrapper) Up() error {
	return m.inner.Up()
}

func (m *migrationWrapper) Close() error {
	sourceErr, databaseErr := m.inner.Close()
	return errors.Join(sourceErr, databaseErr)
}

func newAppMigrator(databaseURL string) (appMigrator, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open sql connection: %w", err)
	}

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("create postgres migration driver: %w", errors.Join(err, closeErr))
		}
		return nil, fmt.Errorf("create postgres migration driver: %w", err)
	}

	sourceDriver, err := iofs.New(dbmigrations.Files, ".")
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("create migration source: %w", errors.Join(err, closeErr))
		}
		return nil, fmt.Errorf("create migration source: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("create migration instance: %w", errors.Join(err, closeErr))
		}
		return nil, fmt.Errorf("create migration instance: %w", err)
	}

	return &migrationWrapper{inner: migrator}, nil
}

func runAppMigrations(
	cfg *config.OSSConfig,
	logger *slog.Logger,
	factory func(databaseURL string) (appMigrator, error),
) error {
	if cfg.Database.URL == "" {
		logger.Warn("database migrations skipped: DATABASE_URL not set")
		return nil
	}

	migrator, err := factory(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("create application migrator: %w", err)
	}

	upErr := migrator.Up()
	closeErr := migrator.Close()
	if closeErr != nil && upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return fmt.Errorf("run application migrations: %w", errors.Join(upErr, closeErr))
	}
	if closeErr != nil {
		return fmt.Errorf("close application migrator: %w", closeErr)
	}
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return fmt.Errorf("run application migrations: %w", upErr)
	}

	if errors.Is(upErr, migrate.ErrNoChange) {
		logger.Info("application schema already up to date")
		return nil
	}

	logger.Info("application schema migrations applied")
	return nil
}
