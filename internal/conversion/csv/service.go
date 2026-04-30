package csv

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"datasus/internal/domain"
	"datasus/internal/observability"
	"datasus/internal/queue"
	"datasus/internal/repository"
	"datasus/internal/storage"
)

// Service orchestrates the csv_conversion stage.
type Service struct {
	converter Converter
	fileRepo  *repository.FileRepository
	stageRepo *repository.StageRepository
	logRepo   *repository.LogRepository
	queue     *queue.PostgresQueue
	policy    *repository.PolicyRepository
	log       *slog.Logger
}

func NewService(
	converter Converter,
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	logRepo *repository.LogRepository,
	q *queue.PostgresQueue,
	policy *repository.PolicyRepository,
	log *slog.Logger,
) *Service {
	return &Service{
		converter: converter,
		fileRepo:  fileRepo,
		stageRepo: stageRepo,
		logRepo:   logRepo,
		queue:     q,
		policy:    policy,
		log:       log,
	}
}

func (s *Service) Process(ctx context.Context, job *queue.Job) error {
	startedAt := time.Now()
	status := "success"
	defer func() {
		observability.JobDuration.WithLabelValues(string(domain.StageCSVConversion)).Observe(time.Since(startedAt).Seconds())
		observability.JobsProcessed.WithLabelValues(string(domain.StageCSVConversion), status).Inc()
	}()

	file, err := s.fileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		status = "failure"
		return fmt.Errorf("get file: %w", err)
	}
	if s.policy != nil {
		allow, err := s.policy.PolicyAllows(ctx, file.Catalog, file.State, file.Year, file.Month)
		if err != nil {
			status = "failure"
			return err
		}
		if !allow {
			_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusIgnored)
			status = "failure"
			return domain.ErrPolicyBlocked
		}
	}
	if file.DBCPath == nil {
		status = "failure"
		return fmt.Errorf("dbc file not on disk for %s", file.Filename)
	}

	_ = s.logRepo.Insert(ctx, file.ID, domain.StageCSVConversion, "started",
		fmt.Sprintf("converting %s to csv", file.Filename), nil)
	_ = s.stageRepo.SetRunning(ctx, file.ID, domain.StageCSVConversion)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusConvertingCSV)

	csvRoot := file.RootPath
	if s.policy != nil {
		if dirs, dirErr := s.policy.ProcessingDirectories(ctx); dirErr == nil {
			csvRoot = storage.ResolveDirectory(dirs.CSVDir, csvRoot)
		}
	}
	csvPath := storage.CSVPath(csvRoot, file.Catalog, file.State, file.Year, file.Month, file.Filename)
	if err := storage.EnsureDir(csvPath); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("ensure dir: %w", err))
	}

	start := time.Now()
	if err := s.converter.Convert(ctx, *file.DBCPath, csvPath); err != nil {
		status = "failure"
		return s.fail(ctx, file, err)
	}
	duration := time.Since(start)

	if err := s.fileRepo.UpdatePaths(ctx, file.ID, file.DBCPath, &csvPath, file.ParquetPath); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("update paths: %w", err))
	}
	if err := s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusCSVReady); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("update status: %w", err))
	}
	if err := s.stageRepo.SetDone(ctx, file.ID, domain.StageCSVConversion); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("mark stage done: %w", err))
	}

	_ = s.logRepo.Insert(ctx, file.ID, domain.StageCSVConversion, "completed",
		fmt.Sprintf("csv ready in %.1fs", duration.Seconds()),
		map[string]any{"duration_ms": duration.Milliseconds(), "path": csvPath})

	s.log.Info("csv conversion complete",
		"file", file.Filename, "duration_ms", duration.Milliseconds())
	return nil
}

func (s *Service) fail(ctx context.Context, file *domain.File, err error) error {
	_ = s.logRepo.Insert(ctx, file.ID, domain.StageCSVConversion, "failed", err.Error(), nil)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusFailed)
	return err
}
