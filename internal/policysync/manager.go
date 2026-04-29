package policysync

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"datasus/internal/domain"
	"datasus/internal/observability"
	"datasus/internal/queue"
	"datasus/internal/repository"
	"datasus/internal/storage"
)

type Manager struct {
	policyRepo   *repository.PolicyRepository
	fileRepo     *repository.FileRepository
	stageRepo    *repository.StageRepository
	logRepo      *repository.LogRepository
	queue        *queue.PostgresQueue
	log          *slog.Logger
	defaultRoot  string
	startupDelay time.Duration

	mu      sync.Mutex
	running bool
	cron    *cron.Cron
}

func NewManager(
	policyRepo *repository.PolicyRepository,
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	logRepo *repository.LogRepository,
	q *queue.PostgresQueue,
	log *slog.Logger,
	defaultRoot string,
) *Manager {
	return &Manager{
		policyRepo:   policyRepo,
		fileRepo:     fileRepo,
		stageRepo:    stageRepo,
		logRepo:      logRepo,
		queue:        q,
		log:          log,
		defaultRoot:  defaultRoot,
		startupDelay: 3 * time.Second,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.cron = cron.New()
	if _, err := m.cron.AddFunc("0 2 * * *", func() {
		m.Trigger("scheduled_2am", "system")
	}); err != nil {
		return err
	}
	m.cron.Start()

	go func() {
		timer := time.NewTimer(m.startupDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.Trigger("startup", "system")
		}
	}()
	go func() {
		<-ctx.Done()
		if m.cron != nil {
			stopCtx := m.cron.Stop()
			select {
			case <-stopCtx.Done():
			case <-time.After(10 * time.Second):
			}
		}
	}()
	return nil
}

func (m *Manager) Trigger(reason, actor string) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()
	go m.run(reason, actor)
}

func (m *Manager) run(reason, actor string) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()
	started := time.Now()
	ctx := context.Background()

	policy, err := m.policyRepo.GetPolicies(ctx)
	if err != nil {
		m.log.Error("policy sync read policy failed", "err", err)
		return
	}
	downloadRoot := storage.ResolveDirectory(policy.Directories.DownloadDir, m.defaultRoot)
	csvRoot := storage.ResolveDirectory(policy.Directories.CSVDir, downloadRoot)
	parquetRoot := storage.ResolveDirectory(policy.Directories.ParquetDir, downloadRoot)

	if err := storage.ValidateDirectoryAccess(downloadRoot, false); err != nil {
		m.log.Warn("policy sync skipped, download dir unavailable", "path", downloadRoot, "err", err)
		return
	}

	type counters struct {
		found     int
		mapped    int
		ignored   int
		enqueued  int
		completed int
	}
	c := counters{}

	walkErr := filepath.WalkDir(downloadRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".dbc") {
			return nil
		}
		c.found++
		parsed, err := domain.ParseFilename(d.Name())
		if err != nil {
			c.ignored++
			return nil
		}
		info, err := d.Info()
		if err != nil {
			c.ignored++
			return nil
		}
		mod := info.ModTime()
		size := info.Size()
		var segPtr *string
		if parsed.Segment != "" {
			s := parsed.Segment
			segPtr = &s
		}
		file, _, err := m.fileRepo.UpsertFromFTP(ctx, repository.UpsertFTPParams{
			Filename:        strings.ToUpper(d.Name()),
			Catalog:         parsed.Catalog,
			State:           parsed.State,
			Year:            parsed.Year,
			Month:           parsed.Month,
			Segment:         segPtr,
			FTPDir:          "local_policy",
			FTPPath:         path,
			SizeBytes:       &size,
			RemoteTimestamp: &mod,
			RootPath:        downloadRoot,
		})
		if err != nil {
			c.ignored++
			return nil
		}
		c.mapped++
		_ = m.stageRepo.InitStages(ctx, file.ID)

		dbcPath := path
		csvPathValue := storage.CSVPath(csvRoot, file.Catalog, file.State, file.Year, file.Month, file.Filename)
		parquetPathValue := storage.ParquetPath(parquetRoot, file.Catalog, file.State, file.Year, file.Month, file.Filename)
		csvExists := fileExists(csvPathValue)
		parquetExists := fileExists(parquetPathValue)
		var csvPath *string
		var parquetPath *string
		if csvExists {
			csvPath = &csvPathValue
		}
		if parquetExists {
			parquetPath = &parquetPathValue
		}
		_ = m.fileRepo.UpdatePaths(ctx, file.ID, &dbcPath, csvPath, parquetPath)
		_ = m.stageRepo.SetDone(ctx, file.ID, domain.StageDownload)

		status := domain.StatusDownloaded
		if csvExists {
			status = domain.StatusCSVReady
			_ = m.stageRepo.SetDone(ctx, file.ID, domain.StageCSVConversion)
		}
		if parquetExists {
			status = domain.StatusParquetReady
			_ = m.stageRepo.SetDone(ctx, file.ID, domain.StageParquetConversion)
		}
		_ = m.fileRepo.UpdateStatus(ctx, file.ID, status)
		c.completed++

		if policy.Processing.EnableCSV && !csvExists {
			if err := m.queue.Enqueue(ctx, file.ID, domain.StageCSVConversion, time.Now()); err == nil {
				c.enqueued++
			}
		}
		if policy.Processing.EnableParquet && !parquetExists {
			if err := m.queue.Enqueue(ctx, file.ID, domain.StageParquetConversion, time.Now()); err == nil {
				c.enqueued++
			}
		}
		return nil
	})
	if walkErr != nil {
		m.log.Error("policy sync walk failed", "err", walkErr)
		return
	}
	_ = m.logRepo.InsertManualAction(context.Background(), "policy_local_sync", nil, actor, map[string]any{
		"reason":        reason,
		"download_root": downloadRoot,
		"found":         c.found,
		"mapped":        c.mapped,
		"ignored":       c.ignored,
		"completed":     c.completed,
		"enqueued":      c.enqueued,
		"duration_ms":   time.Since(started).Milliseconds(),
	})
	observability.PolicySyncRunsTotal.Inc()
	observability.PolicySyncFilesFound.Add(float64(c.found))
	observability.PolicySyncFilesMapped.Add(float64(c.mapped))
	observability.PolicySyncEnqueued.Add(float64(c.enqueued))
	m.log.Info("policy local sync finished",
		"reason", reason,
		"found", c.found,
		"mapped", c.mapped,
		"ignored", c.ignored,
		"completed", c.completed,
		"enqueued", c.enqueued,
		"duration_ms", time.Since(started).Milliseconds(),
	)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
