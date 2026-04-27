package ftp

import (
	"testing"

	"datasus/internal/domain"
)

func TestShouldMoveToIgnored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.OverallStatus
		want   bool
	}{
		{name: "pending", status: domain.StatusPending, want: true},
		{name: "downloaded", status: domain.StatusDownloaded, want: true},
		{name: "csv_ready", status: domain.StatusCSVReady, want: true},
		{name: "failed", status: domain.StatusFailed, want: false},
		{name: "purged", status: domain.StatusPurged, want: false},
		{name: "parquet_ready", status: domain.StatusParquetReady, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldMoveToIgnored(tt.status); got != tt.want {
				t.Fatalf("shouldMoveToIgnored(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

