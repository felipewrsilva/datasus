package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	_ "github.com/microsoft/go-mssqldb"

	"downloader/internal/config"
	"downloader/internal/errlog"
	"downloader/internal/ftpclient"
	"downloader/internal/runner"
	"downloader/internal/store"
)

func main() {
	verbose := flag.Bool("verbose", false, "alias for logging level debug")
	flag.Parse()

	exeDir, _ := errlog.ExeDir()
	logOut, logClose := errlog.LogWriter(exeDir)
	defer logClose()

	preCfg := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, path, err := config.Load()
	if err != nil {
		errlog.Report(preCfg, exeDir, "load_config", err)
		os.Exit(1)
	}

	level := parseLogLevel(cfg.Logging.Level)
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{
		Level: level,
	}))
	localRoot := cfg.Download.LocalRoot
	if abs, err := filepath.Abs(cfg.Download.LocalRoot); err == nil {
		localRoot = abs
	}
	log.Info("config loaded", "app", "downloader", "path", path, "local_root", localRoot)

	st, err := store.Open(cfg.SQLServer.ConnectionString)
	if err != nil {
		errlog.Report(log, exeDir, "sql_server", err)
		os.Exit(1)
	}
	defer st.Close()

	ftp := ftpclient.NewClient(cfg.FTP.Host, cfg.FTP.AnonymousUser, cfg.FTP.AnonymousPassword, cfg.FTP.ConnPool)
	ftp.SetVerifyNoOp(cfg.FTP.PoolVerifyNoOp)
	defer ftp.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runner.Run(ctx, cfg, log, st, ftp); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("shutdown", "reason", "signal or cancel")
			os.Exit(0)
		}
		errlog.Report(log, exeDir, "run", err)
		os.Exit(1)
	}
	log.Info("done")
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
