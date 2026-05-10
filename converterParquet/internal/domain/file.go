package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ParsedFilename holds the structured fields extracted from a DATASUS filename.
type ParsedFilename struct {
	Catalog string
	State   string
	Year    int
	Month   int
	// Segment is a single ASCII letter A-Z when the file is a multi-part variant (e.g. RDSP2401A.DBC); empty otherwise.
	Segment string
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// ParseFilename decodes a DATASUS filename like "SPTO2602.dbc" or "RDSP2401A.dbc" into its fields.
// Formats:
//   - Standard SIH-style: [catalog:2][state:2][year:2][month:2][segment?:1].dbc (segment is one ASCII letter when present; 9 chars total).
//   - SIASUS-style (9 chars, last is a digit): [catalog:3][state:2][year:2][month:2].dbc e.g. "ABOAC1502.DBC".
func ParseFilename(name string) (ParsedFilename, error) {
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))

	if ext != ".dbc" {
		return ParsedFilename{}, fmt.Errorf("expected .dbc extension, got %q", ext)
	}

	origBase := base
	var segment string
	switch len(base) {
	case 8:
	case 9:
		last := base[8]
		if isASCIILetter(last) {
			segment = string(last)
			base = base[:8]
		} else {
			return parseFilenameSIASUSNine(origBase)
		}
	default:
		return ParsedFilename{}, fmt.Errorf("expected 8 or 9 char base name, got %d chars in %q", len(origBase), origBase)
	}

	catalog := base[0:2]
	state := base[2:4]
	yearStr := base[4:6]
	monthStr := base[6:8]

	for _, r := range catalog {
		if !unicode.IsLetter(r) {
			return ParsedFilename{}, fmt.Errorf("catalog must be alpha, got %q", catalog)
		}
	}
	for _, r := range state {
		if !unicode.IsLetter(r) {
			return ParsedFilename{}, fmt.Errorf("state must be alpha, got %q", state)
		}
	}

	year, err := parseTwoDigitInt(yearStr, "year")
	if err != nil {
		return ParsedFilename{}, err
	}
	month, err := parseTwoDigitInt(monthStr, "month")
	if err != nil {
		return ParsedFilename{}, err
	}
	if month < 1 || month > 12 {
		return ParsedFilename{}, fmt.Errorf("month out of range [1,12]: %d", month)
	}

	fullYear := expandTwoDigitYear(year)

	return ParsedFilename{
		Catalog: catalog,
		State:   state,
		Year:    fullYear,
		Month:   month,
		Segment: segment,
	}, nil
}

func parseFilenameSIASUSNine(base string) (ParsedFilename, error) {
	catalog := base[0:3]
	state := base[3:5]
	yearStr := base[5:7]
	monthStr := base[7:9]

	for _, r := range catalog {
		if !unicode.IsLetter(r) {
			return ParsedFilename{}, fmt.Errorf("catalog must be alpha, got %q", catalog)
		}
	}
	for _, r := range state {
		if !unicode.IsLetter(r) {
			return ParsedFilename{}, fmt.Errorf("state must be alpha, got %q", state)
		}
	}

	year, err := parseTwoDigitInt(yearStr, "year")
	if err != nil {
		return ParsedFilename{}, err
	}
	month, err := parseTwoDigitInt(monthStr, "month")
	if err != nil {
		return ParsedFilename{}, err
	}
	if month < 1 || month > 12 {
		return ParsedFilename{}, fmt.Errorf("month out of range [1,12]: %d", month)
	}

	fullYear := expandTwoDigitYear(year)

	return ParsedFilename{
		Catalog: catalog,
		State:   state,
		Year:    fullYear,
		Month:   month,
		Segment: "",
	}, nil
}

func expandTwoDigitYear(year int) int {
	currentTwoDigit := time.Now().Year() % 100
	fullYear := 2000 + year
	if year > currentTwoDigit {
		fullYear = 1900 + year
	}
	return fullYear
}

func parseTwoDigitInt(s, field string) (int, error) {
	if len(s) != 2 {
		return 0, fmt.Errorf("%s must be 2 digits, got %q", field, s)
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%s must be numeric, got %q", field, s)
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

// LogicalBaseStem returns the consolidated output stem (no extension): 8 chars for SIH (segment stripped),
// or 9 chars for SIASUS-style names. name must be the basename or full path of a .dbc file.
func LogicalBaseStem(name string) (string, error) {
	base := strings.ToUpper(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	switch len(base) {
	case 8:
		return base, nil
	case 9:
		last := base[8]
		if isASCIILetter(last) {
			return base[:8], nil
		}
		return base, nil
	default:
		return "", fmt.Errorf("expected 8 or 9 char base name, got %d chars in %q", len(base), base)
	}
}
