package system

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxis/backend/internal/port"
)

const (
	dependencyTimeout = 2 * time.Second
	dependencyBudget  = 3 * time.Second
	sectionTimeout    = 3 * time.Second
)

// Config supplies bounded probes for system health reporting.
type Config struct {
	Pool           *pgxpool.Pool
	StorageBackend string
	DepChecks      []DependencyCheck
	Now            func() time.Time
	Logger         *slog.Logger
}

// Collector gathers the configured system and dependency statistics.
type Collector struct {
	cfg  Config
	host *hostCollector
	ip   *ipResolver
}

// NewCollector creates a statistics collector from cfg.
func NewCollector(cfg Config) *Collector {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if len(cfg.DepChecks) == 0 {
		panic("system: at least one dependency check required")
	}
	return &Collector{cfg: cfg, host: newHostCollector(cfg.Now(), cfg.Now), ip: newIPResolver(os.Getenv)}
}

// GetSystemStats returns the latest bounded system and dependency snapshot.
func (c *Collector) GetSystemStats(ctx context.Context) (*port.SystemStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := c.cfg.Now().UTC()
	stats := &port.SystemStats{
		GeneratedAt:  now,
		Host:         c.hostStats(ctx),
		Build:        port.SystemBuildStats{Available: false, Reason: "build metadata unavailable"},
		Disk:         c.diskStats(),
		Storage:      port.SystemStorageStats{Available: c.cfg.StorageBackend == "localfs", Backend: c.cfg.StorageBackend},
		Database:     c.databaseStats(ctx),
		Users:        c.userStats(ctx, now),
		CreditExpiry: port.SystemCreditExpiryStats{Available: false, Reason: "billing is not included in Voxis Source-Available"},
		Email:        port.SystemEmailStats{Available: false, Reason: "transactional email is not included in Voxis Source-Available"},
		Dependencies: checkDependencies(ctx, c.cfg.DepChecks, c.cfg.Now, dependencyTimeout, dependencyBudget),
	}
	return stats, nil
}

func (c *Collector) hostStats(ctx context.Context) port.SystemHostStats {
	h := port.SystemHostStats{Available: true}
	h.IP, h.ExternalIP = c.ip.resolve(ctx)
	if v, err := c.host.memory(); err == nil {
		h.Memory = v
	} else {
		h.Available, h.Reason = false, err.Error()
	}
	if v, err := c.host.cpu(); err == nil {
		h.CPU = v
	} else {
		h.Available, h.Reason = false, err.Error()
	}
	if v, err := c.host.uptime(); err == nil {
		h.Uptime = v
	} else {
		h.Available, h.Reason = false, err.Error()
	}
	return h
}

func (c *Collector) diskStats() port.SystemDiskStats {
	v, err := c.host.disk("/")
	if err != nil {
		return port.SystemDiskStats{Available: false, Reason: err.Error(), Mount: "/"}
	}
	return v
}

func (c *Collector) databaseStats(ctx context.Context) port.SystemDatabaseStats {
	if c.cfg.Pool == nil {
		return port.SystemDatabaseStats{Available: false, Reason: "no database pool"}
	}
	stat := c.cfg.Pool.Stat()
	pool := port.SystemDBPool{Acquired: stat.AcquiredConns(), Idle: stat.IdleConns(), Max: stat.MaxConns(), Total: stat.TotalConns()}
	var size int64
	if err := c.cfg.Pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err != nil {
		return port.SystemDatabaseStats{Available: false, Reason: err.Error(), Pool: pool}
	}
	return port.SystemDatabaseStats{Available: true, SizeBytes: size, Pool: pool}
}

func (c *Collector) userStats(ctx context.Context, now time.Time) port.SystemUsersStats {
	if c.cfg.Pool == nil {
		return port.SystemUsersStats{Available: false, Reason: "no database pool"}
	}
	qctx, cancel := context.WithTimeout(ctx, sectionTimeout)
	defer cancel()
	var users, orgs, new7, new30 int64
	err := c.cfg.Pool.QueryRow(qctx, `SELECT (SELECT COUNT(*) FROM users), (SELECT COUNT(*) FROM organizations), (SELECT COUNT(*) FROM users WHERE created_at >= $1), (SELECT COUNT(*) FROM users WHERE created_at >= $2)`, now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour)).Scan(&users, &orgs, &new7, &new30)
	if err != nil {
		return port.SystemUsersStats{Available: false, Reason: err.Error()}
	}
	return port.SystemUsersStats{Available: true, Total: users, Organizations: orgs, New7d: new7, New30d: new30}
}

var _ port.SystemStatsCollector = (*Collector)(nil)
