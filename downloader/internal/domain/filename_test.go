package domain

import (
	"testing"
	"time"
)

func TestParseFilename_standardEight(t *testing.T) {
	p, err := ParseFilename("SPTO2602.dbc")
	if err != nil {
		t.Fatal(err)
	}
	if p.Catalog != "SP" || p.State != "TO" || p.Month != 2 {
		t.Fatalf("got %+v", p)
	}
	wantYear := expandTwoDigitYear(26)
	if p.Year != wantYear {
		t.Fatalf("year %d want %d", p.Year, wantYear)
	}
	if p.Segment != "" {
		t.Fatalf("segment %q", p.Segment)
	}
}

func TestParseFilename_segmentNine(t *testing.T) {
	p, err := ParseFilename("RDSP2401A.dbc")
	if err != nil {
		t.Fatal(err)
	}
	if p.Catalog != "RD" || p.State != "SP" || p.Segment != "A" {
		t.Fatalf("got %+v", p)
	}
	if p.Year != expandTwoDigitYear(24) || p.Month != 1 {
		t.Fatalf("got %+v", p)
	}
}

func TestParseFilename_siasusNineDigit(t *testing.T) {
	p, err := ParseFilename("ABOAC1502.DBC")
	if err != nil {
		t.Fatal(err)
	}
	if p.Catalog != "ABO" || p.State != "AC" {
		t.Fatalf("got %+v", p)
	}
	if p.Year != expandTwoDigitYear(15) || p.Month != 2 {
		t.Fatalf("got %+v", p)
	}
}

func TestParseFilename_rejectsNonDbc(t *testing.T) {
	_, err := ParseFilename("foo.txt")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExpandTwoDigitYear(t *testing.T) {
	cur := time.Now().Year() % 100
	for _, y := range []int{0, 1, 50, 99} {
		got := expandTwoDigitYear(y)
		want := 2000 + y
		if y > cur {
			want = 1900 + y
		}
		if got != want {
			t.Fatalf("expandTwoDigitYear(%d)=%d want %d (curTwoDigit=%d)", y, got, want, cur)
		}
	}
}
