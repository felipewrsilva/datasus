//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"datasus/internal/testutil"
)

func TestUpsertFromFTP_SegmentedPartsDistinctRows(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewFileRepository(pool)
	t.Cleanup(func() {
		for _, fn := range []string{"RDSP2401A.DBC", "RDSP2401B.DBC", "RDSP2401.DBC"} {
			_, _ = pool.Exec(ctx, `DELETE FROM files WHERE filename = $1`, fn)
		}
	})

	dir := "/ftp/rd"
	ts := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	size := int64(100)

	a := "A"
	b := "B"
	_, _, err := repo.UpsertFromFTP(ctx, UpsertFTPParams{
		Filename:        "RDSP2401A.DBC",
		Catalog:         "RD",
		State:           "SP",
		Year:            2024,
		Month:           1,
		Segment:         &a,
		FTPDir:          dir,
		FTPPath:         dir + "/RDSP2401A.DBC",
		SizeBytes:       &size,
		RemoteTimestamp: &ts,
		RootPath:        "/data",
	})
	if err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	_, _, err = repo.UpsertFromFTP(ctx, UpsertFTPParams{
		Filename:        "RDSP2401B.DBC",
		Catalog:         "RD",
		State:           "SP",
		Year:            2024,
		Month:           1,
		Segment:         &b,
		FTPDir:          dir,
		FTPPath:         dir + "/RDSP2401B.DBC",
		SizeBytes:       &size,
		RemoteTimestamp: &ts,
		RootPath:        "/data",
	})
	if err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	_, _, err = repo.UpsertFromFTP(ctx, UpsertFTPParams{
		Filename:        "RDSP2401.DBC",
		Catalog:         "RD",
		State:           "SP",
		Year:            2024,
		Month:           1,
		Segment:         nil,
		FTPDir:          dir,
		FTPPath:         dir + "/RDSP2401.DBC",
		SizeBytes:       &size,
		RemoteTimestamp: &ts,
		RootPath:        "/data",
	})
	if err != nil {
		t.Fatalf("upsert base: %v", err)
	}

	listAll, total, err := repo.List(ctx, ListFilters{
		Catalog: "RD",
		State:   "SP",
		Year:    intPtr(2024),
		Month:   intPtr(1),
		Limit:   50,
		Offset:  0,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Fatalf("want 3 rows for RD/SP 2024/01, got %d", total)
	}
	segCount := 0
	baseCount := 0
	for _, f := range listAll {
		if f.Segment != nil && *f.Segment != "" {
			segCount++
		} else {
			baseCount++
		}
	}
	if segCount != 2 || baseCount != 1 {
		t.Fatalf("segmented=%d base=%d, want 2 and 1", segCount, baseCount)
	}

	seg := "A"
	onlyA, totalA, err := repo.List(ctx, ListFilters{
		Catalog: "RD",
		State:   "SP",
		Year:    intPtr(2024),
		Month:   intPtr(1),
		Segment: &seg,
		Limit:   50,
		Offset:  0,
	})
	if err != nil {
		t.Fatalf("list segment A: %v", err)
	}
	if totalA != 1 || len(onlyA) != 1 || onlyA[0].Filename != "RDSP2401A.DBC" {
		t.Fatalf("segment filter: total=%d file=%v", totalA, onlyA)
	}

	hasSeg := true
	withSeg, totalHS, err := repo.List(ctx, ListFilters{
		Catalog:    "RD",
		State:      "SP",
		Year:       intPtr(2024),
		Month:      intPtr(1),
		HasSegment: &hasSeg,
		Limit:      50,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("list has_segment: %v", err)
	}
	if totalHS != 2 || len(withSeg) != 2 {
		t.Fatalf("has_segment true: want 2, got total=%d len=%d", totalHS, len(withSeg))
	}

	noSeg := false
	withoutSeg, totalNS, err := repo.List(ctx, ListFilters{
		Catalog:    "RD",
		State:      "SP",
		Year:       intPtr(2024),
		Month:      intPtr(1),
		HasSegment: &noSeg,
		Limit:      50,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("list has_segment false: %v", err)
	}
	if totalNS != 1 || len(withoutSeg) != 1 || withoutSeg[0].Filename != "RDSP2401.DBC" {
		t.Fatalf("has_segment false: want RDSP2401.DBC, got %+v", withoutSeg)
	}
}

func intPtr(v int) *int { return &v }
