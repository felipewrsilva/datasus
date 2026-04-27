package csv

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"datasus/internal/domain"
	"datasus/internal/queue"
)

const (
	minPoll = 1 * time.Second
	maxPoll = 30 * time.Second
)

// WorkerPool runs N concurrent CSV conversion workers.
type WorkerPool struct {
	service *Service
	queue   *queue.PostgresQueue
	size    int
	log     *slog.Logger
}

func NewWorkerPool(service *Service, q *queue.PostgresQueue, size int, log *slog.Logger) *WorkerPool {
	return &WorkerPool{service: service, queue: q, size: size, log: log}
}

func (p *WorkerPool) Run(ctx context.Context) {
	for i := range p.size {
		go p.runWorker(ctx, i)
	}
	<-ctx.Done()
}

func (p *WorkerPool) runWorker(ctx context.Context, id int) {
	log := p.log.With("worker", "csv", "id", id)
	poll := minPoll

	for {
		if ctx.Err() != nil {
			return
		}

		job, err := p.queue.Claim(ctx, domain.StageCSVConversion)
		if err != nil {
			log.Warn("claim error", "err", err)
			sleep(ctx, poll)
			continue
		}
		if job == nil {
			poll = minDuration(poll*2, maxPoll)
			sleep(ctx, poll)
			continue
		}
		poll = minPoll

		if err := p.service.Process(ctx, job); err != nil {
			if errors.Is(err, domain.ErrPolicyBlocked) {
				finalizeCtx, cancel := finalizeContext(ctx)
				if ab := p.queue.AbandonRunningJob(finalizeCtx, job.ID); ab != nil {
					log.Error("abandon job after policy block", "err", ab)
				}
				cancel()
				log.Info("csv conversion skipped by policy", "file_id", job.FileID)
				continue
			}
			log.Warn("csv conversion failed", "file_id", job.FileID, "err", err)
			finalizeCtx, cancel := finalizeContext(ctx)
			if err := p.queue.Nack(finalizeCtx, job, err); err != nil {
				log.Error("nack error", "err", err)
			}
			cancel()
		} else {
			finalizeCtx, cancel := finalizeContext(ctx)
			if err := p.queue.Ack(finalizeCtx, job.ID, job.FileID, domain.StageCSVConversion); err != nil {
				log.Error("ack error", "err", err)
			}
			cancel()
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func finalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	// During shutdown, use a short-lived context so queue state can be persisted.
	return context.WithTimeout(context.Background(), 5*time.Second)
}
