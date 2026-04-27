package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"datasus/internal/api"
	"datasus/internal/api/handlers"
	"datasus/internal/config"
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

	scanner := ftp.NewScanner(ftpClient, cfg.FTPPaths, fileRepo, stageRepo, q, policyRepo, cfg.StorageRoot, log)
	scanManager := ftp.NewScanManager(scanner, logRepo, policyRepo, log, cfg.CronSchedule, cfg.FTPScanTimeout)
	if err := scanManager.Start(ctx); err != nil {
		log.Error("failed to start ftp scan manager", "err", err)
		os.Exit(1)
	}

	filesHandler := handlers.NewFilesHandler(fileRepo, stageRepo, logRepo, policyRepo)
	actionsHandler := handlers.NewActionsHandler(scanManager, fileRepo, stageRepo, logRepo, q, policyRepo, cfg.PauseDownloads)
	policiesHandler := handlers.NewPoliciesHandler(policyRepo)

	router := api.NewRouter(filesHandler, actionsHandler, policiesHandler, log)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.APIPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info("api listening", "port", cfg.APIPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down api")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
