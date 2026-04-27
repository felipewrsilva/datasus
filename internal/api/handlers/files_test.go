package handlers

import (
	"testing"

	"datasus/internal/domain"
	"datasus/internal/repository"
)

func TestSumStatusCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		counts map[string]int64
		want   int64
	}{
		{
			name:   "empty",
			counts: map[string]int64{},
			want:   0,
		},
		{
			name: "exclusive statuses include async stages",
			counts: map[string]int64{
				"pending":            10,
				"downloading":        3,
				"downloaded":         7,
				"converting_csv":     2,
				"csv_ready":          5,
				"converting_parquet": 1,
				"parquet_ready":      4,
				"failed":             2,
				"purged":             1,
			},
			want: 35,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sumStatusCounts(tt.counts)
			if got != tt.want {
				t.Fatalf("sumStatusCounts() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExpectedCompletedStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		requireDownload bool
		requireCSV      bool
		requireParquet  bool
		want            domain.OverallStatus
	}{
		{
			name:            "all enabled expects parquet ready",
			requireDownload: true,
			requireCSV:      true,
			requireParquet:  true,
			want:            domain.StatusParquetReady,
		},
		{
			name:            "csv enabled expects csv ready",
			requireDownload: true,
			requireCSV:      true,
			requireParquet:  false,
			want:            domain.StatusCSVReady,
		},
		{
			name:            "download only expects downloaded",
			requireDownload: true,
			requireCSV:      false,
			requireParquet:  false,
			want:            domain.StatusDownloaded,
		},
		{
			name:            "nothing enabled falls back to pending",
			requireDownload: false,
			requireCSV:      false,
			requireParquet:  false,
			want:            domain.StatusPending,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := expectedCompletedStatus(tt.requireDownload, tt.requireCSV, tt.requireParquet)
			if got != tt.want {
				t.Fatalf("expectedCompletedStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSumBucketCounts(t *testing.T) {
	t.Parallel()

	buckets := []repository.CountBucket{
		{Key: "SP", Count: 10},
		{Key: "RJ", Count: 12},
		{Key: "MG", Count: 8},
	}
	if got := sumCountBuckets(buckets); got != 30 {
		t.Fatalf("sumCountBuckets() = %d, want %d", got, 30)
	}

	stateBuckets := []repository.StateSizeBucket{
		{Key: "SP", Count: 7},
		{Key: "RJ", Count: 3},
	}
	if got := sumStateSizeBuckets(stateBuckets); got != 10 {
		t.Fatalf("sumStateSizeBuckets() = %d, want %d", got, 10)
	}
}

func TestTruthyQuery(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{" yes ", true},
		{"on", true},
	} {
		if got := truthyQuery(tt.in); got != tt.want {
			t.Fatalf("truthyQuery(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
