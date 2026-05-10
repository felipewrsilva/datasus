package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestYearMinMax(t *testing.T) {
	c := &Config{
		Download: DownloadConfig{YearsBackInclusive: 3},
	}
	minY, maxY := c.YearMinMax(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	if minY != 2023 || maxY != 2026 {
		t.Fatalf("got %d..%d", minY, maxY)
	}
}

func TestDefaultCandidatePaths(t *testing.T) {
	exe := filepath.Join("C:\\", "tools", "downloader", "downloader.exe")
	cwd := filepath.Join("C:\\", "DataSUS", "aplications", "source", "datasus", "downloader")
	paths := defaultCandidatePaths(exe, cwd)

	if len(paths) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(paths))
	}
	if paths[0] != filepath.Join("C:\\", "tools", "downloader", "appsettings.json") {
		t.Fatalf("unexpected first candidate: %s", paths[0])
	}
	if paths[1] != filepath.Join(cwd, "appsettings.json") {
		t.Fatalf("unexpected second candidate: %s", paths[1])
	}
}

func TestLoadFirstExistingUsesSecondCandidate(t *testing.T) {
	tmp := t.TempDir()
	valid := filepath.Join(tmp, "appsettings.json")
	content := `{
  "ftp": {"host": "ftp.datasus.gov.br", "paths": ["/dados"]},
  "download": {"localRoot": "C:/tmp", "catalogs": ["AM"], "yearsBackInclusive": 1, "parallelWorkers": 1, "tempSuffix": ".partial"},
  "sqlServer": {"connectionString": "Server=.;Database=x;Trusted_Connection=True;TrustServerCertificate=True;"},
  "logging": {"level": "info"}
}`
	if err := os.WriteFile(valid, []byte(content), 0o644); err != nil {
		t.Fatalf("write valid config: %v", err)
	}

	_, path, err := loadFirstExisting([]string{filepath.Join(tmp, "missing.json"), valid})
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if path != valid {
		t.Fatalf("expected path %s, got %s", valid, path)
	}
}

func TestLoadFirstExistingStopsOnInvalidFile(t *testing.T) {
	tmp := t.TempDir()
	invalid := filepath.Join(tmp, "appsettings.json")
	if err := os.WriteFile(invalid, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	_, _, err := loadFirstExisting([]string{invalid, filepath.Join(tmp, "ignored.json")})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse json") {
		t.Fatalf("expected parse json error, got: %v", err)
	}
}
