package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"datasus/internal/config"
	csvconv "datasus/internal/conversion/csv"
	pqconv "datasus/internal/conversion/parquet"
	"datasus/internal/download"
	"datasus/internal/ftp"
	"datasus/internal/observability"
	"datasus/internal/queue"
	"datasus/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}

	log := observability.NewLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	fileRepo := repository.NewFileRepository(pool)
	stageRepo := repository.NewStageRepository(pool)
	logRepo := repository.NewLogRepository(pool)
	policyRepo := repository.NewPolicyRepository(pool)

	q := queue.New(pool, cfg.WorkerID,
		cfg.RetryBaseDelay, cfg.RetryMaxDelay, cfg.StuckJobTimeout, log)

	ftpClient := ftp.NewClient(cfg.FTPHost, cfg.FTPConnPool)
	defer ftpClient.Close()

	// Services
	dlService := download.NewService(ftpClient, fileRepo, stageRepo, logRepo, q, policyRepo, log)
	csvConverter := csvconv.NewNativeConverter()
	csvConverter.Timeout = cfg.CSVTimeout
	csvService := csvconv.NewService(csvConverter, fileRepo, stageRepo, logRepo, q, policyRepo, log)
	pqEncoder := pqconv.NewNativeEncoder()
	pqEncoder.Timeout = cfg.ParquetTimeout
	pqEncoder.ParallelWriters = int64(cfg.ParquetNP)
	pqEncoder.RowGroupSize = int64(cfg.ParquetRowGroup) * 1024 * 1024
	pqEncoder.PageSize = int64(cfg.ParquetPageKB) * 1024
	pqEncoder.ProgressEveryRow = int64(cfg.ParquetProgress)
	pqService := pqconv.NewService(pqEncoder, fileRepo, stageRepo, logRepo, q, policyRepo, log)

	// Worker pools
	dlPool := download.NewWorkerPool(dlService, q, cfg.DownloadWorkers, log)
	csvPool := csvconv.NewWorkerPool(csvService, q, cfg.CSVWorkers, log)
	pqPool := pqconv.NewWorkerPool(pqService, q, cfg.ParquetWorkers, log)

	// Stuck job recovery goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n, err := q.RecoverStuckJobs(ctx)
				if err != nil {
					log.Warn("stuck job recovery error", "err", err)
				} else if n > 0 {
					log.Info("recovered stuck jobs", "count", n)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Queue depth metrics ticker.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				entries, err := q.DepthByStageStatus(ctx)
				if err != nil {
					log.Warn("queue depth metrics error", "err", err)
					continue
				}
				observability.QueueDepth.Reset()
				for _, e := range entries {
					observability.QueueDepth.WithLabelValues(string(e.Stage), string(e.Status)).Set(e.Count)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Info("worker started",
		"download_workers", cfg.DownloadWorkers,
		"csv_workers", cfg.CSVWorkers,
		"parquet_workers", cfg.ParquetWorkers,
	)

	// Start all pools (non-blocking — they block on ctx.Done internally)
	go dlPool.Run(ctx)
	go csvPool.Run(ctx)
	go pqPool.Run(ctx)

	<-ctx.Done()
	log.Info("worker shutting down, draining in-flight jobs")

	// Give in-flight jobs 30s to complete
	time.Sleep(30 * time.Second)
	log.Info("worker stopped")
}
