package hash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInputFingerprintSHA256_OrderStable(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.dbc")
	p2 := filepath.Join(dir, "b.dbc")
	if err := os.WriteFile(p1, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("yy"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp1, err := InputFingerprintSHA256([]Part{
		{RelativePath: "p/a.dbc", Segment: "a", AbsPath: p1},
		{RelativePath: "p/b.dbc", Segment: "b", AbsPath: p2},
	})
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := InputFingerprintSHA256([]Part{
		{RelativePath: "p/b.dbc", Segment: "b", AbsPath: p2},
		{RelativePath: "p/a.dbc", Segment: "a", AbsPath: p1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("order should not matter: %s vs %s", fp1, fp2)
	}
}

func TestInputFingerprintSHA256_ContentChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.dbc")
	if err := os.WriteFile(p, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp1, err := InputFingerprintSHA256([]Part{{RelativePath: "x.dbc", Segment: "", AbsPath: p}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp2, err := InputFingerprintSHA256([]Part{{RelativePath: "x.dbc", Segment: "", AbsPath: p}})
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Fatal("expected different fingerprint after content change")
	}
}
