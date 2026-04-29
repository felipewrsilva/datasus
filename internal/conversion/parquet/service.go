package parquet

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

// Service orchestrates the parquet_conversion stage.
type Service struct {
	encoder   Encoder
	fileRepo  *repository.FileRepository
	stageRepo *repository.StageRepository
	logRepo   *repository.LogRepository
	queue     *queue.PostgresQueue
	policy    *repository.PolicyRepository
	log       *slog.Logger
}

func NewService(
	encoder Encoder,
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	logRepo *repository.LogRepository,
	q *queue.PostgresQueue,
	policy *repository.PolicyRepository,
	log *slog.Logger,
) *Service {
	return &Service{
		encoder:   encoder,
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
		observability.JobDuration.WithLabelValues(string(domain.StageParquetConversion)).Observe(time.Since(startedAt).Seconds())
		observability.JobsProcessed.WithLabelValues(string(domain.StageParquetConversion), status).Inc()
	}()

	file, err := s.fileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		status = "failure"
		return fmt.Errorf("get file: %w", err)
	}
	if s.policy != nil {
		allow, err := s.policy.PolicyAllows(ctx, file.Catalog, file.Year, file.Month)
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

	_ = s.logRepo.Insert(ctx, file.ID, domain.StageParquetConversion, "started",
		fmt.Sprintf("converting %s to parquet", file.Filename), nil)
	_ = s.stageRepo.SetRunning(ctx, file.ID, domain.StageParquetConversion)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusConvertingParquet)

	parquetPath := storage.ParquetPath(file.RootPath, file.Catalog, file.State, file.Year, file.Month, file.Filename)
	if err := storage.EnsureDir(parquetPath); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("ensure dir: %w", err))
	}

	start := time.Now()
	if err := s.encoder.Encode(ctx, *file.DBCPath, parquetPath); err != nil {
		status = "failure"
		return s.fail(ctx, file, err)
	}
	duration := time.Since(start)

	if err := s.fileRepo.UpdatePaths(ctx, file.ID, file.DBCPath, file.CSVPath, &parquetPath); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("update paths: %w", err))
	}
	if err := s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusParquetReady); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("update status: %w", err))
	}
	if err := s.stageRepo.SetDone(ctx, file.ID, domain.StageParquetConversion); err != nil {
		status = "failure"
		return s.fail(ctx, file, fmt.Errorf("mark stage done: %w", err))
	}

	_ = s.logRepo.Insert(ctx, file.ID, domain.StageParquetConversion, "completed",
		fmt.Sprintf("parquet ready in %.1fs", duration.Seconds()),
		map[string]any{"duration_ms": duration.Milliseconds(), "path": parquetPath})

	s.log.Info("parquet conversion complete",
		"file", file.Filename, "duration_ms", duration.Milliseconds())
	return nil
}

func (s *Service) fail(ctx context.Context, file *domain.File, err error) error {
	_ = s.logRepo.Insert(ctx, file.ID, domain.StageParquetConversion, "failed", err.Error(), nil)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusFailed)
	return err
}
