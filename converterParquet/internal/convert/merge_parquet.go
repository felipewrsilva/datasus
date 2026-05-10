package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/felipewrsilva/datasusdbc/dbc"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/source"
	"github.com/xitongsys/parquet-go/writer"
)

// WriterOptions controls the parquet-go CSV writer (all columns UTF-8 strings).
type WriterOptions struct {
	ParallelWriters int64
	RowGroupSize    int64
	PageSize        int64
}

// MergeDBCsToParquet decodes ordered .dbc parts into one Parquet file with a single schema (UTF-8 columns).
func MergeDBCsToParquet(ctx context.Context, outPath string, dbcPaths []string, timeout time.Duration, wopt WriterOptions) (dataRows int64, err error) {
	if len(dbcPaths) == 0 {
		return 0, fmt.Errorf("no dbc paths")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}

	tmp := outPath + ".partial"
	var pw *writer.CSVWriter
	fw, fwErr := local.NewLocalFileWriter(tmp)
	if fwErr != nil {
		return 0, fmt.Errorf("create temp parquet: %w", fwErr)
	}

	clean := true
	defer func() {
		if clean {
			_ = fw.Close()
			_ = os.Remove(tmp)
		}
	}()

	var columns []string
	for i, dbcPath := range dbcPaths {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		n, cols, pwNext, convErr := appendOneDBCPart(ctx, fw, pw, dbcPath, i > 0, columns, wopt)
		cancel()
		if convErr != nil {
			if pw != nil {
				_ = pw.WriteStop()
			}
			return 0, convErr
		}
		pw = pwNext
		columns = cols
		dataRows += n
	}

	if err := pw.WriteStop(); err != nil {
		return 0, fmt.Errorf("finalize parquet: %w", err)
	}
	if err := fw.Close(); err != nil {
		return 0, fmt.Errorf("close parquet file: %w", err)
	}
	clean = false
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("rename parquet: %w", err)
	}
	return dataRows, nil
}

func appendOneDBCPart(ctx context.Context, fw source.ParquetFile, pw *writer.CSVWriter, dbcPath string, skipHeader bool, expectCols []string, wopt WriterOptions) (dataRows int64, columns []string, pwOut *writer.CSVWriter, err error) {
	in, err := os.Open(dbcPath)
	if err != nil {
		return 0, nil, pw, fmt.Errorf("open dbc %s: %w", dbcPath, err)
	}
	defer in.Close()

	reader, err := dbc.NewReader(in)
	if err != nil {
		return 0, nil, pw, fmt.Errorf("dbc reader %s: %w", dbcPath, err)
	}
	defer reader.Close()

	cols := columnsFromReader(reader)
	if len(cols) == 0 {
		return 0, nil, pw, fmt.Errorf("dbc %s: empty schema", dbcPath)
	}
	if skipHeader {
		if !slices.Equal(expectCols, cols) {
			return 0, nil, pw, fmt.Errorf("dbc %s: schema column mismatch vs first part", dbcPath)
		}
		columns = expectCols
	} else {
		md := buildParquetCSVMetadata(cols)
		pw, err = writer.NewCSVWriter(md, fw, wopt.ParallelWriters)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("init parquet writer: %w", err)
		}
		pw.CompressionType = 1 // Snappy
		if wopt.RowGroupSize > 0 {
			pw.RowGroupSize = wopt.RowGroupSize
		}
		if wopt.PageSize > 0 {
			pw.PageSize = wopt.PageSize
		}
		columns = cols
	}

	rec := make([]interface{}, len(columns))
	for reader.Next() {
		if ctx.Err() != nil {
			return 0, columns, pw, fmt.Errorf("dbc %s: %w", dbcPath, ctx.Err())
		}
		row := reader.Row()
		for i := range columns {
			if i < len(row) {
				rec[i] = valueToString(row[i])
			} else {
				rec[i] = ""
			}
		}
		if err := pw.Write(rec); err != nil {
			return 0, columns, pw, fmt.Errorf("write parquet row: %w", err)
		}
		dataRows++
	}
	if err := reader.Err(); err != nil {
		return 0, columns, pw, fmt.Errorf("read dbc %s: %w", dbcPath, err)
	}
	return dataRows, columns, pw, nil
}

func columnsFromReader(reader *dbc.Reader) []string {
	schema := reader.Schema()
	hdr := make([]string, len(schema.Fields))
	for i, field := range schema.Fields {
		hdr[i] = field.Name
	}
	return normalizeColumns(hdr)
}

func buildParquetCSVMetadata(columns []string) []string {
	md := make([]string, 0, len(columns))
	for _, col := range columns {
		// PLAIN avoids per-page dictionary builds; PLAIN_DICTIONARY is often much slower on
		// high-cardinality string columns typical in DATASUS extracts.
		md = append(md, fmt.Sprintf("name=%s, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN", sanitizeParquetName(col)))
	}
	return md
}

func normalizeColumns(columns []string) []string {
	out := make([]string, len(columns))
	seen := make(map[string]int, len(columns))
	for i, raw := range columns {
		name := sanitizeParquetName(raw)
		if name == "" {
			name = "column"
		}
		if n, ok := seen[name]; ok {
			n++
			seen[name] = n
			name = name + "_" + strconv.Itoa(n)
		} else {
			seen[name] = 0
		}
		out[i] = name
	}
	return out
}

func sanitizeParquetName(name string) string {
	if name == "" {
		return "column"
	}
	var sb strings.Builder
	sb.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r)
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '_':
			sb.WriteRune('_')
		default:
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

func valueToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", t)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", t)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case time.Time:
		return t.Format("2006-01-02")
	default:
		return fmt.Sprint(v)
	}
}
