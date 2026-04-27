package parquet

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoshVarga/blast"
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
