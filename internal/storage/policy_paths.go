package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	windowsDriveRe = regexp.MustCompile(`^[a-zA-Z]:\\`)
	uncPrefixRe    = regexp.MustCompile(`^\\\\`)
	invalidCharRe  = regexp.MustCompile(`[:*?"<>|]`)
)

type ProcessingDirectories struct {
	DownloadDir *string `json:"download_dir,omitempty"`
	CSVDir      *string `json:"csv_dir,omitempty"`
	ParquetDir  *string `json:"parquet_dir,omitempty"`
}

func normalizeSlashes(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\\`) {
		p := strings.ReplaceAll(path, "/", `\`)
		p = strings.TrimLeft(p, `\`)
		p = `\\` + collapseBackslashes(p)
		return trimPathSuffix(p)
	}

	if looksWindowsPath(path) {
		p := strings.ReplaceAll(path, "/", `\`)
		p = collapseBackslashes(p)
		return trimPathSuffix(p)
	}

	return filepath.Clean(path)
}

func collapseBackslashes(path string) string {
	var b strings.Builder
	lastSlash := false
	for _, r := range path {
		if r == '\\' {
			if lastSlash {
				continue
			}
			lastSlash = true
			b.WriteRune(r)
			continue
		}
		lastSlash = false
		b.WriteRune(r)
	}
	return b.String()
}

func trimPathSuffix(path string) string {
	if strings.HasPrefix(path, `\\`) {
		parts := strings.Split(strings.TrimPrefix(path, `\\`), `\`)
		if len(parts) <= 2 {
			return `\\` + strings.Join(parts, `\`)
		}
		return strings.TrimRight(path, `\`)
	}
	if len(path) == 3 && windowsDriveRe.MatchString(path) && strings.HasSuffix(path, `\`) {
		return path
	}
	return strings.TrimRight(path, `\`)
}

func looksWindowsPath(path string) bool {
	return windowsDriveRe.MatchString(strings.ReplaceAll(path, "/", `\`)) || strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\\`)
}

func validateWindowsPath(path string) error {
	if strings.Contains(path, "/") {
		return fmt.Errorf("use backslashes for windows paths")
	}
	if strings.HasPrefix(path, `\\`) {
		parts := strings.Split(strings.TrimPrefix(path, `\\`), `\`)
		if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid UNC path")
		}
		for _, p := range parts {
			if p == "" {
				return fmt.Errorf("invalid UNC path segment")
			}
			if invalidCharRe.MatchString(strings.ReplaceAll(p, ":", "")) {
				return fmt.Errorf("invalid characters in UNC path")
			}
		}
		return nil
	}

	if !windowsDriveRe.MatchString(path) {
		if regexp.MustCompile(`^[a-zA-Z]:[^\\]`).MatchString(path) {
			return fmt.Errorf("invalid local path format, expected C:\\dir")
		}
		return fmt.Errorf("invalid local path format")
	}
	segments := strings.Split(path[3:], `\`)
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("invalid repeated separators in path")
		}
		if invalidCharRe.MatchString(strings.ReplaceAll(seg, ":", "")) {
			return fmt.Errorf("invalid characters in path")
		}
	}
	return nil
}

func validateUnixPath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid path format")
	}
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("invalid path format")
	}
	return nil
}

func NormalizeAndValidatePolicyPath(raw string) (string, error) {
	normalized := normalizeSlashes(raw)
	if normalized == "" {
		return "", nil
	}
	if looksWindowsPath(normalized) {
		if err := validateWindowsPath(normalized); err != nil {
			return "", err
		}
		return normalized, nil
	}
	if err := validateUnixPath(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func normalizeOptionalPath(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := NormalizeAndValidatePolicyPath(*raw)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}
	return &value, nil
}

func NormalizePolicyDirectories(in ProcessingDirectories) (ProcessingDirectories, error) {
	out := ProcessingDirectories{}
	var err error
	out.DownloadDir, err = normalizeOptionalPath(in.DownloadDir)
	if err != nil {
		return out, fmt.Errorf("download_dir: %w", err)
	}
	out.CSVDir, err = normalizeOptionalPath(in.CSVDir)
	if err != nil {
		return out, fmt.Errorf("csv_dir: %w", err)
	}
	out.ParquetDir, err = normalizeOptionalPath(in.ParquetDir)
	if err != nil {
		return out, fmt.Errorf("parquet_dir: %w", err)
	}
	return out, nil
}

func ResolveDirectory(configured *string, fallback string) string {
	if configured == nil || strings.TrimSpace(*configured) == "" {
		return fallback
	}
	return *configured
}

func ValidateDirectoryAccess(path string, requireWrite bool) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is empty")
	}
	// In Linux dev containers, Windows-style paths are not meaningful.
	// Enforce accessibility in production host runtime.
	if runtime.GOOS != "windows" && looksWindowsPath(path) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}
	if requireWrite {
		probe := filepath.Join(path, ".datasus_write_probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			return err
		}
		_ = os.Remove(probe)
	}
	return nil
}
