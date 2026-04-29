package domain_test

import (
	"testing"
	"time"

	"datasus/internal/domain"
)

func TestParseFilename_ValidCases(t *testing.T) {
	currentYear := time.Now().Year() % 100

	cases := []struct {
		input   string
		catalog string
		state   string
		year    int
		month   int
	}{
		{"SPTO2602.dbc", "SP", "TO", 2026, 2},
		{"RJRJ2401.dbc", "RJ", "RJ", 2024, 1},
		{"ACAO0012.dbc", "AC", "AO", 2000, 12},
		{"MGMG9901.dbc", "MG", "MG", 1999, 1},
		// lowercase extension
		{"SPSP2501.dbc", "SP", "SP", 2025, 1},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			// Override year boundary test: if the hardcoded year is ambiguous, skip
			_ = currentYear
			got, err := domain.ParseFilename(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Catalog != tc.catalog {
				t.Errorf("catalog: want %q, got %q", tc.catalog, got.Catalog)
			}
			if got.State != tc.state {
				t.Errorf("state: want %q, got %q", tc.state, got.State)
			}
			if got.Year != tc.year {
				t.Errorf("year: want %d, got %d", tc.year, got.Year)
			}
			if got.Month != tc.month {
				t.Errorf("month: want %d, got %d", tc.month, got.Month)
			}
		})
	}
}

func TestParseFilename_Invalid(t *testing.T) {
	cases := []struct {
		input string
		desc  string
	}{
		{"SPTO260.dbc", "base too short"},
		{"SPTO26020.dbc", "nine chars non-letter suffix"},
		{"RDSP2401AB.dbc", "base too long"},
		{"SPTO2602.csv", "wrong extension"},
		{"SPTO2602", "no extension"},
		{"12TO2602.dbc", "numeric catalog"},
		{"SPXY26A2.dbc", "non-numeric year"},
		{"SPXY2699.dbc", "month out of range"},
		{"SPXY2600.dbc", "month zero"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := domain.ParseFilename(tc.input)
			if err == nil {
				t.Errorf("expected error for %q (%s), got nil", tc.input, tc.desc)
			}
		})
	}
}

func TestParseFilename_SegmentedParts(t *testing.T) {
	cases := []struct {
		input   string
		catalog string
		state   string
		year    int
		month   int
		segment string
	}{
		{"RDSP2401A.dbc", "RD", "SP", 2024, 1, "A"},
		{"rdsp2401b.dbc", "RD", "SP", 2024, 1, "B"},
		{"SPSP2501Z.dbc", "SP", "SP", 2025, 1, "Z"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := domain.ParseFilename(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Catalog != tc.catalog {
				t.Errorf("catalog: want %q, got %q", tc.catalog, got.Catalog)
			}
			if got.State != tc.state {
				t.Errorf("state: want %q, got %q", tc.state, got.State)
			}
			if got.Year != tc.year {
				t.Errorf("year: want %d, got %d", tc.year, got.Year)
			}
			if got.Month != tc.month {
				t.Errorf("month: want %d, got %d", tc.month, got.Month)
			}
			if got.Segment != tc.segment {
				t.Errorf("segment: want %q, got %q", tc.segment, got.Segment)
			}
		})
	}
}

func TestParseFilename_YearBoundary(t *testing.T) {
	currentTwoDigit := time.Now().Year() % 100

	// A year <= current two-digit year should be in 2000s
	yr2000s := currentTwoDigit // e.g. 26 → 2026
	name2000s := "SPSP" + fmt2d(yr2000s) + "01.dbc"

	// A year > current two-digit year should be in 1900s
	yr1900s := currentTwoDigit + 1
	if yr1900s > 99 {
		yr1900s = 99
	}
	name1900s := "SPSP" + fmt2d(yr1900s) + "01.dbc"

	got, err := domain.ParseFilename(name2000s)
	if err != nil {
		t.Fatalf("2000s case error: %v", err)
	}
	if got.Year != 2000+yr2000s {
		t.Errorf("2000s year: want %d, got %d", 2000+yr2000s, got.Year)
	}

	got, err = domain.ParseFilename(name1900s)
	if err != nil {
		t.Fatalf("1900s case error: %v", err)
	}
	if got.Year != 1900+yr1900s {
		t.Errorf("1900s year: want %d, got %d", 1900+yr1900s, got.Year)
	}
}

