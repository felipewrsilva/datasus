package parquet

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	csvconv "datasus/internal/conversion/csv"

	"github.com/JoshVarga/blast"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

func TestNativeEncoderEncodeFromDBC(t *testing.T) {
	tmp := t.TempDir()
	dbcPath := filepath.Join(tmp, "sample.dbc")
	parquetPath := filepath.Join(tmp, "sample.parquet")

	dbf := buildDBF(t,
		[]field{
			{Name: "NAME", Type: 'C', Length: 8},
			{Name: "AGE", Type: 'N', Length: 3},
		},
		[][]string{
			{"ALICE", "031"},
			{"BOB", "042"},
		},
	)
	dbcData := compressDBC(t, dbf)
	if err := os.WriteFile(dbcPath, dbcData, 0o644); err != nil {
		t.Fatalf("write dbc fixture: %v", err)
	}

	enc := NewNativeEncoder()
	if err := enc.Encode(context.Background(), dbcPath, parquetPath); err != nil {
		t.Fatalf("encode parquet: %v", err)
	}

	info, err := os.Stat(parquetPath)
	if err != nil {
		t.Fatalf("parquet artifact missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("parquet artifact is empty")
	}
}

func TestNativeEncoder_PreservesPerRowValues(t *testing.T) {
	const nRows = 200
	records := make([][]string, nRows)
	for i := range nRows {
		records[i] = []string{
			fmt.Sprintf("%08d", i),
			fmt.Sprintf("%03d", i%1000),
		}
	}
	tmp := t.TempDir()
	dbcPath := filepath.Join(tmp, "many.dbc")
	parquetPath := filepath.Join(tmp, "many.parquet")
	dbf := buildDBF(t,
		[]field{
			{Name: "NAME", Type: 'C', Length: 8},
			{Name: "CODE", Type: 'N', Length: 3},
		},
		records,
	)
	if err := os.WriteFile(dbcPath, compressDBC(t, dbf), 0o644); err != nil {
		t.Fatalf("write dbc: %v", err)
	}
	enc := NewNativeEncoder()
	enc.ProgressEveryRow = 0
	if err := enc.Encode(context.Background(), dbcPath, parquetPath); err != nil {
		t.Fatalf("encode: %v", err)
	}
	rows := readParquetUTF8Rows(t, parquetPath)
	if len(rows) != nRows {
		t.Fatalf("row count: got %d want %d", len(rows), nRows)
	}
	seen := make(map[string]struct{}, nRows)
	for i := range nRows {
		wantName := fmt.Sprintf("%08d", i)
		// N fields are decoded without leading zero padding (same as CSV).
		wantCode := strconv.Itoa(i % 1000)
		if len(rows[i]) != 2 {
			t.Fatalf("row %d cols: got %d want 2", i, len(rows[i]))
		}
		if rows[i][0] != wantName || rows[i][1] != wantCode {
			t.Fatalf("row %d: got [%q,%q] want [%q,%q]", i, rows[i][0], rows[i][1], wantName, wantCode)
		}
		seen[rows[i][0]] = struct{}{}
	}
	if len(seen) != nRows {
		t.Fatalf("unique NAME count: got %d want %d (values collapsed across rows?)", len(seen), nRows)
	}
}

func TestParquetMatchesCSV_Synthetic(t *testing.T) {
	const nRows = 50
	records := make([][]string, nRows)
	for i := range nRows {
		records[i] = []string{
			fmt.Sprintf("S%09d", i),
			strconv.Itoa(i * 7),
			fmt.Sprintf("X%dX", i%17),
			strconv.Itoa(1000 + i),
		}
	}
	tmp := t.TempDir()
	dbcPath := filepath.Join(tmp, "wide.dbc")
	csvPath := filepath.Join(tmp, "wide.csv")
	parquetPath := filepath.Join(tmp, "wide.parquet")
	dbf := buildDBF(t,
		[]field{
			{Name: "COL1", Type: 'C', Length: 10},
			{Name: "COL2", Type: 'N', Length: 5},
			{Name: "COL3", Type: 'C', Length: 8},
			{Name: "COL4", Type: 'N', Length: 4},
		},
		records,
	)
	if err := os.WriteFile(dbcPath, compressDBC(t, dbf), 0o644); err != nil {
		t.Fatalf("write dbc: %v", err)
	}
	conv := csvconv.NewNativeConverter()
	if err := conv.Convert(context.Background(), dbcPath, csvPath); err != nil {
		t.Fatalf("csv convert: %v", err)
	}
	enc := NewNativeEncoder()
	enc.ProgressEveryRow = 0
	if err := enc.Encode(context.Background(), dbcPath, parquetPath); err != nil {
		t.Fatalf("parquet encode: %v", err)
	}
	csvRows, csvHeader := readTSVAll(t, csvPath)
	pqRows := readParquetUTF8Rows(t, parquetPath)
	if len(csvHeader) != 4 {
		t.Fatalf("csv header len: %v", csvHeader)
	}
	if len(csvRows) != nRows || len(pqRows) != nRows {
		t.Fatalf("data rows csv=%d parquet=%d want %d", len(csvRows), len(pqRows), nRows)
	}
	for ri := range nRows {
		for ci := range 4 {
			if csvRows[ri][ci] != pqRows[ri][ci] {
				t.Fatalf("cell [%d][%d]: csv=%q parquet=%q", ri, ci, csvRows[ri][ci], pqRows[ri][ci])
			}
		}
	}
	if !uniqueCountsMatch(csvRows, pqRows) {
		t.Fatal("per-column unique cardinality differs between csv and parquet")
	}
}

func readParquetUTF8Rows(t *testing.T, path string) [][]string {
	t.Helper()
	fr, err := local.NewLocalFileReader(path)
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}
	defer fr.Close()
	pr, err := reader.NewParquetReader(fr, nil, 4)
	if err != nil {
		t.Fatalf("parquet reader: %v", err)
	}
	defer pr.ReadStop()
	n := int(pr.GetNumRows())
	ncols := len(pr.SchemaHandler.ValueColumns)
	if ncols == 0 {
		t.Fatal("parquet has no columns")
	}
	out := make([][]string, n)
	for ri := range out {
		out[ri] = make([]string, ncols)
	}
	for ci := range ncols {
		vals, _, _, err := pr.ReadColumnByIndex(int64(ci), pr.GetNumRows())
		if err != nil {
			t.Fatalf("read column %d: %v", ci, err)
		}
		if len(vals) != n {
			t.Fatalf("column %d value len %d want %d", ci, len(vals), n)
		}
		for ri := range n {
			out[ri][ci] = parquetCellToString(vals[ri])
		}
	}
	return out
}

