package ftp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"datasus/internal/domain"
	"datasus/internal/observability"
	"datasus/internal/queue"
	"datasus/internal/repository"
)

// scanDirBatch is the batched/pipelined directory scan. It collapses what was
// previously O(N) database round trips into a small constant number per scan
// by computing the diff in memory against a snapshot of the catalog.
func (s *Scanner) scanDirBatch(ctx context.Context, dir string) (ScanResult, error) {
	listStart := time.Now()
	entries, err := s.client.ListDir(ctx, dir)
	if err != nil {
		return ScanResult{Dir: dir}, err
	}
	observability.FTPScanPhaseDuration.WithLabelValues("list").Observe(time.Since(listStart).Seconds())

	return s.processEntriesBatch(ctx, dir, entries)
}

// processEntriesBatch runs phases 2..5 (parse, snapshot, diff, persist) for a
// pre-fetched list of FTP entries. Exposed as a separate function so tests can
// drive the pipeline with synthetic entries without standing up an FTP server.
func (s *Scanner) processEntriesBatch(ctx context.Context, dir string, entries []Entry) (ScanResult, error) {
	result := ScanResult{Dir: dir}
	type validEntryT struct {
		upperName string
		entry     Entry
		parsed    domain.ParsedFilename
	}
	valid := make([]validEntryT, 0, len(entries))
	for _, e := range entries {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if !strings.EqualFold(filepath.Ext(e.Name), ".dbc") {
			continue
		}
		parsed, perr := domain.ParseFilename(e.Name)
		if perr != nil {
			result.Skipped++
			s.log.Warn("skipping ftp file with invalid filename format",
				"filename", e.Name, "dir", dir, "error", perr)
			continue
		}
		valid = append(valid, validEntryT{
			upperName: strings.ToUpper(e.Name),
			entry:     e,
			parsed:    parsed,
		})
	}
	result.Found = len(valid)
	if len(valid) == 0 {
		return result, nil
	}

	// Phase 3: load catalog snapshot, policy snapshot, processing root.
	snapStart := time.Now()
	dbSnap, err := s.fileRepo.ListSnapshotByFTPDir(ctx, dir)
	if err != nil {
		return result, fmt.Errorf("snapshot ftp dir: %w", err)
	}
	observability.FTPScanDBRoundtrips.Inc()

	var polSnap repository.PolicySnapshot
	if s.policy != nil {
		polSnap, err = s.policy.LoadPolicySnapshot(ctx)
		if err != nil {
			return result, fmt.Errorf("load policy: %w", err)
		}
		observability.FTPScanDBRoundtrips.Add(3)
	}

	rootPath := s.rootPath
	if s.policy != nil {
		dirs, derr := s.policy.ProcessingDirectories(ctx)
		observability.FTPScanDBRoundtrips.Inc()
		if derr == nil && dirs.DownloadDir != nil && strings.TrimSpace(*dirs.DownloadDir) != "" {
			rootPath = *dirs.DownloadDir
		}
	}
	observability.FTPScanPhaseDuration.WithLabelValues("snapshot").Observe(time.Since(snapStart).Seconds())

	// Phase 4: in-memory diff.
	diffStart := time.Now()
	type kind int
	const (
		kindNew kind = iota
		kindChanged
		kindUnchanged
	)
	type classified struct {
		upperName     string
		entry         Entry
		parsed        domain.ParsedFilename
		k             kind
		existingID    string
		prevStatus    domain.OverallStatus
		allowByPolicy bool
	}
	classifieds := make([]classified, 0, len(valid))
	upserts := make([]repository.UpsertFTPParams, 0, len(valid))
	for _, ve := range valid {
		c := classified{
			upperName: ve.upperName,
			entry:     ve.entry,
			parsed:    ve.parsed,
		}
		if existing, ok := dbSnap[ve.upperName]; ok {
			c.existingID = existing.ID
			c.prevStatus = existing.OverallStatus
			if isUnchanged(existing, ve.entry) {
				c.k = kindUnchanged
			} else {
				c.k = kindChanged
			}
		} else {
			c.k = kindNew
			c.prevStatus = domain.StatusPending
		}
		if s.policy == nil {
			c.allowByPolicy = true
		} else {
			c.allowByPolicy = polSnap.Allows(ve.parsed.Catalog, ve.parsed.Year, ve.parsed.Month)
		}
		if c.k != kindUnchanged {
			upserts = append(upserts, buildUpsertParams(ve.entry, ve.parsed, dir, rootPath))
		}
		classifieds = append(classifieds, c)
	}
	observability.FTPScanPhaseDuration.WithLabelValues("diff").Observe(time.Since(diffStart).Seconds())

	// Phase 5: persistence in batches.
	persistStart := time.Now()

	nameToID := make(map[string]string, len(upserts))
	batchSize := s.batchSize
	if batchSize <= 0 {
		batchSize = 1000
	}
	for start := 0; start < len(upserts); start += batchSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		end := start + batchSize
		if end > len(upserts) {
			end = len(upserts)
		}
		chunk := upserts[start:end]
		res, err := s.fileRepo.BulkUpsertFromFTP(ctx, chunk)
		observability.FTPScanDBRoundtrips.Inc()
		if err != nil {
			s.log.Warn("bulk upsert failed, falling back to per-row",
				"err", err, "size", len(chunk), "dir", dir)
			s.fallbackUpsertChunk(ctx, chunk, nameToID, &result)
			continue
		}
		for i, id := range res.IDs {
			nameToID[chunk[i].Filename] = id
		}
	}

	now := time.Now()
	var unchangedTouchIDs []string
	var ignoreIDs []string
	var initStageIDs []string
	var enqItems []queue.EnqueueItem
	for _, c := range classifieds {
		var id string
		switch c.k {
		case kindUnchanged:
			id = c.existingID
			unchangedTouchIDs = append(unchangedTouchIDs, id)
		default:
			id = nameToID[c.upperName]
			if id == "" {
				continue
			}
		}
		contentChanged := c.k != kindUnchanged
		stage, shouldEnqueue := nextStageFor(c.prevStatus, contentChanged)
		switch {
		case !c.allowByPolicy:
			result.SkippedByPolicy++
			if shouldMoveToIgnored(c.prevStatus) {
				ignoreIDs = append(ignoreIDs, id)
			}
		case shouldEnqueue:
			initStageIDs = append(initStageIDs, id)
			enqItems = append(enqItems, queue.EnqueueItem{
				FileID: id, Stage: stage, AvailableAt: now,
			})
			result.Enqueued++
		}
		switch c.k {
		case kindUnchanged:
			result.Skipped++
		case kindNew:
			result.New++
		case kindChanged:
			result.Changed++
		}
	}

	if err := s.runChunkedIDs(ctx, unchangedTouchIDs, batchSize, "touch last_seen", &result, func(ids []string) error {
		err := s.fileRepo.TouchLastSeen(ctx, ids)
		observability.FTPScanDBRoundtrips.Inc()
		return err
	}); err != nil {
		return result, err
	}

	if err := s.runChunkedIDs(ctx, ignoreIDs, batchSize, "bulk ignore", &result, func(ids []string) error {
		_, err := s.fileRepo.BulkSetIgnoredByPolicy(ctx, ids)
		observability.FTPScanDBRoundtrips.Inc()
		return err
	}); err != nil {
		return result, err
	}

	if err := s.runChunkedIDs(ctx, initStageIDs, batchSize, "bulk init stages", &result, func(ids []string) error {
		err := s.stageRepo.BulkInitStages(ctx, ids)
		observability.FTPScanDBRoundtrips.Inc()
		return err
	}); err != nil {
		return result, err
	}

	for start := 0; start < len(enqItems); start += batchSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		end := start + batchSize
		if end > len(enqItems) {
			end = len(enqItems)
		}
		chunk := enqItems[start:end]
		if err := s.queue.BulkEnqueue(ctx, chunk); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("bulk enqueue: %v", err))
		}
		observability.FTPScanDBRoundtrips.Inc()
	}

	observability.FTPScanFilesUnchanged.Add(float64(len(unchangedTouchIDs)))
	observability.FTPScanFilesChanged.Add(float64(result.Changed))
	observability.FTPScanFilesInserted.Add(float64(result.New))
	observability.FTPScanPhaseDuration.WithLabelValues("persist").Observe(time.Since(persistStart).Seconds())

	return result, nil
}

