//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"datasus/internal/domain"
	"datasus/internal/testutil"
)

func TestBulkUpsertFromFTP_InsertsThenUpdates(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewFileRepository(pool)

	dir := "/ftp/bulk_test"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'BX'`)
	})
	_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'BX'`)

	mod := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	size := int64(1000)

	mk := func(name, segment string) UpsertFTPParams {
		s := size
		m := mod
		var segPtr *string
		if segment != "" {
			seg := segment
			segPtr = &seg
		}
		return UpsertFTPParams{
			Filename:        name,
			Catalog:         "BX",
			State:           "SP",
			Year:            2024,
			Month:           3,
			Segment:         segPtr,
			FTPDir:          dir,
			FTPPath:         dir + "/" + name,
			SizeBytes:       &s,
			RemoteTimestamp: &m,
			RootPath:        "/data",
		}
	}
	in := []UpsertFTPParams{
		mk("BXSP2403A.DBC", "A"), mk("BXSP2403B.DBC", "B"), mk("BXSP2403C.DBC", "C"),
	}
	res, err := repo.BulkUpsertFromFTP(ctx, in)
	if err != nil {
		t.Fatalf("bulk upsert insert: %v", err)
	}
	if len(res.IDs) != 3 || len(res.IsNew) != 3 {
		t.Fatalf("result lengths: ids=%d new=%d", len(res.IDs), len(res.IsNew))
	}
	for i, id := range res.IDs {
		if id == "" {
			t.Fatalf("id[%d] empty", i)
		}
		if !res.IsNew[i] {
			t.Fatalf("expected first call to mark file %d as new", i)
		}
	}

	// Update path: size and mod change.
	newMod := mod.Add(2 * time.Hour)
	newSize := int64(2000)
	updated := []UpsertFTPParams{mk("BXSP2403A.DBC", "A")}
	updated[0].SizeBytes = &newSize
	updated[0].RemoteTimestamp = &newMod
	res2, err := repo.BulkUpsertFromFTP(ctx, updated)
	if err != nil {
		t.Fatalf("bulk upsert update: %v", err)
	}
	if res2.IsNew[0] {
		t.Fatalf("expected second call to NOT mark as new")
	}
	if res2.IDs[0] != res.IDs[0] {
		t.Fatalf("update should keep same id, got %q vs %q", res2.IDs[0], res.IDs[0])
	}

	got, err := repo.GetByID(ctx, res.IDs[0])
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.SizeBytes == nil || *got.SizeBytes != newSize {
		t.Fatalf("size after update: %v, want %d", got.SizeBytes, newSize)
	}
	if got.RemoteTimestamp == nil || !got.RemoteTimestamp.Equal(newMod) {
		t.Fatalf("modtime after update: %v, want %v", got.RemoteTimestamp, newMod)
	}
}

func TestBulkUpsertFromFTP_EmptyIsNoOp(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewFileRepository(pool)

	res, err := repo.BulkUpsertFromFTP(ctx, nil)
	if err != nil {
		t.Fatalf("empty bulk upsert: %v", err)
	}
	if len(res.IDs) != 0 {
		t.Fatalf("expected zero ids on empty input, got %d", len(res.IDs))
	}
}

func TestListSnapshotByFTPDir(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewFileRepository(pool)

	dir := "/ftp/snap_test"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'SN'`)
	})
	_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'SN'`)

	mod := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	size := int64(42)
	in := []UpsertFTPParams{
		{Filename: "SNAPSP2401.DBC", Catalog: "SN", State: "SP", Year: 2024, Month: 1,
			FTPDir: dir, FTPPath: dir + "/SNAPSP2401.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
		{Filename: "SNAPSP2402.DBC", Catalog: "SN", State: "SP", Year: 2024, Month: 2,
			FTPDir: dir, FTPPath: dir + "/SNAPSP2402.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
	}
	if _, err := repo.BulkUpsertFromFTP(ctx, in); err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}

	snap, err := repo.ListSnapshotByFTPDir(ctx, dir)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
	row, ok := snap["SNAPSP2401.DBC"]
	if !ok {
		t.Fatalf("missing snapshot key")
	}
	if row.SizeBytes == nil || *row.SizeBytes != size {
		t.Fatalf("snapshot size: %v", row.SizeBytes)
	}
	if row.RemoteTimestamp == nil || !row.RemoteTimestamp.Equal(mod) {
		t.Fatalf("snapshot modtime: %v", row.RemoteTimestamp)
	}
}

func TestTouchLastSeenAndBulkSetIgnored(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewFileRepository(pool)

	dir := "/ftp/touch_test"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'TC'`)
	})
	_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'TC'`)

	mod := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	size := int64(1)
	in := []UpsertFTPParams{
		{Filename: "TCHSP2401.DBC", Catalog: "TC", State: "SP", Year: 2024, Month: 1,
			FTPDir: dir, FTPPath: dir + "/TCHSP2401.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
	}
	res, err := repo.BulkUpsertFromFTP(ctx, in)
	if err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}
	id := res.IDs[0]

	// Bump last_seen_at via TouchLastSeen and verify it is more recent than the inserted value.
	before, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := repo.TouchLastSeen(ctx, []string{id}); err != nil {
		t.Fatalf("touch: %v", err)
	}
	after, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if !after.LastSeenAt.After(before.LastSeenAt) {
		t.Fatalf("last_seen_at not bumped: before=%v after=%v", before.LastSeenAt, after.LastSeenAt)
	}

	// BulkSetIgnoredByPolicy: status pending becomes ignored.
	n, err := repo.BulkSetIgnoredByPolicy(ctx, []string{id})
	if err != nil {
		t.Fatalf("bulk ignore: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows ignored = %d, want 1", n)
	}
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.OverallStatus != domain.StatusIgnored {
		t.Fatalf("status = %q, want ignored", got.OverallStatus)
	}

	// Calling again with status already ignored should affect 0 rows.
	n, err = repo.BulkSetIgnoredByPolicy(ctx, []string{id})
	if err != nil {
		t.Fatalf("bulk ignore second: %v", err)
	}
	if n != 0 {
		t.Fatalf("second call rows = %d, want 0", n)
	}
}

func TestTouchLastSeen_EmptyIsNoOp(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewFileRepository(pool)
	if err := repo.TouchLastSeen(ctx, nil); err != nil {
		t.Fatalf("empty touch: %v", err)
	}
}
