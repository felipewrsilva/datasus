package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"converterParquet/internal/config"
	"converterParquet/internal/convert"
	"converterParquet/internal/domain"
	"converterParquet/internal/hash"
	"converterParquet/internal/storage"
	"converterParquet/internal/store"
	"converterParquet/internal/walk"
)

const (
	scanProgressEveryDBC        = 500
	conversionHeartbeatInterval = 60 * time.Second
)

type groupKey struct {
	Catalog     string
	State       string
	Year        int
	Month       int
	LogicalBase string
}

type fileEntry struct {
	AbsPath       string
	RelPath       string
	Parsed        domain.ParsedFilename
	LogicalBase   string
	SegmentLetter string // uppercased single letter or ""
}

type parquetWork struct {
	key   groupKey
	group []fileEntry
}

// Run scans, groups, converts, and logs. st may be nil when dryRun.
// Use one process per output_folder so concurrent runs do not race on .partial / final artifacts.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger, st *store.Store, dryRun bool) error {
	sourceRoot, err := filepath.Abs(cfg.SourceFolder)
	if err != nil {
		return fmt.Errorf("source_folder: %w", err)
	}
	outRoot, err := filepath.Abs(cfg.OutputFolder)
	if err != nil {
		return fmt.Errorf("output_folder: %w", err)
	}
	log.Info("run",
		"source_root", sourceRoot,
		"output_root", outRoot,
		"scan_subfolders", cfg.ScanSubfolders,
		"max_scan_depth", cfg.MaxScanDepth,
		"dry_run", dryRun,
	)

	// Pre-flight validation: clean up partial files and check state
	if !dryRun {
		if err := preFlightCheck(log, outRoot, st, ctx); err != nil {
			log.Warn("preflight check warning (continuing anyway)", "err", err)
		}
	}

	log.Info("scanning for .dbc files",
		"source_root", sourceRoot,
		"scan_subfolders", cfg.ScanSubfolders,
		"max_scan_depth", cfg.MaxScanDepth,
	)
	scanStart := time.Now()
	var entries []fileEntry
	err = walk.ListDBC(sourceRoot, cfg.ScanSubfolders, cfg.MaxScanDepth, func(abs, rel string) error {
		parsed, perr := domain.ParseFilename(filepath.Base(abs))
		if perr != nil {
			log.Warn("skip unparseable dbc", "path", abs, "err", perr)
			return nil
		}
		lb, lerr := domain.LogicalBaseStem(abs)
		if lerr != nil {
			log.Warn("skip logical base", "path", abs, "err", lerr)
			return nil
		}
		seg := ""
		if parsed.Segment != "" {
			seg = strings.ToUpper(parsed.Segment)
		}
		entries = append(entries, fileEntry{
			AbsPath:       abs,
			RelPath:       rel,
			Parsed:        parsed,
			LogicalBase:   lb,
			SegmentLetter: seg,
		})
		if len(entries)%scanProgressEveryDBC == 0 {
			log.Info("scan progress",
				"dbc_files_found", len(entries),
				"elapsed", time.Since(scanStart).Round(time.Second).String(),
			)
		}
		return nil
	})
	if err != nil {
		return err
	}
	log.Info("scan walk finished",
		"dbc_files_found", len(entries),
		"elapsed", time.Since(scanStart).Round(time.Second).String(),
	)

	byKey := make(map[groupKey][]fileEntry)
	for _, e := range entries {
		k := groupKey{
			Catalog:     e.Parsed.Catalog,
			State:       e.Parsed.State,
			Year:        e.Parsed.Year,
			Month:       e.Parsed.Month,
			LogicalBase: e.LogicalBase,
		}
		byKey[k] = append(byKey[k], e)
	}

	keys := make([]groupKey, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Catalog != b.Catalog {
			return a.Catalog < b.Catalog
		}
		if a.State != b.State {
			return a.State < b.State
		}
		if a.Year != b.Year {
			return a.Year < b.Year
		}
		if a.Month != b.Month {
			return a.Month < b.Month
		}
		return a.LogicalBase < b.LogicalBase
	})

	wopt := convert.WriterOptions{
		ParallelWriters: int64(cfg.ParquetParallelWriters),
		RowGroupSize:    int64(cfg.ParquetRowGroupMB) * 1024 * 1024,
		PageSize:        int64(cfg.ParquetPageKB) * 1024,
	}

	var work []parquetWork
	for _, k := range keys {
		group := byKey[k]
		if err := sortGroupEntries(&group); err != nil {
			return err
		}
		if err := validateGroup(group, cfg, log); err != nil {
			return err
		}
		work = append(work, parquetWork{key: k, group: group})
	}

	log.Info("scan complete", "dbc_files", len(entries), "groups", len(work))
	if len(work) == 0 {
		log.Info("no .dbc groups to convert (check source path, filters, and max_scan_depth)")
	}

	if dryRun {
		for _, w := range work {
			if err := dryRunOne(ctx, log, outRoot, w); err != nil {
				return err
			}
		}
		return nil
	}

	return runParallel(ctx, cfg, log, st, outRoot, wopt, work)
}

