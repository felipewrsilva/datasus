package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"datasus/internal/domain"
	"datasus/internal/ftp"
	"datasus/internal/queue"
	"datasus/internal/repository"
	"datasus/internal/storage"
)

// Service orchestrates the download stage for a single file.
type Service struct {
	ftpClient *ftp.Client
	fileRepo  *repository.FileRepository
	stageRepo *repository.StageRepository
	logRepo   *repository.LogRepository
	queue     *queue.PostgresQueue
	policy    *repository.PolicyRepository
	log       *slog.Logger
}

func NewService(
	ftpClient *ftp.Client,
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	logRepo *repository.LogRepository,
	q *queue.PostgresQueue,
	policy *repository.PolicyRepository,
	log *slog.Logger,
) *Service {
	return &Service{
		ftpClient: ftpClient,
		fileRepo:  fileRepo,
		stageRepo: stageRepo,
		logRepo:   logRepo,
		queue:     q,
		policy:    policy,
		log:       log,
	}
}

// Process executes the download stage for the given job.
func (s *Service) Process(ctx context.Context, job *queue.Job) error {
	file, err := s.fileRepo.GetByID(ctx, job.FileID)
	if err != nil {
		return fmt.Errorf("get file: %w", err)
	}
	if s.policy != nil {
		allow, err := s.policy.PolicyAllows(ctx, file.Catalog, file.Year, file.Month)
		if err != nil {
			return err
		}
		if !allow {
			_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusIgnored)
			return domain.ErrPolicyBlocked
		}
	}

	_ = s.logRepo.Insert(ctx, file.ID, domain.StageDownload, "started",
		fmt.Sprintf("downloading %s", file.Filename), nil)
	_ = s.stageRepo.SetRunning(ctx, file.ID, domain.StageDownload)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusDownloading)

	destPath := storage.DBCPath(file.RootPath, file.Catalog, file.State, file.Year, file.Month, file.Filename)

	if err := storage.EnsureDir(destPath); err != nil {
		return s.fail(ctx, file, job, fmt.Errorf("ensure dir: %w", err))
	}

	// Download to a temp file to avoid partial writes
	tmp, err := os.CreateTemp("", "datasus-*.dbc")
	if err != nil {
		return s.fail(ctx, file, job, fmt.Errorf("create temp: %w", err))
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // clean up on any path

	hasher := sha256.New()
	w := io.MultiWriter(tmp, hasher)

	startedAt := time.Now()
	if err := s.ftpClient.Download(ctx, file.FTPPath, w); err != nil {
		tmp.Close()
		return s.fail(ctx, file, job, fmt.Errorf("ftp download: %w", err))
	}
	tmp.Close()

	hash := hex.EncodeToString(hasher.Sum(nil))
	duration := time.Since(startedAt)

	// Move temp file to canonical path (atomic on same filesystem)
	if err := storage.MoveFile(tmpPath, destPath); err != nil {
		return s.fail(ctx, file, job, fmt.Errorf("move file: %w", err))
	}

	// Persist metadata
	_ = s.fileRepo.UpdateLocalHash(ctx, file.ID, hash)
	_ = s.fileRepo.UpdatePaths(ctx, file.ID, &destPath, file.CSVPath, file.ParquetPath)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusDownloaded)
	_ = s.stageRepo.SetDone(ctx, file.ID, domain.StageDownload)

	// Enqueue next stages according to processing policy.
	enableCSV := true
	enableParquet := true
	if s.policy != nil {
		processing, err := s.policy.ProcessingStages(ctx)
		if err != nil {
			return s.fail(ctx, file, job, fmt.Errorf("read processing policy: %w", err))
		}
		enableCSV = processing.EnableCSV
		enableParquet = processing.EnableParquet
	}
	if enableCSV {
		_ = s.queue.Enqueue(ctx, file.ID, domain.StageCSVConversion, time.Now())
	}
	if enableParquet {
		_ = s.queue.Enqueue(ctx, file.ID, domain.StageParquetConversion, time.Now())
	}

	_ = s.logRepo.Insert(ctx, file.ID, domain.StageDownload, "completed",
		fmt.Sprintf("downloaded in %.1fs, sha256=%s", duration.Seconds(), hash[:8]),
		map[string]any{"hash": hash, "duration_ms": duration.Milliseconds(), "path": destPath})

	s.log.Info("download complete",
		"file", file.Filename, "hash", hash[:8], "duration_ms", duration.Milliseconds())
	return nil
}

func (s *Service) fail(ctx context.Context, file *domain.File, job *queue.Job, err error) error {
	_ = s.logRepo.Insert(ctx, file.ID, domain.StageDownload, "failed", err.Error(), nil)
	_ = s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusFailed)
	return err
}
