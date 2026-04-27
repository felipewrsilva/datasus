package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/felipewrsilva/datasusdbc/dbc"
)

// Converter converts a .dbc file to a .csv file.
type Converter interface {
	Convert(ctx context.Context, dbcPath, csvPath string) error
}

// NativeConverter decodes .dbc in-process and writes CSV output.
type NativeConverter struct {
	Timeout time.Duration
}

func NewNativeConverter() *NativeConverter {
	timeout := 10 * time.Minute
	return &NativeConverter{Timeout: timeout}
}

func (c *NativeConverter) Convert(ctx context.Context, dbcPath, csvPath string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	in, err := os.Open(dbcPath)
	if err != nil {
		return fmt.Errorf("open dbc: %w", err)
	}
	defer in.Close()

	out, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	defer out.Close()

	reader, err := dbc.NewReader(in)
	if err != nil {
		return fmt.Errorf("init dbc reader: %w", err)
	}
	defer reader.Close()

	w := csv.NewWriter(out)
	w.Comma = '\t'
	schema := reader.Schema()
	header := make([]string, len(schema.Fields))
	for i, field := range schema.Fields {
		header[i] = field.Name
	}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for reader.Next() {
		if ctx.Err() != nil {
			return fmt.Errorf("dbc to csv conversion timed out after %s", c.Timeout)
		}
		row := reader.Row()
		record := make([]string, len(row))
		for i, v := range row {
			record[i] = valueToString(v)
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("write csv record: %w", err)
		}
	}
	if err := reader.Err(); err != nil {
		return fmt.Errorf("read dbc row: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv writer: %w", err)
	}
	return nil
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
