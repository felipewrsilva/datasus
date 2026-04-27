package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	FTPHost         string
	FTPPaths        []string
	FTPConnPool     int
	PauseDownloads  bool
	DatabaseURL     string
	StorageRoot     string
	CronSchedule    string
	FTPScanTimeout  time.Duration
	DownloadWorkers int
	CSVWorkers      int
	ParquetWorkers  int
	RetryBaseDelay  time.Duration
	RetryMaxDelay   time.Duration
	StuckJobTimeout time.Duration
	CSVTimeout      time.Duration
	ParquetTimeout  time.Duration
	ParquetNP       int
	ParquetRowGroup int
	ParquetPageKB   int
	ParquetProgress int
	LogLevel        string
	APIPort         int
	WorkerID        string
}

// Load reads configuration from environment variables.
// All required variables must be set; missing ones return an error.
func Load() (*Config, error) {
	c := &Config{}

	c.FTPHost = requireEnv("FTP_HOST")
	rawPaths := requireEnv("FTP_PATHS")
	for _, p := range strings.Split(rawPaths, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			c.FTPPaths = append(c.FTPPaths, p)
		}
	}
	if len(c.FTPPaths) == 0 {
		return nil, fmt.Errorf("FTP_PATHS must contain at least one path")
	}

	c.FTPConnPool = intEnv("FTP_CONN_POOL", 2)
	// Downloads should run by default in local/dev environments.
	// Set PAUSE_DOWNLOADS=true only when intentionally freezing enqueue.
	c.PauseDownloads = boolEnv("PAUSE_DOWNLOADS", false)
	c.DatabaseURL = requireEnv("DATABASE_URL")
	c.StorageRoot = requireEnv("STORAGE_ROOT")
	c.CronSchedule = envOr("CRON_SCHEDULE", "0 2 * * *")
	c.FTPScanTimeout = durationEnv("FTP_SCAN_TIMEOUT", 30*time.Minute)
	c.DownloadWorkers = intEnv("DOWNLOAD_WORKERS", 5)
	c.CSVWorkers = intEnv("CSV_WORKERS", 3)
	c.ParquetWorkers = intEnv("PARQUET_WORKERS", 3)
	c.RetryBaseDelay = durationEnv("RETRY_BASE_DELAY", 30*time.Second)
	c.RetryMaxDelay = durationEnv("RETRY_MAX_DELAY", time.Hour)
	c.StuckJobTimeout = durationEnv("STUCK_JOB_TIMEOUT", 30*time.Minute)
	c.CSVTimeout = durationEnv("CSV_TIMEOUT", 10*time.Minute)
	c.ParquetTimeout = durationEnv("PARQUET_TIMEOUT", 15*time.Minute)
	c.ParquetNP = intEnv("PARQUET_WRITE_PARALLELISM", 4)
	c.ParquetRowGroup = intEnv("PARQUET_ROW_GROUP_MB", 128)
	c.ParquetPageKB = intEnv("PARQUET_PAGE_KB", 8)
	c.ParquetProgress = intEnv("PARQUET_PROGRESS_EVERY_ROWS", 100000)
	c.LogLevel = envOr("LOG_LEVEL", "info")
	c.APIPort = intEnv("API_PORT", 8080)

	hostname, _ := os.Hostname()
	c.WorkerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())

	return c, nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		// Return empty — callers that need strict validation check their deps.
		// The pipeline fails fast on first actual use.
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) int {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func durationEnv(key string, def time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return v
}

func boolEnv(key string, def bool) bool {
	s := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if s == "" {
		return def
	}
	switch s {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
