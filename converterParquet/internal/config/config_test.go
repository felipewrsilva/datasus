package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_RequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appsettings.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestLoadFile_ValidMinimal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appsettings.json")
	json := `{
		"source_folder": "C:/in",
		"output_folder": "C:/out",
		"scan_subfolders": false,
		"max_scan_depth": 0,
		"sqlServer": { "connectionString": "Server=x;Database=y;" },
		"logging": { "level": "info" }
	}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.SourceFolder != "C:/in" || c.OutputFolder != "C:/out" {
		t.Fatalf("paths: %+v", c)
	}
	if err := c.Validate(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(false); err != nil {
		t.Fatal(err)
	}
	if c.ParallelWorkers != 1 {
		t.Fatalf("parallel_workers default: got %d", c.ParallelWorkers)
	}
}

func TestLoadFile_ScanSubfoldersTrueOmittedMaxDepthDefaultsUnlimited(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appsettings.json")
	json := `{
		"source_folder": "C:/in",
		"output_folder": "C:/out",
		"scan_subfolders": true,
		"sqlServer": { "connectionString": "Server=x;Database=y;" },
		"logging": { "level": "info" }
	}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxScanDepth != -1 {
		t.Fatalf("max_scan_depth: want -1 (unlimited) when omitted with scan_subfolders true, got %d", c.MaxScanDepth)
	}
}

func TestLoadFile_ParallelWorkersZeroDefaultsToOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appsettings.json")
	json := `{
		"source_folder": "C:/in",
		"output_folder": "C:/out",
		"scan_subfolders": false,
		"max_scan_depth": 0,
		"parallel_workers": 0,
		"sqlServer": { "connectionString": "Server=x;Database=y;" },
		"logging": { "level": "info" }
	}`
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.ParallelWorkers != 1 {
		t.Fatalf("parallel_workers: got %d", c.ParallelWorkers)
	}
}

func TestValidate_SQLRequired(t *testing.T) {
	c := &Config{
		SourceFolder: "a",
		OutputFolder: "b",
		SQLServer:    SQLServerConfig{ConnectionString: ""},
	}
	_ = c.applyDefaults()
	if err := c.Validate(true); err == nil {
		t.Fatal("expected error when SQL required")
	}
}
