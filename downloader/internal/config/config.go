package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Config is loaded from appsettings.json beside the executable.
type Config struct {
	FTP       FTPConfig       `json:"ftp"`
	Download  DownloadConfig  `json:"download"`
	SQLServer SQLServerConfig `json:"sqlServer"`
	Logging   LoggingConfig   `json:"logging"`
}

type FTPConfig struct {
	Host              string   `json:"host"`
	Paths             []string `json:"paths"`
	ConnPool          int      `json:"connPool"`
	PoolVerifyNoOp    bool     `json:"poolVerifyNoOp"`
	ScanBatchSize     int      `json:"scanBatchSize"`
	ScanLegacy        bool     `json:"scanLegacy"`
	ScanMaxDepth      int      `json:"scanMaxDepth"`
	ScanTimeout       string   `json:"scanTimeout"`
	ScanTimeoutParsed time.Duration
	AnonymousUser     string `json:"anonymousUser"`
	AnonymousPassword string `json:"anonymousPassword"`
}

type DownloadConfig struct {
	LocalRoot          string   `json:"localRoot"`
	Catalogs           []string `json:"catalogs"`
	YearsBackInclusive int      `json:"yearsBackInclusive"`
	ParallelWorkers    int      `json:"parallelWorkers"`
	TempSuffix         string   `json:"tempSuffix"`
}

type SQLServerConfig struct {
	ConnectionString string `json:"connectionString"`
}

type LoggingConfig struct {
	Level string `json:"level"`
}

// Load resolves appsettings.json from a deterministic fallback chain.
func Load() (*Config, string, error) {
	if p := strings.TrimSpace(os.Getenv("DATASUS_APPSETTINGS")); p != "" {
		return LoadFile(p)
	}

	candidates, err := candidatePaths()
	if err != nil {
		return nil, "", err
	}
	return loadFirstExisting(candidates)
}

func candidatePaths() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("executable path: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("working directory: %w", err)
	}
	return defaultCandidatePaths(exe, cwd), nil
}

func defaultCandidatePaths(exePath, cwd string) []string {
	exeDir := filepath.Dir(exePath)
	return []string{
		filepath.Join(exeDir, "appsettings.json"),
		filepath.Join(cwd, "appsettings.json"),
		filepath.Join(cwd, "..", "..", "..", "downloader", "appsettings.json"),
		filepath.Join(cwd, "..", "..", "downloader", "appsettings.json"),
	}
}

func loadFirstExisting(candidates []string) (*Config, string, error) {
	seen := map[string]struct{}{}
	searched := make([]string, 0, len(candidates))

	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		clean := filepath.Clean(c)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		searched = append(searched, clean)

		st, err := os.Stat(clean)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, clean, fmt.Errorf("stat %s: %w", clean, err)
		}
		if st.IsDir() {
			continue
		}
		return LoadFile(clean)
	}

	sort.Strings(searched)
	return nil, "", fmt.Errorf("appsettings.json not found; searched: %s", strings.Join(searched, ", "))
}

// LoadFile reads and validates config from path (tests).
func LoadFile(path string) (*Config, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, path, fmt.Errorf("parse json: %w", err)
	}
	if err := c.applyDefaults(); err != nil {
		return nil, path, err
	}
	return &c, path, nil
}

func (c *Config) applyDefaults() error {
	if c.FTP.Host == "" {
		return fmt.Errorf("ftp.host is required")
	}
	if c.FTP.ConnPool < 1 {
		c.FTP.ConnPool = 1
	}
	if c.FTP.ScanTimeout == "" {
		c.FTP.ScanTimeout = "30m"
	}
	d, err := time.ParseDuration(c.FTP.ScanTimeout)
	if err != nil {
		return fmt.Errorf("ftp.scanTimeout: %w", err)
	}
	c.FTP.ScanTimeoutParsed = d
	if c.FTP.ScanBatchSize < 1 {
		c.FTP.ScanBatchSize = 1000
	}
	if len(c.FTP.Paths) == 0 {
		return fmt.Errorf("ftp.paths must not be empty")
	}
	if c.Download.LocalRoot == "" {
		return fmt.Errorf("download.localRoot is required")
	}
	if len(c.Download.Catalogs) == 0 {
		return fmt.Errorf("download.catalogs must not be empty")
	}
	if c.Download.YearsBackInclusive < 0 {
		return fmt.Errorf("download.yearsBackInclusive must be >= 0")
	}
	if c.Download.ParallelWorkers < 1 {
		c.Download.ParallelWorkers = 1
	}
	if c.Download.TempSuffix == "" {
		c.Download.TempSuffix = ".partial"
	}
	if c.SQLServer.ConnectionString == "" {
		return fmt.Errorf("sqlServer.connectionString is required")
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	return nil
}

// CatalogSet returns uppercase catalog codes for membership tests.
func (c *Config) CatalogSet() map[string]struct{} {
	m := make(map[string]struct{}, len(c.Download.Catalogs))
	for _, s := range c.Download.Catalogs {
		m[strings.ToUpper(strings.TrimSpace(s))] = struct{}{}
	}
	return m
}

// YearMinMax returns inclusive [minYear, maxYear] from yearsBackInclusive relative to now.
func (c *Config) YearMinMax(now time.Time) (minYear, maxYear int) {
	maxYear = now.Year()
	minYear = maxYear - c.Download.YearsBackInclusive
	return minYear, maxYear
}
