package storage

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CanonicalPath returns the deterministic disk path for a file artifact.
//
// Layout: {root}/{catalog}/{state}/{fullYear}/{paddedMonth}/{basename}.{ext}
// Example: /data/SP/TO/2026/02/SPTO2602.dbc
func CanonicalPath(root, catalog, state string, year, month int, filename, ext string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	return filepath.Join(
		root,
		catalog,
		state,
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
		base+ext,
	)
}

// DBCPath returns the canonical path for a .dbc artifact.
func DBCPath(root, catalog, state string, year, month int, filename string) string {
	return CanonicalPath(root, catalog, state, year, month, filename, ".dbc")
}

// CSVPath returns the canonical path for a .csv artifact.
func CSVPath(root, catalog, state string, year, month int, filename string) string {
	return CanonicalPath(root, catalog, state, year, month, filename, ".csv")
}

// ParquetPath returns the canonical path for a .parquet artifact.
func ParquetPath(root, catalog, state string, year, month int, filename string) string {
	return CanonicalPath(root, catalog, state, year, month, filename, ".parquet")
}
