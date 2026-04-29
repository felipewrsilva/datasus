//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"datasus/internal/domain"
	"datasus/internal/queue"
	"datasus/internal/testutil"
)

func TestBulkInitStages(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	files := NewFileRepository(pool)
	stages := NewStageRepository(pool)

	dir := "/ftp/bulk_init"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'BI'`)
	})
	_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'BI'`)

	mod := time.Now()
	size := int64(1)
	in := []UpsertFTPParams{
		{Filename: "BISTAGSP2401.DBC", Catalog: "BI", State: "SP", Year: 2024, Month: 1,
			FTPDir: dir, FTPPath: dir + "/BISTAGSP2401.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
		{Filename: "BISTAGRJ2401.DBC", Catalog: "BI", State: "RJ", Year: 2024, Month: 1,
			FTPDir: dir, FTPPath: dir + "/BISTAGRJ2401.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
	}
	res, err := files.BulkUpsertFromFTP(ctx, in)
	if err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}
	if err := stages.BulkInitStages(ctx, res.IDs); err != nil {
		t.Fatalf("bulk init: %v", err)
	}

	for _, id := range res.IDs {
		ss, err := stages.ListByFile(ctx, id)
		if err != nil {
			t.Fatalf("list stages: %v", err)
		}
		if len(ss) != 3 {
			t.Fatalf("expected 3 stages for %s, got %d", id, len(ss))
		}
	}

	// Idempotent: calling twice should still leave exactly 3 stages.
	if err := stages.BulkInitStages(ctx, res.IDs); err != nil {
		t.Fatalf("bulk init second: %v", err)
	}
	for _, id := range res.IDs {
		ss, err := stages.ListByFile(ctx, id)
		if err != nil {
			t.Fatalf("list stages after second: %v", err)
		}
		if len(ss) != 3 {
			t.Fatalf("expected 3 stages after second init, got %d", len(ss))
		}
	}

	if err := stages.BulkInitStages(ctx, nil); err != nil {
		t.Fatalf("empty bulk init: %v", err)
	}
}

func TestBulkEnqueue(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	files := NewFileRepository(pool)
	q := queue.New(pool, "test-worker", 30*time.Second, time.Hour, 30*time.Minute, nil)

	dir := "/ftp/bulk_enq"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'BE'`)
	})
	_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'BE'`)

	mod := time.Now().Truncate(time.Microsecond)
	size := int64(1)
	in := []UpsertFTPParams{
		{Filename: "BENQSP2401.DBC", Catalog: "BE", State: "SP", Year: 2024, Month: 1,
			FTPDir: dir, FTPPath: dir + "/BENQSP2401.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
		{Filename: "BENQSP2402.DBC", Catalog: "BE", State: "SP", Year: 2024, Month: 2,
			FTPDir: dir, FTPPath: dir + "/BENQSP2402.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
	}
	res, err := files.BulkUpsertFromFTP(ctx, in)
	if err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}

	now := time.Now()
	items := make([]queue.EnqueueItem, 0, len(res.IDs))
	for _, id := range res.IDs {
		items = append(items, queue.EnqueueItem{
			FileID:      id,
			Stage:       domain.StageDownload,
			AvailableAt: now,
		})
	}
	if err := q.BulkEnqueue(ctx, items); err != nil {
		t.Fatalf("bulk enqueue: %v", err)
	}

	for _, id := range res.IDs {
		var status string
		if err := pool.QueryRow(ctx,
			`SELECT status::text FROM job_queue WHERE file_id = $1 AND stage = $2`,
			id, domain.StageDownload).Scan(&status); err != nil {
			t.Fatalf("verify enqueue %s: %v", id, err)
		}
		if status != "pending" {
			t.Fatalf("expected pending status, got %q", status)
		}
	}

	// Enqueue again with later available_at: ON CONFLICT path should reset.
	later := now.Add(time.Minute).Truncate(time.Microsecond)
	for i := range items {
		items[i].AvailableAt = later
	}
	if err := q.BulkEnqueue(ctx, items); err != nil {
		t.Fatalf("bulk enqueue second: %v", err)
	}
	var availAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT available_at FROM job_queue WHERE file_id = $1 AND stage = $2`,
		res.IDs[0], domain.StageDownload).Scan(&availAt); err != nil {
		t.Fatalf("verify second: %v", err)
	}
	if !availAt.Truncate(time.Microsecond).Equal(later) {
		t.Fatalf("available_at = %v, want %v", availAt, later)
	}

	if err := q.BulkEnqueue(ctx, nil); err != nil {
		t.Fatalf("empty bulk enqueue: %v", err)
	}
}

func TestLoadPolicySnapshot(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	policyRepo := NewPolicyRepository(pool)
	files := NewFileRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'PO'`)
	})
	_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
	_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
	_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
	_, _ = pool.Exec(ctx, `DELETE FROM files WHERE catalog = 'PO'`)

	mod := time.Now()
	size := int64(1)
	if _, err := files.BulkUpsertFromFTP(ctx, []UpsertFTPParams{
		{Filename: "POLSP2403.DBC", Catalog: "PO", State: "SP", Year: 2024, Month: 3,
			FTPDir: "/ftp/policy_snap", FTPPath: "/ftp/policy_snap/POLSP2403.DBC",
			SizeBytes: &size, RemoteTimestamp: &mod, RootPath: "/data"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	in := GlobalPolicy{
		SelectedCatalogs: []string{"PO"},
		SelectedPeriods: PolicyPeriods{
			Months: []YearMonth{{Year: 2024, Month: 3}},
		},
		Processing: ProcessingStages{
			EnableDownload: true,
			EnableCSV:      true,
			EnableParquet:  true,
		},
	}
	if err := policyRepo.ReplacePolicies(ctx, in); err != nil {
		t.Fatalf("replace policies: %v", err)
	}

	snap, err := policyRepo.LoadPolicySnapshot(ctx)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if !snap.HasSelection {
		t.Fatalf("expected HasSelection = true")
	}
	if !snap.Allows("PO", 2024, 3) {
		t.Fatal("expected snap.Allows(PO, 2024, 3) = true")
	}
	if snap.Allows("PO", 2024, 4) {
		t.Fatal("expected snap.Allows(PO, 2024, 4) = false")
	}
	if snap.Allows("XX", 2024, 3) {
		t.Fatal("catalog mismatch should deny")
	}
}
