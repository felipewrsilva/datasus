package runner

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"downloader/internal/config"
	"downloader/internal/domain"
	"downloader/internal/ftpclient"
	"downloader/internal/store"
)

const ftpListProgressEvery = 20

type jobOutcome int

const (
	jobOutcomeNone jobOutcome = iota
	jobOutcomeSkippedUpToDate
	jobOutcomeDownloaded
	jobOutcomeReconciled
)

// Job is one .dbc file to evaluate and optionally download.
type Job struct {
	Entry  ftpclient.Entry
	FTPDir string
}

// Run executes one full scan-and-download pass.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger, st *store.Store, ftp *ftpclient.Client) error {
	catalogs := cfg.CatalogSet()
	minY, maxY := cfg.YearMinMax(time.Now())
	localRoot, err := filepath.Abs(cfg.Download.LocalRoot)
	if err != nil {
		return fmt.Errorf("download local_root: %w", err)
	}
	log.Info("download run",
		"ftp_host", cfg.FTP.Host,
		"local_root", localRoot,
		"year_min", minY,
		"year_max", maxY,
		"catalog_count", len(catalogs),
		"ftp_scan_paths", len(cfg.FTP.Paths),
		"ftp_scan_max_depth", cfg.FTP.ScanMaxDepth,
	)

	catalogSourceCounts := make(map[string]map[string]int, len(catalogs))
	for c := range catalogs {
		catalogSourceCounts[c] = make(map[string]int)
	}

	log.Info("ftp expand starting", "roots", cfg.FTP.Paths, "scan_max_depth", cfg.FTP.ScanMaxDepth)
	expanded, err := ftpclient.ExpandScanDirs(ctx, ftp, cfg.FTP.Paths, cfg.FTP.ScanMaxDepth)
	if err != nil {
		return fmt.Errorf("expand ftp dirs: %w", err)
	}
	log.Info("ftp directory expand complete", "directories", len(expanded))

	var jobs []Job
	for i, dir := range expanded {
		listCtx, cancel := context.WithTimeout(ctx, cfg.FTP.ScanTimeoutParsed)
		entries, err := ftp.ListDir(listCtx, dir)
		cancel()
		if err != nil {
			return fmt.Errorf("list %q: %w", dir, err)
		}
		for _, e := range entries {
			parsed, err := domain.ParseFilename(e.Name)
			if err != nil {
				log.Debug("skip non-matching filename", "name", e.Name, "err", err)
				continue
			}
			if _, ok := catalogs[parsed.Catalog]; !ok {
				continue
			}
			if parsed.Year < minY || parsed.Year > maxY {
				continue
			}
			catalogSourceCounts[parsed.Catalog][dir]++
			jobs = append(jobs, Job{Entry: e, FTPDir: dir})
		}
		listed := i + 1
		if listed%ftpListProgressEvery == 0 || listed == len(expanded) {
			log.Info("ftp list progress",
				"listed_dirs", listed,
				"total_dirs", len(expanded),
				"jobs_so_far", len(jobs),
			)
		}
	}
	logCatalogSourceMap(log, catalogSourceCounts)

	log.Info("download pass", "candidates", len(jobs), "ftp_dirs", len(expanded))
	if len(jobs) == 0 {
		log.Info("nothing to download", "reason", "no ftp files matched configured catalogs/year range")
	}

	if err := os.MkdirAll(cfg.Download.LocalRoot, 0o755); err != nil {
		return fmt.Errorf("local root: %w", err)
	}

	workers := cfg.Download.ParallelWorkers
	if workers < 1 {
		workers = 1
	}
	if len(jobs) > 0 {
		log.Info("processing download jobs", "parallel_workers", workers)
	}
	ch := make(chan Job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	var downloaded, skippedUpToDate, reconciled atomic.Uint64

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				outcome, err := processJob(ctx, cfg, log, st, ftp, j)
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					continue
				}
				switch outcome {
				case jobOutcomeDownloaded:
					downloaded.Add(1)
				case jobOutcomeSkippedUpToDate:
					skippedUpToDate.Add(1)
				case jobOutcomeReconciled:
					reconciled.Add(1)
				}
			}
		}()
	}

