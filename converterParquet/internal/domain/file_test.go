package domain

import (
	"fmt"
	"testing"
	"time"
)

func TestParseFilename_ValidCases(t *testing.T) {
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
		{"SPSP2501.dbc", "SP", "SP", 2025, 1},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseFilename(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Catalog != tc.catalog || got.State != tc.state || got.Year != tc.year || got.Month != tc.month {
				t.Fatalf("got %+v want catalog=%q state=%q year=%d month=%d", got, tc.catalog, tc.state, tc.year, tc.month)
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
		{"SPTO26020.dbc", "SIASUS nine chars but non-alpha state fragment"},
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
			_, err := ParseFilename(tc.input)
			if err == nil {
				t.Errorf("expected error for %q (%s), got nil", tc.input, tc.desc)
			}
		})
	}
}

func TestParseFilename_SIASUSNineDigitTail(t *testing.T) {
	cases := []struct {
		input   string
		catalog string
		state   string
		year    int
		month   int
	}{
		{"ABOAC1502.dbc", "ABO", "AC", 2015, 2},
		{"SADTO1508.dbc", "SAD", "TO", 2015, 8},
		{"aboac1502.DBC", "ABO", "AC", 2015, 2},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseFilename(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Catalog != tc.catalog || got.State != tc.state || got.Year != tc.year || got.Month != tc.month || got.Segment != "" {
				t.Fatalf("got %+v want catalog=%q state=%q year=%d month=%d segment=\"\"", got, tc.catalog, tc.state, tc.year, tc.month)
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
			got, err := ParseFilename(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Segment != tc.segment {
				t.Errorf("segment: want %q, got %q", tc.segment, got.Segment)
			}
		})
	}
}

func TestParseFilename_YearBoundary(t *testing.T) {
	currentTwoDigit := time.Now().Year() % 100

	yr2000s := currentTwoDigit
	name2000s := "SPSP" + fmt2d(yr2000s) + "01.dbc"

	yr1900s := currentTwoDigit + 1
	if yr1900s > 99 {
		yr1900s = 99
	}
	name1900s := "SPSP" + fmt2d(yr1900s) + "01.dbc"

	got, err := ParseFilename(name2000s)
	if err != nil {
		t.Fatalf("2000s case error: %v", err)
	}
	if got.Year != 2000+yr2000s {
		t.Errorf("2000s year: want %d, got %d", 2000+yr2000s, got.Year)
	}

	got, err = ParseFilename(name1900s)
	if err != nil {
		t.Fatalf("1900s case error: %v", err)
	}
	if got.Year != 1900+yr1900s {
		t.Errorf("1900s year: want %d, got %d", 1900+yr1900s, got.Year)
	}
}

func TestLogicalBaseStem(t *testing.T) {
	cases := []struct {
		file string
		want string
	}{
		{"RDSP2401.dbc", "RDSP2401"},
		{"RDSP2401A.dbc", "RDSP2401"},
		{"ABOAC1502.dbc", "ABOAC1502"},
	}
	for _, tc := range cases {
		got, err := LogicalBaseStem(tc.file)
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %q err=%v want %q", tc.file, got, err, tc.want)
		}
	}
}

func fmt2d(n int) string {
	if n < 0 || n > 99 {
		return fmt.Sprintf("%02d", n)
	}
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
