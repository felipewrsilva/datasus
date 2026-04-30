package pipeline

import (
	"context"
	"strings"
	"time"

	"datasus/internal/domain"
	"datasus/internal/queue"
	"datasus/internal/repository"
	"datasus/internal/storage"
)

// Reconciler centralizes stage eligibility and enqueue decisions.
type Reconciler struct {
	fileRepo  *repository.FileRepository
	stageRepo *repository.StageRepository
	queue     *queue.PostgresQueue
	policy    *repository.PolicyRepository
}

func NewReconciler(
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	q *queue.PostgresQueue,
	policy *repository.PolicyRepository,
) *Reconciler {
	return &Reconciler{
		fileRepo:  fileRepo,
		stageRepo: stageRepo,
		queue:     q,
		policy:    policy,
	}
}

func (r *Reconciler) ReconcileByFileID(ctx context.Context, fileID string) (int, error) {
	file, err := r.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return 0, err
	}
	return r.ReconcileFile(ctx, file)
}

func (r *Reconciler) ReconcileFile(ctx context.Context, file *domain.File) (int, error) {
	if file == nil {
		return 0, nil
	}
	if r.policy != nil {
		allow, err := r.policy.PolicyAllows(ctx, file.Catalog, file.State, file.Year, file.Month)
		if err != nil {
			return 0, err
		}
		if !allow {
			return 0, nil
		}
	}
	processing := repository.ProcessingStages{
		EnableDownload: true,
		EnableCSV:      true,
		EnableParquet:  true,
	}
	if r.policy != nil {
		stages, err := r.policy.ProcessingStages(ctx)
		if err != nil {
			return 0, err
		}
		processing = stages
	}
	if err := r.stageRepo.InitStages(ctx, file.ID); err != nil {
		return 0, err
	}
	stageRows, err := r.stageRepo.ListByFile(ctx, file.ID)
	if err != nil {
		return 0, err
	}
	statuses := make(map[domain.StageName]domain.StageStatus, len(stageRows))
	for _, row := range stageRows {
		statuses[row.Stage] = row.Status
	}

	hasDBC := file.DBCPath != nil &&
		strings.TrimSpace(*file.DBCPath) != "" &&
		storage.FileExists(*file.DBCPath)

	enqueued := 0
	tryEnqueue := func(stage domain.StageName, enabled, inputReady bool) error {
		if !enabled || !inputReady {
			return nil
		}
		switch statuses[stage] {
		case domain.StageStatusRunning, domain.StageStatusDone, domain.StageStatusPurged:
			return nil
		}
		if err := r.stageRepo.ResetForRetry(ctx, file.ID, stage); err != nil {
			return err
		}
		if err := r.queue.Enqueue(ctx, file.ID, stage, time.Now()); err != nil {
			return err
		}
		enqueued++
		return nil
	}

	if err := tryEnqueue(domain.StageDownload, processing.EnableDownload, !hasDBC); err != nil {
		return enqueued, err
	}
	if err := tryEnqueue(domain.StageCSVConversion, processing.EnableCSV, hasDBC); err != nil {
		return enqueued, err
	}
	if err := tryEnqueue(domain.StageParquetConversion, processing.EnableParquet, hasDBC); err != nil {
		return enqueued, err
	}
	return enqueued, nil
}