enqueue:
	for _, j := range jobs {
		select {
		case <-ctx.Done():
			break enqueue
		case ch <- j:
		}
	}
	close(ch)
	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(jobs) > 0 {
		log.Info("download pass summary",
			"candidates", len(jobs),
			"downloaded", downloaded.Load(),
			"skipped_up_to_date", skippedUpToDate.Load(),
			"reconciled", reconciled.Load(),
			"job_errors", len(errs),
		)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d job error(s), first: %v", len(errs), errs[0])
	}
	if len(jobs) > 0 {
		log.Info("download pass complete", "candidates", len(jobs))
	}
	return nil
}

func logCatalogSourceMap(log *slog.Logger, catalogSourceCounts map[string]map[string]int) {
	catalogs := make([]string, 0, len(catalogSourceCounts))
	for c := range catalogSourceCounts {
		catalogs = append(catalogs, c)
	}
	sort.Strings(catalogs)

	for _, c := range catalogs {
		dirs := catalogSourceCounts[c]
		if len(dirs) == 0 {
			log.Info("catalog source map", "catalog", c, "sources", "none", "candidates", 0)
			continue
		}

		dirNames := make([]string, 0, len(dirs))
		for d := range dirs {
			dirNames = append(dirNames, d)
		}
		sort.Strings(dirNames)

		total := 0
		pairs := make([]string, 0, len(dirNames))
		for _, d := range dirNames {
			n := dirs[d]
			total += n
			pairs = append(pairs, fmt.Sprintf("%s (%d)", d, n))
		}

		log.Info("catalog source map", "catalog", c, "sources", strings.Join(pairs, "; "), "candidates", total)
	}
}

func processJob(ctx context.Context, cfg *config.Config, log *slog.Logger, st *store.Store, ftp *ftpclient.Client, j Job) (jobOutcome, error) {
	e := j.Entry
	parsed, err := domain.ParseFilename(e.Name)
	if err != nil {
		return jobOutcomeNone, nil
	}

	finalName := strings.ToUpper(e.Name)
	yearDir := filepath.Join(cfg.Download.LocalRoot, strconv.Itoa(parsed.Year))
	if err := os.MkdirAll(yearDir, 0o755); err != nil {
		return jobOutcomeNone, fmt.Errorf("mkdir %s: %w", yearDir, err)
	}
	localPath := filepath.Join(yearDir, finalName)

	ftpHost := cfg.FTP.Host
	remotePath := e.RemotePath
	remoteMod := sql.NullTime{}
	if !e.ModTime.IsZero() {
		remoteMod = sql.NullTime{Time: e.ModTime.UTC(), Valid: true}
	}

	var segPtr *string
	if parsed.Segment != "" {
		s := parsed.Segment
		segPtr = &s
	}

	if err := st.InsertRegistryIfMissing(ctx,
		ftpHost, remotePath, j.FTPDir, finalName,
		parsed.Catalog, parsed.State, parsed.Year, parsed.Month, segPtr,
		e.Size, remoteMod,
	); err != nil {
		return jobOutcomeNone, fmt.Errorf("registry insert %s: %w", remotePath, err)
	}

	row, err := st.GetRegistry(ctx, ftpHost, remotePath)
	if err != nil {
		return jobOutcomeNone, fmt.Errorf("registry get %s: %w", remotePath, err)
	}
	if row == nil {
		return jobOutcomeNone, fmt.Errorf("registry missing after insert: %s", remotePath)
	}

	need, reason := needDownload(row, e, localPath)
	if !need {
		if row.DTLastDownloadUTC.Valid {
			return jobOutcomeSkippedUpToDate, nil
		}
		if err := reconcileLocal(ctx, st, row, j, e, parsed, finalName, localPath, remoteMod, ftpHost, remotePath, "new", log); err != nil {
			return jobOutcomeNone, err
		}
		return jobOutcomeReconciled, nil
	}

	log.Info("download", "reason", reason, "remote", remotePath, "local", localPath)

	started := time.Now().UTC()
	dlErr := downloadToFile(ctx, ftp, remotePath, localPath, cfg.Download.TempSuffix)
	finished := time.Now().UTC()

	if dlErr != nil {
		msg := dlErr.Error()
		return jobOutcomeNone, st.RecordAttempt(ctx,
			row.ID, started, finished,
			ftpHost, remotePath,
			e.Size, remoteMod,
			reason,
			false,
			&msg,
			nil, nil,
			"", "", "", "", 0, 0, nil, "",
		)
	}

	sha, size, err := fileSHA256AndSize(localPath)
	if err != nil {
		msg := err.Error()
		return jobOutcomeNone, st.RecordAttempt(ctx,
			row.ID, started, finished,
			ftpHost, remotePath,
			e.Size, remoteMod,
			reason,
			false,
			&msg,
			&localPath, nil,
			"", "", "", "", 0, 0, nil, "",
		)
	}

	absLocal, err := filepath.Abs(localPath)
	if err != nil {
		absLocal = localPath
	}
	err = st.RecordAttempt(ctx,
		row.ID, started, finished,
		ftpHost, remotePath,
		e.Size, remoteMod,
		reason,
		true,
		nil,
		&absLocal, &size,
		j.FTPDir, finalName,
		parsed.Catalog, parsed.State, parsed.Year, parsed.Month, segPtr,
		sha,
	)
	if err != nil {
		return jobOutcomeNone, err
	}
	return jobOutcomeDownloaded, nil
}