func fmt2d(n int) string {
	return [...]string{
		"00", "01", "02", "03", "04", "05", "06", "07", "08", "09",
		"10", "11", "12", "13", "14", "15", "16", "17", "18", "19",
		"20", "21", "22", "23", "24", "25", "26", "27", "28", "29",
		"30", "31", "32", "33", "34", "35", "36", "37", "38", "39",
		"40", "41", "42", "43", "44", "45", "46", "47", "48", "49",
		"50", "51", "52", "53", "54", "55", "56", "57", "58", "59",
		"60", "61", "62", "63", "64", "65", "66", "67", "68", "69",
		"70", "71", "72", "73", "74", "75", "76", "77", "78", "79",
		"80", "81", "82", "83", "84", "85", "86", "87", "88", "89",
		"90", "91", "92", "93", "94", "95", "96", "97", "98", "99",
	}[n]
}

func TestFile_TransitionTo_Valid(t *testing.T) {
	cases := []struct {
		from domain.OverallStatus
		to   domain.OverallStatus
	}{
		{domain.StatusPending, domain.StatusDownloading},
		{domain.StatusPending, domain.StatusIgnored},
		{domain.StatusIgnored, domain.StatusPending},
		{domain.StatusDownloading, domain.StatusDownloaded},
		{domain.StatusDownloaded, domain.StatusConvertingCSV},
		{domain.StatusConvertingCSV, domain.StatusCSVReady},
		{domain.StatusCSVReady, domain.StatusConvertingParquet},
		{domain.StatusConvertingParquet, domain.StatusParquetReady},
		{domain.StatusPending, domain.StatusFailed},
		{domain.StatusFailed, domain.StatusDownloading},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			f := &domain.File{OverallStatus: tc.from}
			if err := f.TransitionTo(tc.to); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if f.OverallStatus != tc.to {
				t.Errorf("status not updated: want %q, got %q", tc.to, f.OverallStatus)
			}
		})
	}
}

func TestFile_TransitionTo_Invalid(t *testing.T) {
	cases := []struct {
		from domain.OverallStatus
		to   domain.OverallStatus
	}{
		{domain.StatusPending, domain.StatusCSVReady},
		{domain.StatusParquetReady, domain.StatusPending},
		{domain.StatusDownloaded, domain.StatusParquetReady},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			f := &domain.File{OverallStatus: tc.from}
			err := f.TransitionTo(tc.to)
			if err == nil {
				t.Errorf("expected ErrInvalidTransition, got nil")
			}
		})
	}
}

func TestFile_TransitionTo_Purged(t *testing.T) {
	statuses := []domain.OverallStatus{
		domain.StatusPending,
		domain.StatusIgnored,
		domain.StatusDownloading,
		domain.StatusDownloaded,
		domain.StatusConvertingCSV,
		domain.StatusCSVReady,
		domain.StatusConvertingParquet,
		domain.StatusParquetReady,
		domain.StatusFailed,
	}

	for _, s := range statuses {
		t.Run(string(s)+"→purged", func(t *testing.T) {
			f := &domain.File{OverallStatus: s}
			if err := f.TransitionTo(domain.StatusPurged); err != nil {
				t.Errorf("unexpected error purging from %q: %v", s, err)
			}
		})
	}
}

func TestFile_TransitionTo_PurgedIsTerminal(t *testing.T) {
	f := &domain.File{OverallStatus: domain.StatusPurged}
	err := f.TransitionTo(domain.StatusPending)
	if err == nil {
		t.Error("expected error transitioning out of purged, got nil")
	}
}
