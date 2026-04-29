package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"datasus/internal/domain"
)

// PostgresQueue implements a durable job queue backed by PostgreSQL SKIP LOCKED.
type PostgresQueue struct {
	db              *pgxpool.Pool
	workerID        string
	retryBaseDelay  time.Duration
	retryMaxDelay   time.Duration
	stuckJobTimeout time.Duration
	log             *slog.Logger
}

type DepthEntry struct {
	Stage  domain.StageName
	Status domain.StageStatus
	Count  float64
}

func New(db *pgxpool.Pool, workerID string, baseDelay, maxDelay, stuckTimeout time.Duration, log *slog.Logger) *PostgresQueue {
	return &PostgresQueue{
		db:              db,
		workerID:        workerID,
		retryBaseDelay:  baseDelay,
		retryMaxDelay:   maxDelay,
		stuckJobTimeout: stuckTimeout,
		log:             log,
	}
}

// AbandonRunningJob deletes a claimed (running) job without updating file_stages.
// Use when the work item must be dropped, e.g. the file no longer matches download policy.
// Succeeds even if no row matched (idempotent).
func (q *PostgresQueue) AbandonRunningJob(ctx context.Context, jobID string) error {
	_, err := q.db.Exec(ctx, `DELETE FROM job_queue WHERE id = $1 AND status = 'running'`, jobID)
	if err != nil {
		return fmt.Errorf("abandon job: %w", err)
	}
	return nil
}

// Claim atomically takes one pending job for the given stage.
// Returns (nil, nil) when no jobs are available.
func (q *PostgresQueue) Claim(ctx context.Context, stage domain.StageName) (*Job, error) {
	const sql = `
		UPDATE job_queue
		SET    status     = 'running',
		       locked_at  = now(),
		       locked_by  = $2,
		       attempts   = attempts + 1,
		       updated_at = now()
		WHERE  id = (
		    SELECT id FROM job_queue
		    WHERE  stage        = $1
		      AND  status       = 'pending'
		      AND  available_at <= now()
		    ORDER  BY available_at ASC
		    FOR UPDATE SKIP LOCKED
		    LIMIT  1
		)
		RETURNING id, file_id, stage, status, available_at, locked_at, locked_by,
		          attempts, payload_json, created_at, updated_at`

	row := q.db.QueryRow(ctx, sql, stage, q.workerID)
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

// Ack marks a job as done and updates the corresponding file_stage to done.
// All updates happen in one transaction.
func (q *PostgresQueue) Ack(ctx context.Context, jobID, fileID string, stage domain.StageName) error {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		UPDATE job_queue SET status='done', updated_at=now() WHERE id=$1`, jobID)
	if err != nil {
		return fmt.Errorf("ack job_queue: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE file_stages
		SET status='done', finished_at=now(), error_message=NULL, updated_at=now()
		WHERE file_id=$1 AND stage=$2`, fileID, stage)
	if err != nil {
		return fmt.Errorf("ack file_stages: %w", err)
	}

	return tx.Commit(ctx)
}

