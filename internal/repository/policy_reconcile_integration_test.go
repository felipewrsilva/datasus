//go:build integration

package repository

import (
	"context"
	"testing"

	"datasus/internal/domain"
	"datasus/internal/testutil"
)

func TestPolicyReconcileIgnoredStatusesAndParity(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	policyRepo := NewPolicyRepository(pool)
	fileRepo := NewFileRepository(pool)

	files := []struct {
		filename string
		catalog  string
		year     int
		month    int
	}{
		{filename: "SPAC2401.dbc", catalog: "SP", year: 2024, month: 1},
		{filename: "RJAC2401.dbc", catalog: "RJ", year: 2024, month: 1},
		{filename: "SPAC2301.dbc", catalog: "SP", year: 2023, month: 1},
	}
	for _, item := range files {
		_, err := pool.Exec(ctx, `
			INSERT INTO files (filename, catalog, state, year, month, ftp_dir, ftp_path, root_path, overall_status)
			VALUES ($1, $2, 'AC', $3, $4, 'd', 'p', '/', 'pending')`,
			item.filename, item.catalog, item.year, item.month,
		)
		if err != nil {
			t.Fatalf("insert file %s: %v", item.filename, err)
		}
	}
	t.Cleanup(func() {
		for _, item := range files {
			_, _ = pool.Exec(ctx, `DELETE FROM files WHERE filename = $1`, item.filename)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_states`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
	})

	err := policyRepo.ReplacePolicies(ctx, GlobalPolicy{
		SelectedCatalogs: []string{"SP"},
		SelectedStates:   []string{"AC"},
		SelectedPeriods: PolicyPeriods{
			Years: []int{2024},
		},
		Processing: ProcessingStages{EnableDownload: true, EnableCSV: true, EnableParquet: true},
	})
	if err != nil {
		t.Fatalf("ReplacePolicies initial: %v", err)
	}

	pending, ignored, err := policyRepo.PendingAndIgnoredCounts(ctx)
	if err != nil {
		t.Fatalf("PendingAndIgnoredCounts: %v", err)
	}
	if pending != 1 || ignored != 2 {
		t.Fatalf("counts after initial policy = pending:%d ignored:%d, want pending:1 ignored:2", pending, ignored)
	}

	_, ignoredTotal, err := fileRepo.List(ctx, ListFilters{
		OverallStatus: domain.StatusIgnored,
		Limit:         100,
		Offset:        0,
	})
	if err != nil {
		t.Fatalf("List ignored: %v", err)
	}
	if int64(ignoredTotal) != ignored {
		t.Fatalf("ignored parity mismatch list=%d policy=%d", ignoredTotal, ignored)
	}

	err = policyRepo.ReplacePolicies(ctx, GlobalPolicy{
		SelectedCatalogs: []string{"SP", "RJ"},
		SelectedStates:   []string{"AC"},
		SelectedPeriods: PolicyPeriods{
			Years: []int{2023, 2024},
		},
		Processing: ProcessingStages{EnableDownload: true, EnableCSV: true, EnableParquet: true},
	})
	if err != nil {
		t.Fatalf("ReplacePolicies expanded: %v", err)
	}

	pending, ignored, err = policyRepo.PendingAndIgnoredCounts(ctx)
	if err != nil {
		t.Fatalf("PendingAndIgnoredCounts after expand: %v", err)
	}
	if pending != 3 || ignored != 0 {
		t.Fatalf("counts after expanded policy = pending:%d ignored:%d, want pending:3 ignored:0", pending, ignored)
	}

	var queuedAfterExpand int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_queue jq
		JOIN files f ON f.id = jq.file_id
		WHERE jq.stage = 'download'
		  AND jq.status = 'pending'
		  AND f.filename IN ('SPAC2401.dbc', 'RJAC2401.dbc', 'SPAC2301.dbc')
	`).Scan(&queuedAfterExpand); err != nil {
		t.Fatalf("count queued after expand: %v", err)
	}
	if queuedAfterExpand != 3 {
		t.Fatalf("queued after expand = %d, want 3", queuedAfterExpand)
	}

	// Reapplying the same policy must keep queue idempotent.
	err = policyRepo.ReplacePolicies(ctx, GlobalPolicy{
		SelectedCatalogs: []string{"SP", "RJ"},
		SelectedStates:   []string{"AC"},
		SelectedPeriods: PolicyPeriods{
			Years: []int{2023, 2024},
		},
		Processing: ProcessingStages{EnableDownload: true, EnableCSV: true, EnableParquet: true},
	})
	if err != nil {
		t.Fatalf("ReplacePolicies reapply: %v", err)
	}
	var queuedAfterReapply int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_queue jq
		JOIN files f ON f.id = jq.file_id
		WHERE jq.stage = 'download'
		  AND jq.status = 'pending'
		  AND f.filename IN ('SPAC2401.dbc', 'RJAC2401.dbc', 'SPAC2301.dbc')
	`).Scan(&queuedAfterReapply); err != nil {
		t.Fatalf("count queued after reapply: %v", err)
	}
	if queuedAfterReapply != 3 {
		t.Fatalf("queued after reapply = %d, want 3", queuedAfterReapply)
	}
}

func TestPolicyReconcileEmptySelectionMarksPendingAsIgnored(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	policyRepo := NewPolicyRepository(pool)

	files := []struct {
		filename string
		status   domain.OverallStatus
	}{
		{filename: "SPAC2402.dbc", status: domain.StatusPending},
		{filename: "RJAC2402.dbc", status: domain.StatusDownloaded},
		{filename: "MGAC2402.dbc", status: domain.StatusCSVReady},
	}
	for _, item := range files {
		_, err := pool.Exec(ctx, `
			INSERT INTO files (filename, catalog, state, year, month, ftp_dir, ftp_path, root_path, overall_status)
			VALUES ($1, 'SP', 'AC', 2024, 2, 'd', 'p', '/', $2)`,
			item.filename, item.status,
		)
		if err != nil {
			t.Fatalf("insert file %s: %v", item.filename, err)
		}
	}
	t.Cleanup(func() {
		for _, item := range files {
			_, _ = pool.Exec(ctx, `DELETE FROM files WHERE filename = $1`, item.filename)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_states`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM download_policy_months`); err != nil {
		t.Fatalf("clear policy months: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM download_policy_years`); err != nil {
		t.Fatalf("clear policy years: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM download_policy_states`); err != nil {
		t.Fatalf("clear policy states: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM download_policy_catalogs`); err != nil {
		t.Fatalf("clear policy catalogs: %v", err)
	}

	toIgnored, toPending, err := policyRepo.ReconcileIgnoredStatuses(ctx)
	if err != nil {
		t.Fatalf("ReconcileIgnoredStatuses: %v", err)
	}
	if toIgnored != 3 || toPending != 0 {
		t.Fatalf("reconcile counts = toIgnored:%d toPending:%d, want toIgnored:3 toPending:0", toIgnored, toPending)
	}

	pending, ignored, err := policyRepo.PendingAndIgnoredCounts(ctx)
	if err != nil {
		t.Fatalf("PendingAndIgnoredCounts: %v", err)
	}
	if pending != 0 || ignored < 3 {
		t.Fatalf("policy counts = pending:%d ignored:%d, want pending:0 ignored>=3", pending, ignored)
	}
}
