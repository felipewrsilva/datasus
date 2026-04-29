package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type OverallStatus string

const (
	StatusPending           OverallStatus = "pending"
	StatusIgnored           OverallStatus = "ignored"
	StatusDownloading       OverallStatus = "downloading"
	StatusDownloaded        OverallStatus = "downloaded"
	StatusConvertingCSV     OverallStatus = "converting_csv"
	StatusCSVReady          OverallStatus = "csv_ready"
	StatusConvertingParquet OverallStatus = "converting_parquet"
	StatusParquetReady      OverallStatus = "parquet_ready"
	StatusFailed            OverallStatus = "failed"
	StatusPurged            OverallStatus = "purged"
)

// validTransitions maps each status to the set of statuses it may transition to.
var validTransitions = map[OverallStatus]map[OverallStatus]bool{
	StatusPending: {
		StatusIgnored:     true,
		StatusDownloading: true,
		StatusFailed:      true,
		StatusPurged:      true,
	},
	StatusIgnored: {
		StatusPending: true,
		StatusPurged:  true,
	},
	StatusDownloading: {
		StatusDownloaded: true,
		StatusFailed:     true,
		StatusPurged:     true,
	},
	StatusDownloaded: {
		StatusConvertingCSV:     true,
		StatusConvertingParquet: true,
		StatusDownloading:       true, // re-download on change
		StatusFailed:            true,
		StatusPurged:            true,
	},
	StatusConvertingCSV: {
		StatusCSVReady: true,
		StatusFailed:   true,
		StatusPurged:   true,
	},
	StatusCSVReady: {
		StatusConvertingParquet: true,
		StatusConvertingCSV:     true, // re-convert on re-download
		StatusFailed:            true,
		StatusPurged:            true,
	},
	StatusConvertingParquet: {
		StatusParquetReady: true,
		StatusFailed:       true,
		StatusPurged:       true,
	},
	StatusParquetReady: {
		StatusConvertingParquet: true, // re-convert on new csv
		StatusConvertingCSV:     true, // re-convert on re-download
		StatusDownloading:       true, // full re-process on change
		StatusFailed:            true,
		StatusPurged:            true,
	},
	StatusFailed: {
		StatusDownloading:       true,
		StatusConvertingCSV:     true,
		StatusConvertingParquet: true,
		StatusPurged:            true,
	},
	StatusPurged: {}, // terminal
}

// File represents a DATASUS .dbc file and its processing state.
type File struct {
	ID              string
	Filename        string
	Catalog         string
	State           string
	Year            int
	Month           int
	Segment         *string `json:"segment,omitempty"` // optional single-letter part (e.g. A for RDSP2401A.DBC)
	FTPDir          string
	FTPPath         string
	SizeBytes       *int64
	RemoteChecksum  *string
	RemoteTimestamp *time.Time
	LocalHash       *string
	RootPath        string
	DBCPath         *string
	CSVPath         *string
	ParquetPath     *string
	OverallStatus   OverallStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastSeenAt      time.Time
}

// TransitionTo validates and applies a status transition.
func (f *File) TransitionTo(next OverallStatus) error {
	allowed, ok := validTransitions[f.OverallStatus]
	if !ok {
		return fmt.Errorf("%w: unknown current status %q", ErrInvalidTransition, f.OverallStatus)
	}
	if !allowed[next] {
		return fmt.Errorf("%w: %q → %q", ErrInvalidTransition, f.OverallStatus, next)
	}
	f.OverallStatus = next
	return nil
}

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
// Format: [catalog:2][state:2][year:2][month:2][segment?:1].dbc (case-insensitive extension; segment is one ASCII letter when present)
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
		// standard CAESYYMM
	case 9:
		last := base[8]
		if !isASCIILetter(last) {
			return ParsedFilename{}, fmt.Errorf("expected segment suffix to be a single ASCII letter, got %q in %q", string(last), origBase)
		}
		segment = string(last)
		base = base[:8]
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

	// Two-digit year expansion:
	// Current 2-digit year and below → 2000s; above → 1900s.
	currentTwoDigit := time.Now().Year() % 100
	fullYear := 2000 + year
	if year > currentTwoDigit {
		fullYear = 1900 + year
	}

	return ParsedFilename{
		Catalog: catalog,
		State:   state,
		Year:    fullYear,
		Month:   month,
		Segment: segment,
	}, nil
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
