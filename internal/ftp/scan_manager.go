package ftp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"datasus/internal/observability"
	"datasus/internal/repository"
)

var ErrScanAlreadyRunning = errors.New("scan already running")

type ScanState string

const (
	ScanStateStopped ScanState = "stopped"
	ScanStateRunning ScanState = "running"
	ScanStateError   ScanState = "error"
)

type ScanSnapshot struct {
	State         ScanState     `json:"state"`
	Running       bool          `json:"running"`
	CurrentReason string        `json:"current_reason,omitempty"`
	LastReason    string        `json:"last_reason,omitempty"`
	LastActor     string        `json:"last_actor,omitempty"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
	LastSuccessAt *time.Time    `json:"last_success_at,omitempty"`
	LastError     string        `json:"last_error,omitempty"`
	LastDuration  time.Duration `json:"last_duration_ns"`
	LastFound     int           `json:"last_found"`
	LastEnqueued  int           `json:"last_enqueued"`
}

type ScanManager struct {
	scanner      *Scanner
	logRepo      *repository.LogRepository
	policyRepo   *repository.PolicyRepository
	log          *slog.Logger
	schedule     string
	scanTimeout  time.Duration
	startupDelay time.Duration

	mu       sync.Mutex
	snapshot ScanSnapshot
	cron     *cron.Cron
}

func NewScanManager(
	scanner *Scanner,
	logRepo *repository.LogRepository,
	policyRepo *repository.PolicyRepository,
	log *slog.Logger,
	schedule string,
	scanTimeout time.Duration,
) *ScanManager {
	return &ScanManager{
		scanner:      scanner,
		logRepo:      logRepo,
		policyRepo:   policyRepo,
		log:          log,
		schedule:     schedule,
		scanTimeout:  scanTimeout,
		startupDelay: 2 * time.Second,
		snapshot: ScanSnapshot{
			State:   ScanStateStopped,
			Running: false,
		},
	}
}

func (m *ScanManager) Start(ctx context.Context) error {
	m.cron = cron.New()
	if _, err := m.cron.AddFunc(m.schedule, func() {
		if _, err := m.Trigger("scheduled", "system", nil); err != nil && !errors.Is(err, ErrScanAlreadyRunning) {
			m.log.Error("scheduled scan trigger failed", "err", err)
		}
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
			if _, err := m.Trigger("startup", "system", nil); err != nil && !errors.Is(err, ErrScanAlreadyRunning) {
				m.log.Error("startup scan trigger failed", "err", err)
			}
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

func (m *ScanManager) Trigger(reason, actor string, paths []string) (ScanSnapshot, error) {
	now := time.Now()

	m.mu.Lock()
	if m.snapshot.Running {
		s := m.snapshot
		m.mu.Unlock()
		return s, ErrScanAlreadyRunning
	}
	m.snapshot.Running = true
	m.snapshot.State = ScanStateRunning
	m.snapshot.CurrentReason = reason
	m.snapshot.LastReason = reason
	m.snapshot.LastActor = actor
	m.snapshot.StartedAt = &now
	m.snapshot.FinishedAt = nil
	m.snapshot.LastError = ""
	m.mu.Unlock()

	_ = m.logRepo.InsertManualAction(context.Background(), "scan_triggered", nil, actor, map[string]any{
		"reason": reason,
		"paths":  paths,
	})

	go m.run(reason, actor, paths)

	return m.Snapshot(), nil
}

func (m *ScanManager) Snapshot() ScanSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}

func (m *ScanManager) run(reason, actor string, paths []string) {
	start := time.Now()
	ctx := context.Background()
	if m.scanTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.scanTimeout)
		defer cancel()
	}

	m.log.Info("ftp scan started", "reason", reason, "actor", actor)
	results, err := m.scanner.Scan(ctx, paths)
	if m.policyRepo != nil {
		toIgnored, toPending, reconcileErr := m.policyRepo.ReconcileIgnoredStatuses(context.Background())
		if reconcileErr != nil {
			if err == nil {
				err = fmt.Errorf("reconcile ignored statuses: %w", reconcileErr)
			} else {
				m.log.Warn("policy reconciliation failed after scan", "err", reconcileErr)
			}
		} else {
			m.log.Info("policy status reconciliation complete", "to_ignored", toIgnored, "to_pending", toPending)
		}
	}
	duration := time.Since(start)

	found := 0
	enqueued := 0
	errorsCount := 0
	for _, r := range results {
		found += r.Found
		enqueued += r.Enqueued
		errorsCount += len(r.Errors)
	}

	observability.FTPScanDuration.Observe(duration.Seconds())
	if found > 0 {
		observability.FTPScanFilesFound.Add(float64(found))
	}
	if enqueued > 0 {
		observability.FTPScanFilesEnqueued.Add(float64(enqueued))
	}

	now := time.Now()
	m.mu.Lock()
	m.snapshot.Running = false
	m.snapshot.CurrentReason = ""
	m.snapshot.FinishedAt = &now
	m.snapshot.LastDuration = duration
	m.snapshot.LastFound = found
	m.snapshot.LastEnqueued = enqueued
	if err != nil {
		m.snapshot.State = ScanStateError
		m.snapshot.LastError = err.Error()
	} else {
		m.snapshot.State = ScanStateStopped
		m.snapshot.LastError = ""
		m.snapshot.LastSuccessAt = &now
	}
	s := m.snapshot
	m.mu.Unlock()

	_ = m.logRepo.InsertManualAction(context.Background(), "scan_finished", nil, actor, map[string]any{
		"reason":       reason,
		"duration_ms":  duration.Milliseconds(),
		"found":        found,
		"enqueued":     enqueued,
		"error_count":  errorsCount,
		"status":       s.State,
		"error_detail": s.LastError,
	})

	if err != nil {
		m.log.Error("ftp scan failed",
			"reason", reason,
			"actor", actor,
			"duration_ms", duration.Milliseconds(),
			"found", found,
			"enqueued", enqueued,
			"err", err,
		)
		return
	}
	m.log.Info("ftp scan finished",
		"reason", reason,
		"actor", actor,
		"duration_ms", duration.Milliseconds(),
		"found", found,
		"enqueued", enqueued,
		"errors", errorsCount,
	)
}
