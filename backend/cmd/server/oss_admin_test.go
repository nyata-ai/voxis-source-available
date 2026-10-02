package main

import (
	"context"
	"testing"
	"time"

	"github.com/voxis/backend/internal/config"
	"github.com/voxis/backend/internal/port"
)

type ossAdminRepositoryStub struct {
	stats *port.AdminOpsStats
}

func (s ossAdminRepositoryStub) GetOpsStats(context.Context, time.Time) (*port.AdminOpsStats, error) {
	return s.stats, nil
}

func TestOSSAdminRepositoryReportsConfiguredBuildAndMeasuredUsage(t *testing.T) {
	repo := newOSSAdminRepository(ossAdminRepositoryStub{stats: &port.AdminOpsStats{
		Providers: port.AdminProviderUsageStats{Gemma: port.AdminGemmaUsageStats{UsageAvailable: false}},
	}}, config.GemmaConfig{
		Model: "google/gemma-4-12B-it", Runtime: "llama.cpp", ModelRevision: "r1", Quantization: "q4_0",
	})

	stats, err := repo.GetOpsStats(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("GetOpsStats() error = %v", err)
	}
	if stats.SummaryModel == nil || stats.SummaryModel.Runtime != "llama.cpp" {
		t.Fatalf("SummaryModel = %#v, want configured runtime", stats.SummaryModel)
	}
	if stats.SummaryModel.UsageAvailable {
		t.Fatal("SummaryModel.UsageAvailable = true, want unavailable measured usage")
	}
}
