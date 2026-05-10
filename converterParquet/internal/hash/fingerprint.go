package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"converterParquet/internal/storage"
)

// Part describes one .dbc input for fingerprinting and merge order.
type Part struct {
	RelativePath string // filepath.ToSlash, relative to source root
	Segment      string // single letter or "" for unsegmented
	AbsPath      string
}

// InputFingerprintSHA256 is the lowercase hex SHA-256 of a canonical UTF-8 blob:
//
// 1. Sort parts by segment order (same rules as staging/importGo): compare strings.ToLower(segment),
// then tie-break with segment string; if still equal, compare RelativePath.
// 2. For each part in order, append one line:
//
//	relativePath + "\x00" + strconv.FormatInt(sizeBytes, 10) + "\x00" + fileSha256HexLower + "\n"
//
// where fileSha256HexLower is the SHA-256 of the file at AbsPath (hex, lowercase).
func InputFingerprintSHA256(parts []Part) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("no parts")
	}
	sorted := append([]Part(nil), parts...)
	sort.Slice(sorted, func(i, j int) bool {
		si, sj := strings.ToLower(sorted[i].Segment), strings.ToLower(sorted[j].Segment)
		if si != sj {
			return si < sj
		}
		if sorted[i].Segment != sorted[j].Segment {
			return sorted[i].Segment < sorted[j].Segment
		}
		return sorted[i].RelativePath < sorted[j].RelativePath
	})

	var b strings.Builder
	for _, p := range sorted {
		st, h, err := storage.FileStatAndSHA256Hex(p.AbsPath)
		if err != nil {
			return "", err
		}
		rel := filepath.ToSlash(strings.TrimSpace(p.RelativePath))
		b.WriteString(rel)
		b.WriteByte(0)
		b.WriteString(strconv.FormatInt(st, 10))
		b.WriteByte(0)
		b.WriteString(h)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}
