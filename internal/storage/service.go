package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EnsureDir creates all directories in path if they don't exist.
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir %q: %w", dir, err)
	}
	return nil
}

// MoveFile moves src to dst, creating dst directories as needed.
// If src and dst are on different filesystems, falls back to copy+delete.
func MoveFile(src, dst string) error {
	if err := EnsureDir(dst); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// cross-device fallback
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// DeleteFile removes a file from disk. Returns nil if the file does not exist.
func DeleteFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// FileSize returns the size of a file in bytes, or 0 if it does not exist.
func FileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst %q: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q → %q: %w", src, dst, err)
	}
	return out.Sync()
}
