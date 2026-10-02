package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

// UsageHandler handles usage statistics HTTP endpoints.
type UsageHandler struct {
	usageRepo       port.UsageRepository
	orgService      *service.OrganizationService
	maxDailyURLUser int
	maxDailyURLOrg  int
	quotaRepo       port.UserStorageUsageRepository
	storageBackend  string
	logger          *slog.Logger
}

// UsageHandlerOption configures optional authenticated-user usage sections.
type UsageHandlerOption func(*UsageHandler)

// WithUserStorageQuota adds the per-user bucket-storage view to GET usage.
func WithUserStorageQuota(repo port.UserStorageUsageRepository, backend string) UsageHandlerOption {
	return func(h *UsageHandler) {
		h.quotaRepo = repo
		h.storageBackend = backend
	}
}

// NewUsageHandler creates a new usage handler.
// Panics if usageRepo or orgService is nil (programming error).
func NewUsageHandler(
	usageRepo port.UsageRepository,
	orgService *service.OrganizationService,
	maxDailyURLUser, maxDailyURLOrg int,
	logger *slog.Logger,
	opts ...UsageHandlerOption,
) *UsageHandler {
	if usageRepo == nil {
		panic("handler: usageRepo must not be nil")
	}
	if orgService == nil {
		panic("handler: orgService must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	h := &UsageHandler{
		usageRepo:       usageRepo,
		orgService:      orgService,
		maxDailyURLUser: maxDailyURLUser,
		maxDailyURLOrg:  maxDailyURLOrg,
		logger:          logger,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	return h
}

// GetStats handles GET /api/v1/usage/stats.
// Returns aggregated usage statistics for the authenticated user's organization.
func (h *UsageHandler) GetStats(c *gin.Context) {
	orgID, claims := resolveOrgIDOrAPIKey(c, h.orgService, h.logger)
	if orgID == "" {
		return // error already written by resolveOrgIDOrAPIKey
	}

	stats, err := h.usageRepo.GetStats(c.Request.Context(), orgID)
	if err != nil {
		h.writeStatsError(c, orgID, err)
		return
	}
	counts, err := h.usageRepo.GetURLTranscriptionCounts(c.Request.Context(), orgID, claims.Subject)
	if err != nil {
		h.writeStatsError(c, orgID, err)
		return
	}
	stats.URLTranscriptionsRemainingToday = min(
		max(int64(h.maxDailyURLUser)-counts.User, 0),
		max(int64(h.maxDailyURLOrg)-counts.Org, 0),
	)
	if err := h.addUserStorage(c.Request.Context(), stats, orgID, claims.Subject); err != nil {
		h.writeStatsError(c, orgID, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *UsageHandler) addUserStorage(ctx context.Context, stats *port.UsageStats, orgID, userID string) error {
	usage := port.UserStorageUsage{
		WarningThresholdPercent: domain.StorageQuotaWarningThresholdPercent,
		State:                   "unlimited",
	}
	if h.quotaRepo == nil || !domain.IsBucketStorageBackend(h.storageBackend) {
		stats.UserStorage = usage
		return nil
	}
	policy, err := h.quotaRepo.GetStorageQuotaPolicy(ctx)
	if err != nil {
		return err
	}
	used, err := h.quotaRepo.GetUserStorageUsedBytes(ctx, orgID, userID)
	if err != nil {
		return err
	}
	usage.UsedBytes = used
	usage.WarningThresholdPercent = policy.WarningThresholdPercent
	usage.Enforced = policy.Enabled
	if !policy.Enabled {
		stats.UserStorage = usage
		return nil
	}
	usage.LimitBytes = policy.DefaultLimitBytes
	usage.RemainingBytes = max(policy.DefaultLimitBytes-used, 0)
	if policy.DefaultLimitBytes > 0 {
		usage.UsagePercent = int(used * 100 / policy.DefaultLimitBytes)
	}
	switch {
	case used >= policy.DefaultLimitBytes:
		usage.State = "full"
	case usage.UsagePercent >= policy.WarningThresholdPercent:
		usage.State = "warning"
	default:
		usage.State = "ok"
	}
	stats.UserStorage = usage
	return nil
}

func (h *UsageHandler) writeStatsError(c *gin.Context, orgID string, err error) {
	h.logger.Error("failed to get usage stats",
		"error", err,
		"org_id", orgID,
		"request_id", c.GetString("request_id"),
	)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "failed to retrieve usage statistics",
	})
}