func parquetCellToString(v interface{}) string {
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

func readTSVAll(t *testing.T, path string) (rows [][]string, header []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open csv: %v", err)
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
			t.Fatalf("read row: %v", err)
		}
		rows = append(rows, row)
	}
	return rows, header
}

func uniqueCountsMatch(a, b [][]string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	if len(a[0]) != len(b[0]) {
		return false
	}
	nc := len(a[0])
	for c := range nc {
		sa, sb := make(map[string]int), make(map[string]int)
		for ri := range a {
			sa[a[ri][c]]++
			sb[b[ri][c]]++
		}
		if len(sa) != len(sb) {
			return false
		}
		for k, va := range sa {
			if sb[k] != va {
				return false
			}
		}
	}
	return true
}

type field struct {
	Name   string
	Type   byte
	Length int
}

func buildDBF(t *testing.T, fields []field, records [][]string) []byte {
	t.Helper()
	recordLen := 1
	for _, f := range fields {
		recordLen += f.Length
	}
	headerLen := 32 + len(fields)*32 + 1
	totalLen := headerLen + len(records)*recordLen + 1
	buf := make([]byte, totalLen)
	buf[0] = 0x03
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(records)))
	binary.LittleEndian.PutUint16(buf[8:10], uint16(headerLen))
	binary.LittleEndian.PutUint16(buf[10:12], uint16(recordLen))

	off := 32
	for _, f := range fields {
		desc := make([]byte, 32)
		copy(desc[0:11], []byte(f.Name))
		desc[11] = f.Type
		desc[16] = byte(f.Length)
		copy(buf[off:off+32], desc)
		off += 32
	}
	buf[off] = 0x0D
	off++

	for _, rec := range records {
		buf[off] = ' '
		off++
		for i, f := range fields {
			val := []byte(rec[i])
			if len(val) > f.Length {
				val = val[:f.Length]
			}
			copy(buf[off:off+f.Length], val)
			for j := len(val); j < f.Length; j++ {
				buf[off+j] = ' '
			}
			off += f.Length
		}
	}
	buf[off] = 0x1A
	return buf
}

func compressDBC(t *testing.T, dbf []byte) []byte {
	t.Helper()
	headerLen := int(binary.LittleEndian.Uint16(dbf[8:10]))
	header := dbf[:headerLen]
	records := dbf[headerLen:]

	var body bytes.Buffer
	w := blast.NewWriter(&body, blast.Binary, blast.DictionarySize4096)
	if _, err := w.Write(records); err != nil {
		t.Fatalf("compress records: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close compressor: %v", err)
	}

	var out bytes.Buffer
	if _, err := out.Write(header); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := out.Write([]byte{0, 0, 0, 0}); err != nil {
		t.Fatalf("write checksum: %v", err)
	}
	if _, err := io.Copy(&out, &body); err != nil {
		t.Fatalf("write compressed body: %v", err)
	}
	return out.Bytes()
}