func (s *Scanner) runChunkedIDs(
	ctx context.Context,
	ids []string,
	batchSize int,
	label string,
	result *ScanResult,
	fn func([]string) error,
) error {
	for start := 0; start < len(ids); start += batchSize {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := fn(ids[start:end]); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", label, err))
		}
	}
	return nil
}

func (s *Scanner) fallbackUpsertChunk(
	ctx context.Context,
	chunk []repository.UpsertFTPParams,
	nameToID map[string]string,
	result *ScanResult,
) {
	for _, p := range chunk {
		if ctx.Err() != nil {
			return
		}
		f, _, err := s.fileRepo.UpsertFromFTP(ctx, p)
		observability.FTPScanDBRoundtrips.Inc()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", p.Filename, err))
			continue
		}
		nameToID[p.Filename] = f.ID
	}
}

func isUnchanged(existing repository.FileSnapshotRow, e Entry) bool {
	if existing.SizeBytes == nil || existing.RemoteTimestamp == nil {
		return false
	}
	if *existing.SizeBytes != e.Size {
		return false
	}
	return existing.RemoteTimestamp.Equal(e.ModTime)
}

func buildUpsertParams(e Entry, parsed domain.ParsedFilename, dir, rootPath string) repository.UpsertFTPParams {
	var segPtr *string
	if parsed.Segment != "" {
		s := parsed.Segment
		segPtr = &s
	}
	size := e.Size
	mod := e.ModTime
	return repository.UpsertFTPParams{
		Filename:        strings.ToUpper(e.Name),
		Catalog:         parsed.Catalog,
		State:           parsed.State,
		Year:            parsed.Year,
		Month:           parsed.Month,
		Segment:         segPtr,
		FTPDir:          dir,
		FTPPath:         e.RemotePath,
		SizeBytes:       &size,
		RemoteChecksum:  nil,
		RemoteTimestamp: &mod,
		RootPath:        rootPath,
	}
}
