//go:build integration

package repository

import (
	"context"
	"testing"

	"datasus/internal/domain"
	"datasus/internal/testutil"
)

func TestPipelineCompletedListTotal_matchesPipelineConsistency(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	stageRepo := NewStageRepository(pool)
	fileRepo := NewFileRepository(pool)
	policyRepo := NewPolicyRepository(pool)

	proc, err := policyRepo.ProcessingStages(ctx)
	if err != nil {
		t.Fatalf("ProcessingStages: %v", err)
	}
	requireDownload := proc.EnableDownload
	requireCSV := proc.EnableCSV
	requireParquet := proc.EnableParquet

	terminal := domain.StatusPending
	switch {
	case requireParquet:
		terminal = domain.StatusParquetReady
	case requireCSV:
		terminal = domain.StatusCSVReady
	case requireDownload:
		terminal = domain.StatusDownloaded
	}

	pc, err := stageRepo.PipelineConsistency(ctx, terminal, requireDownload, requireCSV, requireParquet)
	if err != nil {
		t.Fatalf("PipelineConsistency: %v", err)
	}

	_, total, err := fileRepo.List(ctx, ListFilters{
		PipelineCompleted: true,
		RequireDownload:   requireDownload,
		RequireCSV:        requireCSV,
		RequireParquet:    requireParquet,
		Limit:             1,
		Offset:            0,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if int64(total) != pc.PipelineCompletedCount {
		t.Fatalf("List total=%d pipeline_completed, PipelineConsistency=%d", total, pc.PipelineCompletedCount)
	}
}
