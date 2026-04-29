package ftp

import (
	"testing"
	"time"

	"datasus/internal/domain"
	"datasus/internal/repository"
)

func TestIsUnchanged(t *testing.T) {
	t.Parallel()

	mod := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	size := int64(123456)
	row := repository.FileSnapshotRow{
		ID:              "abc",
		Filename:        "RDSP2401.DBC",
		SizeBytes:       &size,
		RemoteTimestamp: &mod,
	}
	tests := []struct {
		name     string
		row      repository.FileSnapshotRow
		entry    Entry
		expected bool
	}{
		{
			name:     "same size and modtime",
			row:      row,
			entry:    Entry{Name: "RDSP2401.DBC", Size: 123456, ModTime: mod},
			expected: true,
		},
		{
			name:     "different size",
			row:      row,
			entry:    Entry{Name: "RDSP2401.DBC", Size: 999, ModTime: mod},
			expected: false,
		},
		{
			name:     "different modtime",
			row:      row,
			entry:    Entry{Name: "RDSP2401.DBC", Size: 123456, ModTime: mod.Add(time.Second)},
			expected: false,
		},
		{
			name: "row missing size",
			row: repository.FileSnapshotRow{
				ID:              "abc",
				Filename:        "RDSP2401.DBC",
				RemoteTimestamp: &mod,
			},
			entry:    Entry{Name: "RDSP2401.DBC", Size: 123456, ModTime: mod},
			expected: false,
		},
		{
			name: "row missing modtime",
			row: repository.FileSnapshotRow{
				ID:        "abc",
				Filename:  "RDSP2401.DBC",
				SizeBytes: &size,
			},
			entry:    Entry{Name: "RDSP2401.DBC", Size: 123456, ModTime: mod},
			expected: false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isUnchanged(tc.row, tc.entry)
			if got != tc.expected {
				t.Fatalf("isUnchanged = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestBuildUpsertParams(t *testing.T) {
	t.Parallel()

	mod := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	parsed := domain.ParsedFilename{
		Catalog: "RD",
		State:   "SP",
		Year:    2024,
		Month:   1,
		Segment: "A",
	}
	entry := Entry{
		Name:       "RDSP2401A.dbc",
		Size:       42,
		ModTime:    mod,
		RemotePath: "/dissemin/x/RDSP2401A.dbc",
	}
	got := buildUpsertParams(entry, parsed, "/dissemin/x", "/data")

	if got.Filename != "RDSP2401A.DBC" {
		t.Fatalf("Filename = %q, want uppercase RDSP2401A.DBC", got.Filename)
	}
	if got.Catalog != "RD" || got.State != "SP" || got.Year != 2024 || got.Month != 1 {
		t.Fatalf("unexpected core fields: %+v", got)
	}
	if got.Segment == nil || *got.Segment != "A" {
		t.Fatalf("Segment = %v, want pointer to A", got.Segment)
	}
	if got.FTPDir != "/dissemin/x" || got.FTPPath != "/dissemin/x/RDSP2401A.dbc" {
		t.Fatalf("ftp paths wrong: dir=%q path=%q", got.FTPDir, got.FTPPath)
	}
	if got.SizeBytes == nil || *got.SizeBytes != 42 {
		t.Fatalf("Size mismatch: %v", got.SizeBytes)
	}
	if got.RemoteTimestamp == nil || !got.RemoteTimestamp.Equal(mod) {
		t.Fatalf("RemoteTimestamp mismatch: %v", got.RemoteTimestamp)
	}
	if got.RootPath != "/data" {
		t.Fatalf("RootPath = %q, want /data", got.RootPath)
	}
	if got.RemoteChecksum != nil {
		t.Fatalf("RemoteChecksum should be nil for FTP source, got %v", got.RemoteChecksum)
	}
}

func TestBuildUpsertParams_NoSegment(t *testing.T) {
	t.Parallel()

	parsed := domain.ParsedFilename{Catalog: "SP", State: "TO", Year: 2026, Month: 2}
	entry := Entry{Name: "SPTO2602.dbc", Size: 1, ModTime: time.Now()}
	got := buildUpsertParams(entry, parsed, "/dir", "/root")
	if got.Segment != nil {
		t.Fatalf("Segment should be nil, got %v", got.Segment)
	}
}
