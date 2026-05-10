package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// FileStatSize returns the size in bytes of path.
func FileStatSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// FileSHA256Hex returns the lowercase hex SHA-256 of the file at path.
func FileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FileStatAndSHA256Hex returns both the size and SHA-256 hash of a file in a single operation.
// This is more efficient than calling FileStatSize and FileSHA256Hex separately.
func FileStatAndSHA256Hex(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	// Get file info while it's open
	st, err := f.Stat()
	if err != nil {
		return 0, "", err
	}

	// Compute hash while reading the file
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", fmt.Errorf("read file: %w", err)
	}

	return st.Size(), hex.EncodeToString(h.Sum(nil)), nil
}
