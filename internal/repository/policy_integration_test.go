//go:build integration

package repository

import (
	"context"
	"testing"

	"datasus/internal/testutil"
)

func TestReplacePolicies_acceptsMonthNotPresentInFiles(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewPolicyRepository(pool)

	const fn = "policy_repo_test_month_only.dbc"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE filename = $1`, fn)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
	})

	_, err := pool.Exec(ctx, `
		INSERT INTO files (filename, catalog, state, year, month, ftp_dir, ftp_path, root_path, overall_status)
		VALUES ($1, 'SP', 'AC', 2024, 3, 'd', 'p', '/', 'pending')`,
		fn,
	)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}

	in := GlobalPolicy{
		SelectedCatalogs: []string{"SP"},
		SelectedPeriods: PolicyPeriods{
			Years:  nil,
			Months: []YearMonth{{Year: 2024, Month: 6}},
		},
		Processing: ProcessingStages{
			EnableDownload: true,
			EnableCSV:      true,
			EnableParquet:  true,
		},
	}
	if err := repo.ReplacePolicies(ctx, in); err != nil {
		t.Fatalf("ReplacePolicies: %v", err)
	}

	got, err := repo.GetPolicies(ctx)
	if err != nil {
		t.Fatalf("GetPolicies: %v", err)
	}
	if len(got.SelectedPeriods.Months) != 1 || got.SelectedPeriods.Months[0].Year != 2024 || got.SelectedPeriods.Months[0].Month != 6 {
		t.Fatalf("GetPolicies months = %+v, want one 2024-06", got.SelectedPeriods.Months)
	}

	allow, err := repo.PolicyAllows(ctx, "SP", 2024, 6)
	if err != nil || !allow {
		t.Fatalf("PolicyAllows SP 2024-6 = %v, %v", allow, err)
	}
	allow, err = repo.PolicyAllows(ctx, "SP", 2024, 3)
	if err != nil || allow {
		t.Fatalf("PolicyAllows SP 2024-3 = %v, %v (should be false)", allow, err)
	}
}

func TestReplacePolicies_rejectsYearNotInAvailablePeriods(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewPolicyRepository(pool)

	const fn = "policy_repo_test_year_reject.dbc"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE filename = $1`, fn)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
	})

	_, err := pool.Exec(ctx, `
		INSERT INTO files (filename, catalog, state, year, month, ftp_dir, ftp_path, root_path, overall_status)
		VALUES ($1, 'SP', 'AC', 2024, 1, 'd', 'p', '/', 'pending')`,
		fn,
	)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}

	in := GlobalPolicy{
		SelectedCatalogs: []string{"SP"},
		SelectedPeriods: PolicyPeriods{
			Years:  []int{2099},
			Months: nil,
		},
		Processing: ProcessingStages{
			EnableDownload: true,
			EnableCSV:      true,
			EnableParquet:  true,
		},
	}
	if err := repo.ReplacePolicies(ctx, in); err == nil {
		t.Fatal("ReplacePolicies: want error for year not in available periods")
	}
}

func TestPolicyAllows_emptySelectionDeniesAll(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	repo := NewPolicyRepository(pool)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_months`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_years`)
		_, _ = pool.Exec(ctx, `DELETE FROM download_policy_catalogs`)
	})

	allow, err := repo.PolicyAllows(ctx, "XX", 2099, 12)
	if err != nil || allow {
		t.Fatalf("PolicyAllows with empty policy = %v, %v (want false)", allow, err)
	}
	complete, err := repo.PolicySelectionComplete(ctx)
	if err != nil || complete {
		t.Fatalf("PolicySelectionComplete with empty policy = %v, %v (want false)", complete, err)
	}
}