// preFlightCheck validates state and cleans up before conversion starts
func preFlightCheck(log *slog.Logger, outRoot string, st *store.Store, ctx context.Context) error {
	// Ensure output folder exists
	if err := os.MkdirAll(outRoot, 0755); err != nil {
		return fmt.Errorf("create output folder: %w", err)
	}

	// Remove only VERY old partial files (older than 2 hours) to avoid interfering with ongoing conversions.
	// Recent partial files are likely being worked on and should be resumed.
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	removed := 0
	if err := filepath.WalkDir(outRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".partial") {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if info.ModTime().Before(twoHoursAgo) {
			log.Warn("removing stale partial file (older than 2 hours)", "path", path, "mtime", info.ModTime())
			if removeErr := os.Remove(path); removeErr != nil {
				log.Warn("failed to remove partial file", "path", path, "err", removeErr)
			} else {
				removed++
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("cleanup partial files: %w", err)
	}

	log.Info("preflight check complete", "output_root", outRoot, "stale_partials_removed", removed)
	return nil
}

func dryRunOne(ctx context.Context, log *slog.Logger, outRoot string, w parquetWork) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k := w.key
	group := w.group
	outPath := storage.ParquetPath(outRoot, k.Year, k.LogicalBase)
	parts := make([]hash.Part, len(group))
	dbcPaths := make([]string, len(group))
	for i, g := range group {
		parts[i] = hash.Part{
			RelativePath: filepath.ToSlash(g.RelPath),
			Segment:      g.SegmentLetter,
			AbsPath:      g.AbsPath,
		}
		dbcPaths[i] = g.AbsPath
	}
	fp, err := hash.InputFingerprintSHA256(parts)
	if err != nil {
		return fmt.Errorf("fingerprint %s: %w", outPath, err)
	}
	log.Info("dry-run", "output", outPath, "sources", dbcPaths, "fingerprint", fp)
	return nil
}

func runParallel(ctx context.Context, cfg *config.Config, log *slog.Logger, st *store.Store, outRoot string, wopt convert.WriterOptions, work []parquetWork) error {
	if len(work) == 0 {
		return nil
	}
	n := cfg.ParallelWorkers
	log.Info("conversion starting", "groups", len(work), "parallel_workers", n)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan parquetWork, len(work))
	go func() {
		defer close(jobs)
		for _, w := range work {
			select {
			case <-runCtx.Done():
				return
			case jobs <- w:
			}
		}
	}()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var completed atomic.Uint64

	heartbeatStop := make(chan struct{})
	go func() {
		t := time.NewTicker(conversionHeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				log.Info("conversion still running",
					"completed_groups", completed.Load(),
					"total_groups", len(work),
					"parallel_workers", n,
				)
			}
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range jobs {
				if runCtx.Err() != nil {
					return
				}
				err := convertOne(runCtx, cfg, log, st, outRoot, wopt, w)
				completed.Add(1)
				if err == nil {
					continue
				}
				// Another worker failed and canceled runCtx; keep the root cause, not derived cancellation.
				if errors.Is(err, context.Canceled) && ctx.Err() == nil {
					return
				}
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				cancel()
				return
			}
		}()
	}
	wg.Wait()
	close(heartbeatStop)
	if firstErr != nil {
		return firstErr
	}
	if ctx.Err() == nil {
		log.Info("conversion pass complete", "groups", len(work))
	}
	return ctx.Err()
}

