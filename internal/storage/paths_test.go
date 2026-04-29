package storage_test

import (
	"path/filepath"
	"testing"

	"datasus/internal/storage"
)

func TestCanonicalPath_Basic(t *testing.T) {
	got := storage.DBCPath("/data", "SP", "TO", 2026, 2, "SPTO2602.dbc")
	want := filepath.Join("/data", "SP", "TO", "2026", "02", "SPTO2602.dbc")
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestCanonicalPath_PaddedMonth(t *testing.T) {
	got := storage.CSVPath("/data", "RJ", "RJ", 2024, 1, "RJRJ2401.dbc")
	want := filepath.Join("/data", "RJ", "RJ", "2024", "01", "RJRJ2401.csv")
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestCanonicalPath_Parquet(t *testing.T) {
	got := storage.ParquetPath("/data", "MG", "MG", 1999, 12, "MGMG9912.dbc")
	want := filepath.Join("/data", "MG", "MG", "1999", "12", "MGMG9912.parquet")
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestCanonicalPath_Idempotent(t *testing.T) {
	a := storage.DBCPath("/data", "SP", "TO", 2026, 2, "SPTO2602.dbc")
	b := storage.DBCPath("/data", "SP", "TO", 2026, 2, "SPTO2602.dbc")
	if a != b {
		t.Error("path generation must be deterministic")
	}
}

func TestCanonicalPath_SegmentedPartsDistinct(t *testing.T) {
	a := storage.DBCPath("/data", "RD", "SP", 2024, 1, "RDSP2401A.dbc")
	b := storage.DBCPath("/data", "RD", "SP", 2024, 1, "RDSP2401B.dbc")
	if a == b {
		t.Fatal("segmented parts must not share the same dbc path")
	}
	wantA := filepath.Join("/data", "RD", "SP", "2024", "01", "RDSP2401A.dbc")
	wantB := filepath.Join("/data", "RD", "SP", "2024", "01", "RDSP2401B.dbc")
	if a != wantA {
		t.Errorf("want %q, got %q", wantA, a)
	}
	if b != wantB {
		t.Errorf("want %q, got %q", wantB, b)
	}
}
