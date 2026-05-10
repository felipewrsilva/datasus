package storage

import (
	"fmt"
	"path/filepath"
)

// ParquetPath returns {root}/{year}/{logicalBase}.parquet.
// year is the expanded calendar year from the DATASUS filename (same as ParseFilename), not the source folder path.
func ParquetPath(root string, year int, logicalBase string) string {
	return filepath.Join(
		root,
		fmt.Sprintf("%04d", year),
		logicalBase+".parquet",
	)
}
