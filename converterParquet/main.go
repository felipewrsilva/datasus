package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/microsoft/go-mssqldb"

	"converterParquet/internal/config"
	"converterParquet/internal/errlog"
	"converterParquet/internal/runner"
	"converterParquet/internal/store"
)

func main() {
	configPath := flag.String("config", "", "path to appsettings.json (default: beside executable)")
	dryRun := flag.Bool("dry-run", false, "list planned outputs and fingerprints only; no SQL or file writes")
	verbose := flag.Bool("verbose", false, "alias for logging level debug")
	flag.Parse()

	exeDir, _ := errlog.ExeDir()
	logOut, logClose := errlog.LogWriter(exeDir)
	defer logClose()

	preCfg := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var cfg *config.Config
	var loadedPath string
	var err error
	if strings.TrimSpace(*configPath) != "" {
		cfg, loadedPath, err = config.LoadFile(*configPath)
	} else {
		cfg, loadedPath, err = config.Load()
	}
	if err != nil {
		errlog.Report(preCfg, exeDir, "load_config", err)
		os.Exit(1)
	}

	level := parseLogLevel(cfg.Logging.Level)
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))
	log.Info("config loaded", "app", "converterParquet", "path", loadedPath)

	if err := cfg.Validate(!*dryRun); err != nil {
		errlog.Report(log, exeDir, "config_invalid", err)
		os.Exit(1)
	}

	var st *store.Store
	if !*dryRun {
		st, err = store.Open(cfg.SQLServer.ConnectionString)
		if err != nil {
			errlog.Report(log, exeDir, "sql_server", err)
			os.Exit(1)
		}
		defer st.Close()
		maxConns := max(16, cfg.ParallelWorkers*5)
		st.SetMaxOpenConns(maxConns)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runner.Run(ctx, cfg, log, st, *dryRun); err != nil {
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
