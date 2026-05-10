package errlog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExeDir returns the directory containing the current executable.
func ExeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// OpenAppLog opens logs/app.log for append under exeDir.
func OpenAppLog(exeDir string) (*os.File, error) {
	if strings.TrimSpace(exeDir) == "" {
		return nil, fmt.Errorf("exeDir is empty")
	}
	dir := filepath.Join(exeDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "app.log")
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// LogWriter returns stderr plus logs/app.log when exeDir is set and the file opens; close must be called on shutdown.
func LogWriter(exeDir string) (io.Writer, func()) {
	if strings.TrimSpace(exeDir) == "" {
		return os.Stderr, func() {}
	}
	f, err := OpenAppLog(exeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "errlog: open logs/app.log: %v\n", err)
		return os.Stderr, func() {}
	}
	return io.MultiWriter(os.Stderr, f), func() { _ = f.Close() }
}

// WriteErrorLog creates {exeDir}/logs/error-<utc-timestamp>.log with phase and err.
func WriteErrorLog(exeDir, phase string, err error) (string, error) {
	if err == nil {
		return "", nil
	}
	dir := filepath.Join(exeDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC()
	name := fmt.Sprintf("error-%s-%09d.log", ts.Format("20060102-150405"), ts.Nanosecond())
	path := filepath.Join(dir, name)
	body := fmt.Sprintf("time_utc=%s\nphase=%s\n\n%v\n", ts.Format(time.RFC3339Nano), phase, err)
	if werr := os.WriteFile(path, []byte(body), 0o644); werr != nil {
		return "", werr
	}
	return path, nil
}

// Report logs err to lg and appends a file under exeDir/logs when exeDir is non-empty.
func Report(lg *slog.Logger, exeDir, phase string, err error) {
	if err == nil {
		return
	}
	if lg == nil {
		lg = slog.Default()
	}
	if exeDir != "" {
		path, werr := WriteErrorLog(exeDir, phase, err)
		if werr != nil {
			lg.Error(phase, "err", err, "errlog_write", werr)
			return
		}
		lg.Error(phase, "err", err, "errlog_path", path)
		return
	}
	lg.Error(phase, "err", err)
}
