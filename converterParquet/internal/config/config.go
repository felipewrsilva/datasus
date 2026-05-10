package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is loaded from appsettings.json beside the executable (or -config path).
type Config struct {
	SourceFolder    string          `json:"source_folder"`
	OutputFolder    string          `json:"output_folder"`
	ScanSubfolders  bool            `json:"scan_subfolders"`
	MaxScanDepth    int             `json:"max_scan_depth"`
	SQLServer       SQLServerConfig `json:"sqlServer"`
	Logging         LoggingConfig   `json:"logging"`
	StrictSegments  bool            `json:"strict_segments"`
	ConflictPolicy  string          `json:"conflict_policy"`
	ConvertTimeoutS int             `json:"convert_timeout_seconds"`
	ParallelWorkers int             `json:"parallel_workers"`
	// Parquet writer tuning (optional; see applyDefaults).
	ParquetRowGroupMB      int `json:"parquet_row_group_mb"`
	ParquetPageKB          int `json:"parquet_page_kb"`
	ParquetParallelWriters int `json:"parquet_parallel_writers"`
}

type SQLServerConfig struct {
	ConnectionString string `json:"connectionString"`
}

type LoggingConfig struct {
	Level string `json:"level"`
}

// Load reads appsettings.json from the same directory as the current executable.
func Load() (*Config, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, "", fmt.Errorf("executable path: %w", err)
	}
	dir := filepath.Dir(exe)
	path := filepath.Join(dir, "appsettings.json")
	return LoadFile(path)
}

// LoadFile reads and validates config from path (tests and -config flag).
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
	if strings.TrimSpace(c.SourceFolder) == "" {
		return fmt.Errorf("source_folder is required")
	}
	if strings.TrimSpace(c.OutputFolder) == "" {
		return fmt.Errorf("output_folder is required")
	}
	if !c.ScanSubfolders {
		c.MaxScanDepth = 0
	}
	// JSON omits max_scan_depth as 0; with scan_subfolders true that would mean "root only"
	// (no files under subdirs), which is almost never intended—default to unlimited (-1).
	if c.ScanSubfolders && c.MaxScanDepth == 0 {
		c.MaxScanDepth = -1
	}
	if c.ScanSubfolders && c.MaxScanDepth < -1 {
		return fmt.Errorf("max_scan_depth must be >= -1 (-1 = unlimited)")
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.ConflictPolicy == "" {
		c.ConflictPolicy = "error"
	}
	cp := strings.ToLower(strings.TrimSpace(c.ConflictPolicy))
	if cp != "error" {
		return fmt.Errorf("conflict_policy must be \"error\" (got %q)", c.ConflictPolicy)
	}
	c.ConflictPolicy = cp
	if c.ConvertTimeoutS <= 0 {
		c.ConvertTimeoutS = 600
	}
	if c.ParallelWorkers <= 0 {
		c.ParallelWorkers = 1
	}
	if c.ParquetRowGroupMB <= 0 {
		c.ParquetRowGroupMB = 128
	}
	if c.ParquetPageKB <= 0 {
		c.ParquetPageKB = 64
	}
	if c.ParquetParallelWriters <= 0 {
		c.ParquetParallelWriters = 4
	}
	return nil
}

// Validate checks optional requirements. When requireSQL is false (-dry-run), connection string may be empty.
func (c *Config) Validate(requireSQL bool) error {
	if requireSQL && strings.TrimSpace(c.SQLServer.ConnectionString) == "" {
		return fmt.Errorf("sqlServer.connectionString is required")
	}
	return nil
}
