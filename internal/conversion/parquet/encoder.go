package parquet

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/felipewrsilva/datasusdbc/dbc"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/writer"
)

// Encoder converts a DBC file to Parquet.
type Encoder interface {
	Encode(ctx context.Context, dbcPath, parquetPath string) error
}

// NativeEncoder converts DBC rows to Parquet in-process using Go.
// All columns are encoded as UTF8 strings to preserve schema flexibility.
type NativeEncoder struct {
	Timeout          time.Duration
	ParallelWriters  int64
	RowGroupSize     int64
	PageSize         int64
	ProgressEveryRow int64
}

func NewNativeEncoder() *NativeEncoder {
	return &NativeEncoder{
		Timeout:          15 * time.Minute,
		ParallelWriters:  4,
		RowGroupSize:     128 * 1024 * 1024,
		PageSize:         8 * 1024,
		ProgressEveryRow: 100000,
	}
}

func (e *NativeEncoder) Encode(ctx context.Context, dbcPath, parquetPath string) error {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()

	in, err := os.Open(dbcPath)
	if err != nil {
		return fmt.Errorf("open dbc: %w", err)
	}
	defer in.Close()

	dbcReader, err := dbc.NewReader(in)
	if err != nil {
		return fmt.Errorf("init dbc reader: %w", err)
	}
	defer dbcReader.Close()

	schema := dbcReader.Schema()
	if len(schema.Fields) == 0 {
		return fmt.Errorf("dbc schema is empty")
	}

	if err := os.MkdirAll(filepath.Dir(parquetPath), 0o755); err != nil {
		return fmt.Errorf("create parquet dir: %w", err)
	}
	fw, err := local.NewLocalFileWriter(parquetPath)
	if err != nil {
		return fmt.Errorf("create parquet writer: %w", err)
	}
	defer fw.Close()

	header := make([]string, len(schema.Fields))
	for i, field := range schema.Fields {
		header[i] = field.Name
	}
	columns := normalizeColumns(header)
	md := buildParquetCSVMetadata(columns)
	pw, err := writer.NewCSVWriter(md, fw, e.ParallelWriters)
	if err != nil {
		_ = fw.Close()
		return fmt.Errorf("init parquet csv writer: %w", err)
	}
	pw.CompressionType = 1 // Snappy
	if e.RowGroupSize > 0 {
		pw.RowGroupSize = e.RowGroupSize
	}
	if e.PageSize > 0 {
		pw.PageSize = e.PageSize
	}

	start := time.Now()
	var rows int64

	for dbcReader.Next() {
		if ctx.Err() != nil {
			_ = pw.WriteStop()
			return fmt.Errorf("dbc to parquet conversion timed out after %s, rows=%d", e.Timeout, rows)
		}
		record := dbcReader.Row()
		// New slice per row: parquet-go's ParquetWriter.Write appends the slice by reference
		// and only serializes on Flush, so reusing one buffer would make every row show the last row's values.
		rec := make([]interface{}, len(columns))
		for i := range columns {
			if i < len(record) {
				rec[i] = valueToString(record[i])
			} else {
				rec[i] = ""
			}
		}
		if err := pw.Write(rec); err != nil {
			_ = pw.WriteStop()
			return fmt.Errorf("write parquet row: %w", err)
		}
		rows++
		if e.ProgressEveryRow > 0 && rows%e.ProgressEveryRow == 0 {
			elapsed := time.Since(start).Seconds()
			rate := float64(rows)
			if elapsed > 0 {
				rate = rate / elapsed
			}
			slog.Info("parquet conversion progress",
				"rows_processed", rows,
				"elapsed_ms", time.Since(start).Milliseconds(),
				"rows_per_sec", rate)
		}
	}
	if err := dbcReader.Err(); err != nil {
		_ = pw.WriteStop()
		return fmt.Errorf("read dbc row: %w", err)
	}
	if err := pw.WriteStop(); err != nil {
		return fmt.Errorf("finalize parquet writer: %w", err)
	}
	elapsed := time.Since(start).Seconds()
	rate := float64(rows)
	if elapsed > 0 {
		rate = rate / elapsed
	}
	slog.Info("parquet conversion finished",
		"rows_processed", rows,
		"elapsed_ms", time.Since(start).Milliseconds(),
		"rows_per_sec", rate)
	return nil
}

func buildParquetCSVMetadata(columns []string) []string {
	md := make([]string, 0, len(columns))
	for _, col := range columns {
		md = append(md, fmt.Sprintf("name=%s, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY", sanitizeParquetName(col)))
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
	s := ""
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			s += string(r)
		case r >= 'A' && r <= 'Z':
			s += string(r)
		case r >= '0' && r <= '9':
			s += string(r)
		case r == '_':
			s += "_"
		default:
			s += "_"
		}
	}
	return s
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
