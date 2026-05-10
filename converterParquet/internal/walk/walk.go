package walk

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// DirDepth returns directory depth relative to root: file in root => 0, in root/a => 1.
func DirDepth(relDir string) int {
	relDir = filepath.ToSlash(relDir)
	if relDir == "." || relDir == "" {
		return 0
	}
	return strings.Count(relDir, "/") + 1
}

// ListDBC collects .dbc files under root respecting scanSubfolders and maxScanDepth (-1 = unlimited).
func ListDBC(root string, scanSubfolders bool, maxScanDepth int, visit func(absPath, relPath string) error) error {
	if !scanSubfolders {
		maxScanDepth = 0
	}
	unlimited := maxScanDepth < 0

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if unlimited {
				return nil
			}
			dpt := DirDepth(rel)
			if dpt > maxScanDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if unlimited {
			// still inside allowed tree
		} else {
			parent := filepath.Dir(rel)
			if DirDepth(parent) > maxScanDepth {
				return nil
			}
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".dbc") {
			return nil
		}
		return visit(path, rel)
	})
}