func convertOne(ctx context.Context, cfg *config.Config, log *slog.Logger, st *store.Store, outRoot string, wopt convert.WriterOptions, w parquetWork) error {
	k := w.key
	group := w.group
	outPath := storage.ParquetPath(outRoot, k.Year, k.LogicalBase)

	parts := make([]hash.Part, len(group))
	dbcPaths := make([]string, len(group))
	for i, g := range group {
		parts[i] = hash.Part{
			RelativePath: filepath.ToSlash(g.RelPath),
			Segment:      g.SegmentLetter,
			AbsPath:      g.AbsPath,
		}
		dbcPaths[i] = g.AbsPath
	}

	fp, err := hash.InputFingerprintSHA256(parts)
	if err != nil {
		return fmt.Errorf("fingerprint %s: %w", outPath, err)
	}

	if st != nil {
		art, aerr := st.GetArtifactByOutputPath(ctx, outPath)
		if aerr != nil {
			return aerr
		}
		if art != nil && art.InputFingerprint == fp && art.ParquetSha256.Valid {
			if _, statErr := os.Stat(outPath); statErr == nil {
				cur, herr := storage.FileSHA256Hex(outPath)
				if herr == nil && cur == strings.TrimSpace(art.ParquetSha256.String) {
					log.Info("skip up to date", "output", outPath)
					return nil
				}
			}
		}
	} else {
		if _, statErr := os.Stat(outPath); statErr == nil {
			log.Info("skip existing output (no SQL idempotency)", "output", outPath)
			return nil
		}
	}

	if err := storage.RemovePartialIfExists(outPath); err != nil {
		return fmt.Errorf("remove partial %s: %w", outPath, err)
	}

	log.Info("converting",
		"output", outPath,
		"logical_base", k.LogicalBase,
		"catalog", k.Catalog,
		"state", k.State,
		"year", k.Year,
		"month", k.Month,
		"dbc_sources", len(dbcPaths),
	)
	timeout := time.Duration(cfg.ConvertTimeoutS) * time.Second
	started := time.Now().UTC()
	dataRows, convErr := convert.MergeDBCsToParquet(ctx, outPath, dbcPaths, timeout, wopt)
	finished := time.Now().UTC()

	srcJSON, _ := json.Marshal(dbcPaths)
	srcJSONStr := string(srcJSON)

	if convErr != nil {
		if errors.Is(convErr, context.Canceled) {
			return context.Canceled
		}
		msg := convErr.Error()
		log.Error("convert failed", "output", outPath, "err", convErr)
		if st != nil {
			_ = st.InsertRun(ctx, started, finished, outPath, false, &msg, nil, srcJSONStr, &fp, nil)
			_ = st.UpsertArtifact(ctx, outPath, k.Catalog, k.State, k.Year, k.Month, k.LogicalBase, fp, "", "failed", &msg)
		}
		return convErr
	}

	pqHex, herr := storage.FileSHA256Hex(outPath)
	if herr != nil {
		msg := herr.Error()
		if st != nil {
			_ = st.InsertRun(ctx, started, finished, outPath, false, &msg, nil, srcJSONStr, &fp, nil)
			_ = st.UpsertArtifact(ctx, outPath, k.Catalog, k.State, k.Year, k.Month, k.LogicalBase, fp, "", "failed", &msg)
		}
		return fmt.Errorf("hash output: %w", herr)
	}

	if st != nil {
		if err := st.InsertRun(ctx, started, finished, outPath, true, nil, &dataRows, srcJSONStr, &fp, &pqHex); err != nil {
			return err
		}
		if err := st.UpsertArtifact(ctx, outPath, k.Catalog, k.State, k.Year, k.Month, k.LogicalBase, fp, pqHex, "success", nil); err != nil {
			return err
		}
	}
	log.Info("converted", "output", outPath, "rows", dataRows, "sources", len(dbcPaths))
	return nil
}

func sortGroupEntries(group *[]fileEntry) error {
	g := *group
	sort.Slice(g, func(i, j int) bool {
		si, sj := strings.ToLower(g[i].SegmentLetter), strings.ToLower(g[j].SegmentLetter)
		if si != sj {
			return si < sj
		}
		if g[i].SegmentLetter != g[j].SegmentLetter {
			return g[i].SegmentLetter < g[j].SegmentLetter
		}
		return g[i].RelPath < g[j].RelPath
	})
	*group = g
	return nil
}

func validateGroup(group []fileEntry, cfg *config.Config, log *slog.Logger) error {
	var unseg int
	var seg int
	seenUnsegPath := make(map[string]struct{})
	for _, e := range group {
		if e.SegmentLetter == "" {
			unseg++
			if _, dup := seenUnsegPath[e.AbsPath]; dup {
				return fmt.Errorf("duplicate path in group %s", e.LogicalBase)
			}
			seenUnsegPath[e.AbsPath] = struct{}{}
		} else {
			seg++
		}
	}
	if unseg > 1 {
		return fmt.Errorf("ambiguous group for base %s: multiple unsegmented .dbc files", group[0].LogicalBase)
	}
	if unseg == 1 && seg > 0 {
		return fmt.Errorf("conflict for base %s: both unsegmented and segmented .dbc present", group[0].LogicalBase)
	}
	if cfg.StrictSegments && seg > 1 {
		if err := checkSegmentGaps(group); err != nil {
			return err
		}
	} else if seg > 1 {
		if warn := segmentGapWarning(group); warn != "" {
			log.Warn("incomplete segment set", "base", group[0].LogicalBase, "detail", warn)
		}
	}
	return nil
}

func checkSegmentGaps(group []fileEntry) error {
	letters := segmentLetters(group)
	if len(letters) <= 1 {
		return nil
	}
	for i := 0; i < len(letters)-1; i++ {
		for c := letters[i] + 1; c < letters[i+1]; c++ {
			return fmt.Errorf("strict_segments: missing segment %q between %q and %q for base %s",
				string(c), string(letters[i]), string(letters[i+1]), group[0].LogicalBase)
		}
	}
	return nil
}

func segmentGapWarning(group []fileEntry) string {
	letters := segmentLetters(group)
	if len(letters) <= 1 {
		return ""
	}
	var missing []rune
	for i := 0; i < len(letters)-1; i++ {
		for c := letters[i] + 1; c < letters[i+1]; c++ {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("missing letters between min and max: %q", string(missing))
}

func segmentLetters(group []fileEntry) []rune {
	var letters []rune
	for _, e := range group {
		if e.SegmentLetter == "" {
			continue
		}
		if len(e.SegmentLetter) != 1 {
			continue
		}
		letters = append(letters, rune(e.SegmentLetter[0]))
	}
	sort.Slice(letters, func(i, j int) bool { return letters[i] < letters[j] })
	return letters
}
