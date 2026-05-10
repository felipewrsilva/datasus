package walk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDBC_Depth(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.dbc"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Mkdir(filepath.Join(root, "s1"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "s1", "b.dbc"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Mkdir(filepath.Join(root, "s1", "s2"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "s1", "s2", "c.dbc"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	var names []string
	err := ListDBC(root, true, 1, func(abs, rel string) error {
		names = append(names, filepath.Base(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("depth 1: want 2 files, got %v", names)
	}
}

func TestListDBC_NoSubfolders(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.dbc"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Mkdir(filepath.Join(root, "s1"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "s1", "b.dbc"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = ListDBC(root, false, 0, func(_, _ string) error {
		n++
		return nil
	})
	if n != 1 {
		t.Fatalf("want 1 file, got %d", n)
	}
}
