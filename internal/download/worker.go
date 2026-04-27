package download

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"datasus/internal/domain"
	"datasus/internal/queue"
)

const (
	minPollInterval = 1 * time.Second
	maxPollInterval = 30 * time.Second
)

// WorkerPool runs N concurrent download workers that poll the job queue.
type WorkerPool struct {
	service  *Service
	queue    *queue.PostgresQueue
	size     int
	log      *slog.Logger
}

func NewWorkerPool(service *Service, q *queue.PostgresQueue, size int, log *slog.Logger) *WorkerPool {
	return &WorkerPool{service: service, queue: q, size: size, log: log}
}

// Run starts all workers and blocks until ctx is cancelled.
func (p *WorkerPool) Run(ctx context.Context) {
	for i := range p.size {
		go p.runWorker(ctx, i)
	}
	<-ctx.Done()
}

func (p *WorkerPool) runWorker(ctx context.Context, id int) {
	log := p.log.With("worker", "download", "id", id)
	poll := minPollInterval

	for {
		if ctx.Err() != nil {
			return
		}

		job, err := p.queue.Claim(ctx, domain.StageDownload)
		if err != nil {
			log.Warn("claim error", "err", err)
			select {
			case <-time.After(poll):
			case <-ctx.Done():
				return
			}
			continue
		}

		if job == nil {
			// No work — back off
			poll = min(poll*2, maxPollInterval)
			select {
			case <-time.After(poll):
			case <-ctx.Done():
				return
			}
			continue
		}

		// Reset poll interval — work found
		poll = minPollInterval

		if err := p.service.Process(ctx, job); err != nil {
			if errors.Is(err, domain.ErrPolicyBlocked) {
				finalizeCtx, cancel := finalizeContext(ctx)
				if ab := p.queue.AbandonRunningJob(finalizeCtx, job.ID); ab != nil {
					log.Error("abandon job after policy block", "err", ab)
				}
				cancel()
				log.Info("download skipped by policy", "file_id", job.FileID)
				continue
			}
			log.Warn("download failed", "file_id", job.FileID, "err", err)
			finalizeCtx, cancel := finalizeContext(ctx)
			if err := p.queue.Nack(finalizeCtx, job, err); err != nil {
				log.Error("nack error", "err", err)
			}
			cancel()
		} else {
			finalizeCtx, cancel := finalizeContext(ctx)
			if err := p.queue.Ack(finalizeCtx, job.ID, job.FileID, domain.StageDownload); err != nil {
				log.Error("ack error", "err", err)
			}
			cancel()
		}
	}
}

func finalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	// During shutdown, use a short-lived context so queue state can be persisted.
	return context.WithTimeout(context.Background(), 5*time.Second)
}