func needDownload(row *store.RegistryRow, listing ftpclient.Entry, localPath string) (bool, string) {
	hasSuccessful := row.DTLastDownloadUTC.Valid

	if !hasSuccessful {
		st, err := os.Stat(localPath)
		if err == nil && st.Size() == listing.Size {
			return false, ""
		}
		if os.IsNotExist(err) {
			return true, "local_missing"
		}
		if err != nil {
			return true, "new"
		}
		if st.Size() != listing.Size {
			return true, "local_size_mismatch"
		}
		return true, "new"
	}

	if listing.Size != row.QTRemoteSize {
		return true, "remote_size_changed"
	}
	if row.DTRemoteModified.Valid && !listing.ModTime.IsZero() {
		if !mtimeEqual(row.DTRemoteModified.Time, listing.ModTime) {
			return true, "remote_mtime_changed"
		}
	}

	st, err := os.Stat(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, "local_missing"
		}
		return true, "local_missing"
	}
	if st.Size() != listing.Size {
		return true, "local_size_mismatch"
	}
	return false, ""
}

func mtimeEqual(a, b time.Time) bool {
	return a.UTC().Truncate(time.Millisecond).Equal(b.UTC().Truncate(time.Millisecond))
}

func downloadToFile(ctx context.Context, ftp *ftpclient.Client, remotePath, finalPath, tempSuffix string) error {
	tmp := finalPath + tempSuffix
	_ = os.Remove(tmp)

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	err = ftp.Download(ctx, remotePath, f)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close: %w", err)
	}

	if err := os.Rename(tmp, finalPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func fileSHA256AndSize(path string) (hexStr string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), st.Size(), nil
}

func reconcileLocal(
	ctx context.Context,
	st *store.Store,
	row *store.RegistryRow,
	j Job,
	e ftpclient.Entry,
	parsed domain.ParsedFilename,
	finalName, localPath string,
	remoteMod sql.NullTime,
	ftpHost, remotePath, reason string,
	log *slog.Logger,
) error {
	log.Info("reconcile local file with registry", "remote", remotePath, "local", localPath)
	started := time.Now().UTC()
	sha, size, err := fileSHA256AndSize(localPath)
	finished := time.Now().UTC()
	if err != nil {
		msg := err.Error()
		return st.RecordAttempt(ctx,
			row.ID, started, finished,
			ftpHost, remotePath,
			e.Size, remoteMod,
			reason,
			false,
			&msg,
			&localPath, nil,
			"", "", "", "", 0, 0, nil, "",
		)
	}
	absLocal, err := filepath.Abs(localPath)
	if err != nil {
		absLocal = localPath
	}
	var segPtr *string
	if parsed.Segment != "" {
		s := parsed.Segment
		segPtr = &s
	}
	return st.RecordAttempt(ctx,
		row.ID, started, finished,
		ftpHost, remotePath,
		e.Size, remoteMod,
		reason,
		true,
		nil,
		&absLocal, &size,
		j.FTPDir, finalName,
		parsed.Catalog, parsed.State, parsed.Year, parsed.Month, segPtr,
		sha,
	)
}
