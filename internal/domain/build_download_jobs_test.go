package domain_test

import (
	"reflect"
	"testing"
	"time"

	"datasus/internal/domain"
)

func TestBuildDownloadJobs_CatalogSelectionIncludesAllPeriods(t *testing.T) {
	selection := domain.SelectionIntent{
		CatalogsAll: []domain.CatalogCode{domain.CatalogSIH},
	}

	got := domain.BuildDownloadJobs(selection)
	years := domain.AvailableYears()
	expectedTotal := len(years) * 12
	if len(got.Jobs) != expectedTotal {
		t.Fatalf("expected %d jobs, got %d", expectedTotal, len(got.Jobs))
	}
	if len(got.Invalid) != 0 {
		t.Fatalf("expected no invalid selections, got %d", len(got.Invalid))
	}

	first := got.Jobs[0]
	last := got.Jobs[len(got.Jobs)-1]

	if first.Catalog != domain.CatalogSIH || first.Year != years[0] || first.Month != 1 {
		t.Fatalf("unexpected first job: %+v", first)
	}
	if last.Catalog != domain.CatalogSIH || last.Year != time.Now().Year() || last.Month != 12 {
		t.Fatalf("unexpected last job: %+v", last)
	}
}

func TestBuildDownloadJobs_YearSelectionIncludesAllMonths(t *testing.T) {
	selection := domain.SelectionIntent{
		YearsAll: []string{domain.YearSelectionKey(domain.CatalogSIA, 2024)},
	}

	got := domain.BuildDownloadJobs(selection)
	if len(got.Jobs) != 12 {
		t.Fatalf("expected 12 jobs, got %d", len(got.Jobs))
	}
	for idx, job := range got.Jobs {
		if job.Catalog != domain.CatalogSIA {
			t.Fatalf("job[%d] catalog mismatch: %+v", idx, job)
		}
		if job.Year != 2024 {
			t.Fatalf("job[%d] year mismatch: %+v", idx, job)
		}
		if job.Month != idx+1 {
			t.Fatalf("job[%d] month mismatch: %+v", idx, job)
		}
	}
}

func TestBuildDownloadJobs_PartialAndOverlappingSelectionsDeduplicate(t *testing.T) {
	selection := domain.SelectionIntent{
		CatalogsAll: []domain.CatalogCode{domain.CatalogCNES},
		YearsAll: []string{
			domain.YearSelectionKey(domain.CatalogCNES, 2024),
			domain.YearSelectionKey(domain.CatalogSIH, 2024),
		},
		Months: []string{
			domain.MonthSelectionKey(domain.CatalogSIH, 2024, 2),
			domain.MonthSelectionKey(domain.CatalogSIH, 2024, 2), // duplicate on purpose
			domain.MonthSelectionKey(domain.CatalogSIM, 2025, 7),
		},
	}

	got := domain.BuildDownloadJobs(selection)
	cnesTotal := len(domain.AvailableYears()) * 12
	expected := cnesTotal + 12 + 1

	if len(got.Jobs) != expected {
		t.Fatalf("expected %d jobs, got %d", expected, len(got.Jobs))
	}
	if len(got.Invalid) != 0 {
		t.Fatalf("expected no invalid selections, got %d", len(got.Invalid))
	}
}

func TestBuildDownloadJobs_IsIdempotentAndDeterministic(t *testing.T) {
	selection := domain.SelectionIntent{
		CatalogsAll: []domain.CatalogCode{domain.CatalogSIA, domain.CatalogSIA},
		YearsAll: []string{
			domain.YearSelectionKey(domain.CatalogSINAN, 2023),
			domain.YearSelectionKey(domain.CatalogSINAN, 2023),
		},
		Months: []string{
			domain.MonthSelectionKey(domain.CatalogPNI, 2022, 1),
			domain.MonthSelectionKey(domain.CatalogPNI, 2022, 1),
		},
	}

	first := domain.BuildDownloadJobs(selection)
	second := domain.BuildDownloadJobs(selection)

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic result, got different outputs")
	}
}

func TestBuildDownloadJobs_ReportsInvalidSelections(t *testing.T) {
	selection := domain.SelectionIntent{
		CatalogsAll: []domain.CatalogCode{"UNKNOWN"},
		YearsAll:    []string{"SIA:2019", "broken-year-key"},
		Months:      []string{"SIM:2024:13", "bad"},
	}

	got := domain.BuildDownloadJobs(selection)
	if len(got.Invalid) != 4 {
		t.Fatalf("expected 4 invalid selections, got %d", len(got.Invalid))
	}
}
