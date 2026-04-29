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
	batchSize    int

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
		batchSize:    1000,
	}
}

// SetBatchSize lets the caller tune chunking of bulk DB operations during
// local sync. Values <= 0 are ignored.
func (m *Manager) SetBatchSize(n int) {
	if n > 0 {
		m.batchSize = n
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

	type localItem struct {
		path             string
		filename         string
		parsed           domain.ParsedFilename
		size             int64
		mod              time.Time
		csvPathValue     string
		parquetPathValue string
		csvExists        bool
		parquetExists    bool
	}

	items := make([]localItem, 0, 1024)
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
		filename := strings.ToUpper(d.Name())
		csvPathValue := storage.CSVPath(csvRoot, parsed.Catalog, parsed.State, parsed.Year, parsed.Month, filename)
		parquetPathValue := storage.ParquetPath(parquetRoot, parsed.Catalog, parsed.State, parsed.Year, parsed.Month, filename)
		items = append(items, localItem{
			path:             path,
			filename:         filename,
			parsed:           parsed,
			size:             info.Size(),
			mod:              info.ModTime(),
			csvPathValue:     csvPathValue,
			parquetPathValue: parquetPathValue,
			csvExists:        fileExists(csvPathValue),
			parquetExists:    fileExists(parquetPathValue),
		})
		return nil
	})
	if walkErr != nil {
		m.log.Error("policy sync walk failed", "err", walkErr)
		return
	}

	batchSize := m.batchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	upserts := make([]repository.UpsertFTPParams, len(items))
	for i, it := range items {
		var segPtr *string
		if it.parsed.Segment != "" {
			s := it.parsed.Segment
			segPtr = &s
		}
		size := it.size
		mod := it.mod
		upserts[i] = repository.UpsertFTPParams{
			Filename:        it.filename,
			Catalog:         it.parsed.Catalog,
			State:           it.parsed.State,
			Year:            it.parsed.Year,
			Month:           it.parsed.Month,
			Segment:         segPtr,
			FTPDir:          "local_policy",
			FTPPath:         it.path,
			SizeBytes:       &size,
			RemoteTimestamp: &mod,
			RootPath:        downloadRoot,
		}
	}

	nameToID := make(map[string]string, len(items))
	for start := 0; start < len(upserts); start += batchSize {
		end := start + batchSize
		if end > len(upserts) {
			end = len(upserts)
		}
		chunk := upserts[start:end]
		res, err := m.fileRepo.BulkUpsertFromFTP(ctx, chunk)
		if err != nil {
			m.log.Warn("policy sync bulk upsert failed, falling back per-row", "err", err, "size", len(chunk))
			for _, p := range chunk {
				f, _, perr := m.fileRepo.UpsertFromFTP(ctx, p)
				if perr != nil {
					c.ignored++
					continue
				}
				nameToID[p.Filename] = f.ID
			}
			continue
		}
		for i, id := range res.IDs {
			nameToID[chunk[i].Filename] = id
		}
	}

	mappedIDs := make([]string, 0, len(items))
	for _, it := range items {
		if _, ok := nameToID[it.filename]; ok {
			mappedIDs = append(mappedIDs, nameToID[it.filename])
		}
	}
	c.mapped = len(mappedIDs)
	c.ignored += len(items) - c.mapped

	for start := 0; start < len(mappedIDs); start += batchSize {
		end := start + batchSize
		if end > len(mappedIDs) {
			end = len(mappedIDs)
		}
		if err := m.stageRepo.BulkInitStages(ctx, mappedIDs[start:end]); err != nil {
			m.log.Warn("policy sync bulk init stages failed", "err", err)
		}
	}

	enqItems := make([]queue.EnqueueItem, 0, len(items))
	now := time.Now()
	for _, it := range items {
		id, ok := nameToID[it.filename]
		if !ok {
			continue
		}

		dbcPath := it.path
		var csvPath, parquetPath *string
		if it.csvExists {
			v := it.csvPathValue
			csvPath = &v
		}
		if it.parquetExists {
			v := it.parquetPathValue
			parquetPath = &v
		}
		_ = m.fileRepo.UpdatePaths(ctx, id, &dbcPath, csvPath, parquetPath)
		_ = m.stageRepo.SetDone(ctx, id, domain.StageDownload)

		status := domain.StatusDownloaded
		if it.csvExists {
			status = domain.StatusCSVReady
			_ = m.stageRepo.SetDone(ctx, id, domain.StageCSVConversion)
		}
		if it.parquetExists {
			status = domain.StatusParquetReady
			_ = m.stageRepo.SetDone(ctx, id, domain.StageParquetConversion)
		}
		_ = m.fileRepo.UpdateStatus(ctx, id, status)
		c.completed++

		if policy.Processing.EnableCSV && !it.csvExists {
			enqItems = append(enqItems, queue.EnqueueItem{FileID: id, Stage: domain.StageCSVConversion, AvailableAt: now})
		}
		if policy.Processing.EnableParquet && !it.parquetExists {
			enqItems = append(enqItems, queue.EnqueueItem{FileID: id, Stage: domain.StageParquetConversion, AvailableAt: now})
		}
	}

	for start := 0; start < len(enqItems); start += batchSize {
		end := start + batchSize
		if end > len(enqItems) {
			end = len(enqItems)
		}
		if err := m.queue.BulkEnqueue(ctx, enqItems[start:end]); err != nil {
			m.log.Warn("policy sync bulk enqueue failed", "err", err)
			continue
		}
		c.enqueued += end - start
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
