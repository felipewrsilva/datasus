package ftp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"datasus/internal/domain"
	"datasus/internal/queue"
	"datasus/internal/repository"
)

// Scanner walks configured FTP directories, updates discovery metadata, and enqueues pending processing.
type Scanner struct {
	client    *Client
	dirs      []string
	fileRepo  *repository.FileRepository
	stageRepo *repository.StageRepository
	queue     *queue.PostgresQueue
	policy    *repository.PolicyRepository
	rootPath  string
	log       *slog.Logger
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
		client:    client,
		dirs:      dirs,
		fileRepo:  fileRepo,
		stageRepo: stageRepo,
		queue:     q,
		policy:    policy,
		rootPath:  rootPath,
		log:       log,
	}
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
		r, err := s.scanDir(ctx, dir)
		if err != nil {
			return results, fmt.Errorf("scan dir %q: %w", dir, err)
		}
		results = append(results, r)
		s.log.Info("scan complete",
			"dir", dir, "found", r.Found, "new", r.New,
			"changed", r.Changed, "enqueued", r.Enqueued)
	}
	return results, nil
}

func (s *Scanner) scanDir(ctx context.Context, dir string) (ScanResult, error) {
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

	params := repository.UpsertFTPParams{
		Filename:        strings.ToUpper(e.Name),
		Catalog:         parsed.Catalog,
		State:           parsed.State,
		Year:            parsed.Year,
		Month:           parsed.Month,
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
		allowByPolicy, err = s.policy.PolicyAllows(ctx, file.Catalog, file.Year, file.Month)
		if err != nil {
			return fmt.Errorf("check policy: %w", err)
		}
	}

	stage, shouldEnqueue := nextStageFor(file.OverallStatus, changed)
	if !allowByPolicy {
		result.SkippedByPolicy++
		if shouldMoveToIgnored(file.OverallStatus) {
			if err := s.fileRepo.UpdateStatus(ctx, file.ID, domain.StatusIgnored); err != nil {
				return fmt.Errorf("set ignored by policy: %w", err)
			}
			file.OverallStatus = domain.StatusIgnored
		}
	} else if shouldEnqueue {
		if err := s.stageRepo.InitStages(ctx, file.ID); err != nil {
			return fmt.Errorf("init stages: %w", err)
		}
		if err := s.queue.Enqueue(ctx, file.ID, stage, time.Now()); err != nil {
			return fmt.Errorf("enqueue %s: %w", stage, err)
		}
		result.Enqueued++
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

func nextStageFor(status domain.OverallStatus, changed bool) (domain.StageName, bool) {
	if changed {
		return domain.StageDownload, true
	}
	switch status {
	case domain.StatusPending, domain.StatusFailed:
		return domain.StageDownload, true
	case domain.StatusDownloaded:
		return domain.StageCSVConversion, true
	case domain.StatusCSVReady:
		return domain.StageParquetConversion, true
	default:
		return "", false
	}
}
