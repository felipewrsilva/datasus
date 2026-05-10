package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParquetPath(t *testing.T) {
	got := ParquetPath(`C:\out`, 2024, "RDSP2401")
	got = filepath.ToSlash(got)
	wantSuffix := "out/2024/RDSP2401.parquet"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("got %q want suffix %q", got, wantSuffix)
	}
}
