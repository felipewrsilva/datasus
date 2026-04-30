package ftp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"datasus/internal/domain"
	"datasus/internal/observability"
	"datasus/internal/pipeline"
	"datasus/internal/queue"
	"datasus/internal/repository"
)

// Scanner walks configured FTP directories, updates discovery metadata, and enqueues pending processing.
type Scanner struct {
	client     *Client
	dirs       []string
	fileRepo   *repository.FileRepository
	stageRepo  *repository.StageRepository
	queue      *queue.PostgresQueue
	policy     *repository.PolicyRepository
	reconciler *pipeline.Reconciler
	rootPath   string
	log        *slog.Logger
	batchSize  int
	legacy     bool
}

func NewScanner(
	client *Client,
	dirs []string,
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	q *queue.PostgresQueue,
	policy *repository.PolicyRepository,
	rootPath string,
	log *slog.Logger,
) *Scanner {
	return &Scanner{
		client:     client,
		dirs:       dirs,
		fileRepo:   fileRepo,
		stageRepo:  stageRepo,
		queue:      q,
		policy:     policy,
		reconciler: pipeline.NewReconciler(fileRepo, stageRepo, q, policy),
		rootPath:   rootPath,
		log:        log,
		batchSize:  1000,
	}
}

// Configure tunes batch behavior and selects between the batched pipeline
// (default) and the legacy per-file path. Safe to call before Scan; ignored
// when called concurrently with a running scan.
func (s *Scanner) Configure(batchSize int, legacy bool) {
	if batchSize > 0 {
		s.batchSize = batchSize
	}
	s.legacy = legacy
}

type ScanResult struct {
	Dir             string   `json:"dir"`
	Found           int      `json:"found"`
	New             int      `json:"new"`
	Changed         int      `json:"changed"`
	Skipped         int      `json:"skipped"`
	SkippedByPolicy int      `json:"skipped_by_policy"`
	Enqueued        int      `json:"enqueued"`
	Errors          []string `json:"errors"`
}

// Scan walks the given directories (or all configured dirs if paths is empty).
func (s *Scanner) Scan(ctx context.Context, paths []string) ([]ScanResult, error) {
	dirs := s.dirs
	if len(paths) > 0 {
		dirs = paths
	}

	var results []ScanResult
	for _, dir := range dirs {
		var (
			r   ScanResult
			err error
		)
		if s.legacy {
			r, err = s.scanDirLegacy(ctx, dir)
		} else {
			r, err = s.scanDirBatch(ctx, dir)
		}
		if err != nil {
			return results, fmt.Errorf("scan dir %q: %w", dir, err)
		}
		results = append(results, r)
		s.log.Info("scan complete",
			"dir", dir, "found", r.Found, "new", r.New,
			"changed", r.Changed, "enqueued", r.Enqueued,
			"unchanged", r.Skipped, "skipped_by_policy", r.SkippedByPolicy,
			"errors", len(r.Errors))
	}
	return results, nil
}

func (s *Scanner) scanDirLegacy(ctx context.Context, dir string) (ScanResult, error) {
	result := ScanResult{Dir: dir}
	entries, err := s.client.ListDir(ctx, dir)
	if err != nil {
		return result, err
	}

	for _, e := range entries {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if !strings.EqualFold(filepath.Ext(e.Name), ".dbc") {
			continue
		}
		result.Found++

		if err := s.processEntry(ctx, dir, e, &result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", e.Name, err))
			s.log.Warn("entry processing error", "file", e.Name, "err", err)
		}
	}
	return result, nil
}

func (s *Scanner) processEntry(ctx context.Context, dir string, e Entry, result *ScanResult) error {
	parsed, err := domain.ParseFilename(e.Name)
	if err != nil {
		result.Skipped++
		s.log.Warn("skipping ftp file with invalid filename format",
			"filename", e.Name,
			"dir", dir,
			"error", err,
		)
		return nil
	}

	var remoteChecksum *string // DATASUS FTP does not expose checksums
	modTime := e.ModTime
	size := e.Size

	var segPtr *string
	if parsed.Segment != "" {
		s := parsed.Segment
		segPtr = &s
	}

	params := repository.UpsertFTPParams{
		Filename:        strings.ToUpper(e.Name),
		Catalog:         parsed.Catalog,
		State:           parsed.State,
		Year:            parsed.Year,
		Month:           parsed.Month,
		Segment:         segPtr,
		FTPDir:          dir,
		FTPPath:         e.RemotePath,
		SizeBytes:       &size,
		RemoteChecksum:  remoteChecksum,
		RemoteTimestamp: &modTime,
		RootPath:        s.rootPath,
	}
	if s.policy != nil {
		if dirs, dirErr := s.policy.ProcessingDirectories(ctx); dirErr == nil && dirs.DownloadDir != nil && strings.TrimSpace(*dirs.DownloadDir) != "" {
			params.RootPath = *dirs.DownloadDir
		}
	}

	file, changed, err := s.fileRepo.UpsertFromFTP(ctx, params)
	if err != nil {
		return err
	}
	previousStatus := file.OverallStatus

	allowByPolicy := true
	if s.policy != nil {
		allowByPolicy, err = s.policy.PolicyAllows(ctx, file.Catalog, file.State, file.Year, file.Month)
		if err != nil {
			return fmt.Errorf("check policy: %w", err)
		}
	}

	if !allowByPolicy {
		result.SkippedByPolicy++
		observability.PolicySkipsByState.WithLabelValues(parsed.State, "scanner_legacy").Inc()
		if shouldMoveToIgnored(file.OverallStatus) {
			if err := s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusIgnored); err != nil {
				return fmt.Errorf("set ignored by policy: %w", err)
			}
			file.OverallStatus = domain.StatusIgnored
		}
	} else {
		enqueued, err := s.reconciler.ReconcileFile(ctx, file)
		if err != nil {
			return fmt.Errorf("reconcile stages: %w", err)
		}
		result.Enqueued += enqueued
	}

	if !changed {
		result.Skipped++
		return nil
	}

	if previousStatus == domain.StatusPending {
		result.New++
	} else {
		result.Changed++
	}
	return nil
}

func shouldMoveToIgnored(status domain.OverallStatus) bool {
	switch status {
	case domain.StatusPending, domain.StatusDownloaded, domain.StatusCSVReady:
		return true
	default:
		return false
	}
}
