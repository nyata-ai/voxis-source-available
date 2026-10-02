package main

import (
	"context"
	"time"

	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/port"
)

type ossAdminRepository struct {
	port.AdminRepository
	model config.GemmaConfig
}

func newOSSAdminRepository(repo port.AdminRepository, model config.GemmaConfig) *ossAdminRepository {
	return &ossAdminRepository{AdminRepository: repo, model: model}
}

func (r *ossAdminRepository) GetOpsStats(ctx context.Context, now time.Time) (*port.AdminOpsStats, error) {
	stats, err := r.AdminRepository.GetOpsStats(ctx, now)
	if err != nil {
		return nil, err
	}
	stats.SummaryModel = &port.AdminSummaryModelMetadata{
		Model: r.model.Model, Runtime: r.model.Runtime, Revision: r.model.ModelRevision,
		Quantization: r.model.Quantization, UsageAvailable: stats.Providers.Gemma.UsageAvailable,
	}
	return stats, nil
}