// Nack marks a job as failed or requeues it with exponential backoff.
// If max attempts is reached, the stage and job are set to failed permanently.
func (q *PostgresQueue) Nack(ctx context.Context, job *Job, reason error) error {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	errMsg := reason.Error()
	maxed := job.Attempts >= domain.MaxStageAttempts

	if maxed {
		_, err = tx.Exec(ctx, `
			UPDATE job_queue SET status='failed', updated_at=now() WHERE id=$1`, job.ID)
		if err != nil {
			return fmt.Errorf("nack job_queue failed: %w", err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE file_stages
			SET status='failed', finished_at=now(), error_message=$3, updated_at=now()
			WHERE file_id=$1 AND stage=$2`, job.FileID, job.Stage, errMsg)
	} else {
		backoff := q.backoffDuration(job.Attempts)
		_, err = tx.Exec(ctx, `
			UPDATE job_queue
			SET status='pending', locked_at=NULL, locked_by=NULL,
			    available_at=$2, updated_at=now()
			WHERE id=$1`, job.ID, time.Now().Add(backoff))
		if err != nil {
			return fmt.Errorf("nack job_queue requeue: %w", err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE file_stages
			SET status='failed', error_message=$3, updated_at=now()
			WHERE file_id=$1 AND stage=$2`, job.FileID, job.Stage, errMsg)
	}
	if err != nil {
		return fmt.Errorf("nack file_stages: %w", err)
	}

	return tx.Commit(ctx)
}

// Enqueue inserts a job into the queue. The UNIQUE constraint on (file_id, stage)
// ensures idempotency — duplicate enqueues are silently ignored.
func (q *PostgresQueue) Enqueue(ctx context.Context, fileID string, stage domain.StageName, availableAt time.Time) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO job_queue (file_id, stage, status, available_at)
		VALUES ($1, $2, 'pending', $3)
		ON CONFLICT (file_id, stage) DO UPDATE
		    SET status='pending', available_at=$3, locked_at=NULL, locked_by=NULL, attempts=0, updated_at=now()
		    WHERE job_queue.status IN ('failed', 'done', 'pending')`, fileID, stage, availableAt)
	if err != nil {
		return fmt.Errorf("enqueue %s/%s: %w", fileID, stage, err)
	}
	return nil
}

// EnqueueItem describes a single queue insertion in BulkEnqueue.
type EnqueueItem struct {
	FileID      string
	Stage       domain.StageName
	AvailableAt time.Time
}

// BulkEnqueue inserts many jobs in one round trip with the same idempotent
// semantics as Enqueue: only pending/done/failed rows get reset; running rows
// are left alone.
func (q *PostgresQueue) BulkEnqueue(ctx context.Context, items []EnqueueItem) error {
	if len(items) == 0 {
		return nil
	}
	fileIDs := make([]string, len(items))
	stages := make([]string, len(items))
	avails := make([]time.Time, len(items))
	for i, it := range items {
		fileIDs[i] = it.FileID
		stages[i] = string(it.Stage)
		avails[i] = it.AvailableAt
	}
	_, err := q.db.Exec(ctx, `
		INSERT INTO job_queue (file_id, stage, status, available_at)
		SELECT file_id, stage::stage_name, 'pending', avail
		FROM unnest($1::uuid[], $2::text[], $3::timestamptz[])
		    AS t(file_id, stage, avail)
		ON CONFLICT (file_id, stage) DO UPDATE
		    SET status='pending', available_at=EXCLUDED.available_at,
		        locked_at=NULL, locked_by=NULL, attempts=0, updated_at=now()
		    WHERE job_queue.status IN ('failed', 'done', 'pending')`,
		fileIDs, stages, avails)
	if err != nil {
		return fmt.Errorf("bulk enqueue: %w", err)
	}
	return nil
}

func (q *PostgresQueue) DepthByStageStatus(ctx context.Context) ([]DepthEntry, error) {
	rows, err := q.db.Query(ctx, `
		SELECT stage, status, COUNT(*)::float8
		FROM job_queue
		GROUP BY stage, status`)
	if err != nil {
		return nil, fmt.Errorf("queue depth: %w", err)
	}
	defer rows.Close()

	out := make([]DepthEntry, 0, 12)
	for rows.Next() {
		var e DepthEntry
		if err := rows.Scan(&e.Stage, &e.Status, &e.Count); err != nil {
			return nil, fmt.Errorf("scan queue depth: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue depth: %w", err)
	}
	return out, nil
}

// RecoverStuckJobs resets jobs that have been locked longer than stuckJobTimeout.
// Intended to be called periodically (e.g. every 5 minutes) by a background goroutine.
func (q *PostgresQueue) RecoverStuckJobs(ctx context.Context) (int64, error) {
	res, err := q.db.Exec(ctx, `
		UPDATE job_queue
		SET status='pending', locked_at=NULL, locked_by=NULL, available_at=now(), updated_at=now()
		WHERE status='running'
		  AND locked_at < now() - $1::interval`,
		q.stuckJobTimeout.String())
	if err != nil {
		return 0, fmt.Errorf("recover stuck jobs: %w", err)
	}
	return res.RowsAffected(), nil
}

// backoffDuration computes min(base * 2^attempt, max) with ±10% jitter.
func (q *PostgresQueue) backoffDuration(attempt int) time.Duration {
	exp := math.Pow(2, float64(attempt))
	d := time.Duration(float64(q.retryBaseDelay) * exp)
	if d > q.retryMaxDelay {
		d = q.retryMaxDelay
	}
	// ±10% jitter
	jitter := time.Duration(rand.Float64()*0.2*float64(d)) - time.Duration(0.1*float64(d))
	return d + jitter
}

func scanJob(row pgx.Row) (*Job, error) {
	j := &Job{}
	err := row.Scan(
		&j.ID, &j.FileID, &j.Stage, &j.Status,
		&j.AvailableAt, &j.LockedAt, &j.LockedBy,
		&j.Attempts, &j.PayloadJSON, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return j, nil
}
