package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps pgxpool.Pool with health checking.
type Pool struct {
	*pgxpool.Pool
}

// PoolConfig holds configuration for the database pool.
type PoolConfig struct {
	DatabaseURL     string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	// StatementTimeout and IdleInTxnTimeout are server-side backstops set as
	// session parameters on every pooled connection. Without them a wedged
	// query holds its connection for the whole job deadline (up to 2h for a
	// stitch job), and the worker fleet outnumbers the pool by far. Zero
	// leaves the server default (no timeout) in place.
	StatementTimeout time.Duration
	IdleInTxnTimeout time.Duration
}

// DefaultPoolConfig returns sensible defaults for the connection pool.
func DefaultPoolConfig(databaseURL string) PoolConfig {
	return PoolConfig{
		DatabaseURL:     databaseURL,
		MaxConns:        20,
		MinConns:        5,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
	}
}

// buildPoolConfig turns a PoolConfig into a pgxpool.Config. Kept pure and
// separate from NewPool so the session-parameter rules are testable without a
// database.
func buildPoolConfig(cfg PoolConfig) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	// Configure for PgBouncer compatibility and performance
	config.MaxConns = cfg.MaxConns
	config.MinConns = cfg.MinConns
	config.MaxConnLifetime = cfg.MaxConnLifetime
	config.MaxConnIdleTime = cfg.MaxConnIdleTime
	config.HealthCheckPeriod = time.Minute

	// Session parameters travel with the startup packet, so every connection
	// the pool opens carries them. Postgres expects milliseconds, and reads 0
	// as "no timeout" — hence the > 0 guard on the rounded value, not on the
	// duration.
	if ms := cfg.StatementTimeout.Milliseconds(); ms > 0 {
		config.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(ms, 10)
	}
	if ms := cfg.IdleInTxnTimeout.Milliseconds(); ms > 0 {
		config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(ms, 10)
	}

	return config, nil
}

// NewPool creates a new PostgreSQL connection pool.
func NewPool(ctx context.Context, cfg PoolConfig) (*Pool, error) {
	config, err := buildPoolConfig(cfg)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// Ping implements port.HealthChecker.
func (p *Pool) Ping(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}
