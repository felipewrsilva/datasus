//go:build integration

package conversion

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	csvconv "datasus/internal/conversion/csv"
	"datasus/internal/conversion/parquet"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

func fixtureRoot() string {
	if p := os.Getenv("DATASUS_FIXTURE_DIR"); p != "" {
		return p
	}
	return "/data"
}

type dbcFixture struct {
	Catalog, State string
	Year, Month     int
	Base            string
}

// Real DBC paths under DATASUS_FIXTURE_DIR (default /data). Skips if file missing.
var parquetCSVEquivalenceFixtures = []dbcFixture{
	{"AD", "DF", 2026, 2, "ADDF2602"},
	{"AQ", "AC", 2026, 2, "AQAC2602"},
	{"AD", "DF", 2026, 1, "ADDF2601"},
	{"AQ", "AC", 2026, 1, "AQAC2601"},
}

func TestParquetMatchesCSV_RealDBC(t *testing.T) {
	root := fixtureRoot()
	for _, fx := range parquetCSVEquivalenceFixtures {
		fx := fx
		name := fmt.Sprintf("%s_%s_%04d_%02d_%s", fx.Catalog, fx.State, fx.Year, fx.Month, fx.Base)
		t.Run(name, func(t *testing.T) {
			dbcPath := filepath.Join(root, fx.Catalog, fx.State,
				fmt.Sprintf("%04d", fx.Year),
				fmt.Sprintf("%02d", fx.Month),
				fx.Base+".dbc")
			if _, err := os.Stat(dbcPath); err != nil {
				t.Skip("fixture missing:", dbcPath)
			}
			tmp := t.TempDir()
			csvPath := filepath.Join(tmp, fx.Base+".csv")
			pqPath := filepath.Join(tmp, fx.Base+".parquet")
			if err := csvconv.NewNativeConverter().Convert(context.Background(), dbcPath, csvPath); err != nil {
				t.Fatalf("dbc to csv: %v", err)
			}
			enc := parquet.NewNativeEncoder()
			enc.ProgressEveryRow = 0
			if err := enc.Encode(context.Background(), dbcPath, pqPath); err != nil {
				t.Fatalf("dbc to parquet: %v", err)
			}
			assertTSVMatchesParquet(t, csvPath, pqPath)
		})
	}
}

func assertTSVMatchesParquet(t *testing.T, csvPath, pqPath string) {
	t.Helper()
	csvRows, header := readTSVAllIntegration(t, csvPath)
	pqRows := readParquetUTF8RowsIntegration(t, pqPath)
	if len(csvRows) != len(pqRows) {
		t.Fatalf("row count csv=%d parquet=%d", len(csvRows), len(pqRows))
	}
	nc := len(header)
	if nc == 0 {
		t.Fatal("empty header")
	}
	for ri := range csvRows {
		if len(csvRows[ri]) != nc {
			t.Fatalf("row %d: csv has %d cols want %d", ri, len(csvRows[ri]), nc)
		}
		if len(pqRows[ri]) != nc {
			t.Fatalf("row %d: parquet has %d cols want %d", ri, len(pqRows[ri]), nc)
		}
		for ci := range nc {
			if csvRows[ri][ci] != pqRows[ri][ci] {
				t.Fatalf("row %d col %d (%s): csv=%q parquet=%q", ri, ci, header[ci], csvRows[ri][ci], pqRows[ri][ci])
			}
		}
	}
	for _, col := range []string{"AP_AUTORIZ", "AP_CODUNI", "AP_CIDPRI"} {
		ci := indexOfString(header, col)
		if ci < 0 {
			continue
		}
		if uniqueCountColumn(csvRows, ci) != uniqueCountColumn(pqRows, ci) {
			t.Fatalf("%s unique count differs csv=%d parquet=%d", col,
				uniqueCountColumn(csvRows, ci), uniqueCountColumn(pqRows, ci))
		}
	}
}

func readTSVAllIntegration(t *testing.T, path string) (rows [][]string, header []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	header, err = r.Read()
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("row: %v", err)
		}
		rows = append(rows, row)
	}
	return rows, header
}

func readParquetUTF8RowsIntegration(t *testing.T, path string) [][]string {
	t.Helper()
	fr, err := local.NewLocalFileReader(path)
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}
	defer fr.Close()
	pr, err := reader.NewParquetReader(fr, nil, 4)
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	defer pr.ReadStop()
	n := int(pr.GetNumRows())
	ncols := len(pr.SchemaHandler.ValueColumns)
	if ncols == 0 {
		t.Fatal("no columns in parquet")
	}
	out := make([][]string, n)
	for ri := range out {
		out[ri] = make([]string, ncols)
	}
	for ci := range ncols {
		vals, _, _, err := pr.ReadColumnByIndex(int64(ci), pr.GetNumRows())
		if err != nil {
			t.Fatalf("column %d: %v", ci, err)
		}
		if len(vals) != n {
			t.Fatalf("column %d len %d want %d", ci, len(vals), n)
		}
		for ri := range n {
			out[ri][ci] = parquetCellToStringIntegration(vals[ri])
		}
	}
	return out
}

func parquetCellToStringIntegration(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func indexOfString(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}

func uniqueCountColumn(rows [][]string, col int) int {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if col < len(row) {
			seen[row[col]] = struct{}{}
		}
	}
	return len(seen)
}
