package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"datasus/internal/domain"
)

type StageRepository struct {
	db *pgxpool.Pool
}

type FailureReasonCount struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type StageBottleneck struct {
	Stage        string `json:"stage"`
	PendingCount int64  `json:"pending_count"`
	RunningCount int64  `json:"running_count"`
	FailedCount  int64  `json:"failed_count"`
}

type PipelineConsistency struct {
	PipelineCompletedCount   int64 `json:"pipeline_completed_count"`
	StatusStageMismatchCount int64 `json:"status_stage_mismatch_count"`
}

func NewStageRepository(db *pgxpool.Pool) *StageRepository {
	return &StageRepository{db: db}
}

func (r *StageRepository) GetByFileAndStage(ctx context.Context, fileID string, stage domain.StageName) (*domain.Stage, error) {
	const q = `
		SELECT id, file_id, stage, status, attempts,
		       started_at, finished_at, error_message, updated_at
		FROM file_stages WHERE file_id=$1 AND stage=$2`

	row := r.db.QueryRow(ctx, q, fileID, stage)
	s, err := scanStage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return s, err
}

func (r *StageRepository) ListByFile(ctx context.Context, fileID string) ([]*domain.Stage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, file_id, stage, status, attempts,
		       started_at, finished_at, error_message, updated_at
		FROM file_stages WHERE file_id=$1 ORDER BY stage`, fileID)
	if err != nil {
		return nil, fmt.Errorf("list stages: %w", err)
	}
	defer rows.Close()

	var stages []*domain.Stage
	for rows.Next() {
		s, err := scanStage(rows)
		if err != nil {
			return nil, err
		}
		stages = append(stages, s)
	}
	return stages, rows.Err()
}

// InitStages creates pending file_stage rows for all three stages if they don't exist.
func (r *StageRepository) InitStages(ctx context.Context, fileID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO file_stages (file_id, stage, status)
		VALUES ($1, 'download', 'pending'),
		       ($1, 'csv_conversion', 'pending'),
		       ($1, 'parquet_conversion', 'pending')
		ON CONFLICT (file_id, stage) DO NOTHING`, fileID)
	return err
}

func (r *StageRepository) SetRunning(ctx context.Context, fileID string, stage domain.StageName) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_stages
		SET status='running', started_at=now(), error_message=NULL, updated_at=now()
		WHERE file_id=$1 AND stage=$2`, fileID, stage)
	return err
}

func (r *StageRepository) SetDone(ctx context.Context, fileID string, stage domain.StageName) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_stages
		SET status='done', finished_at=now(), error_message=NULL, updated_at=now()
		WHERE file_id=$1 AND stage=$2`, fileID, stage)
	return err
}

func (r *StageRepository) SetFailed(ctx context.Context, fileID string, stage domain.StageName, errMsg string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_stages
		SET status='failed', finished_at=now(), error_message=$3, updated_at=now()
		WHERE file_id=$1 AND stage=$2`, fileID, stage, errMsg)
	return err
}

func (r *StageRepository) IncrementAttempts(ctx context.Context, fileID string, stage domain.StageName) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_stages SET attempts=attempts+1, updated_at=now()
		WHERE file_id=$1 AND stage=$2`, fileID, stage)
	return err
}

func (r *StageRepository) SetPurged(ctx context.Context, fileID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_stages SET status='purged', updated_at=now()
		WHERE file_id=$1`, fileID)
	return err
}

// ResetForRetry resets a stage back to pending so it can be retried.
func (r *StageRepository) ResetForRetry(ctx context.Context, fileID string, stage domain.StageName) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_stages
		SET status='pending', started_at=NULL, finished_at=NULL,
		    error_message=NULL, updated_at=now()
		WHERE file_id=$1 AND stage=$2`, fileID, stage)
	return err
}

func (r *StageRepository) TopFailureReasons(ctx context.Context, limit int) ([]FailureReasonCount, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := r.db.Query(ctx, `
		SELECT stage::text, COALESCE(NULLIF(TRIM(error_message), ''), 'Unknown error') AS reason, COUNT(*) AS total
		FROM file_stages
		WHERE status='failed'
		GROUP BY stage, reason
		ORDER BY total DESC, stage ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("top failure reasons: %w", err)
	}
	defer rows.Close()

	out := make([]FailureReasonCount, 0, limit)
	for rows.Next() {
		var item FailureReasonCount
		if err := rows.Scan(&item.Stage, &item.Reason, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *StageRepository) Bottlenecks(ctx context.Context) ([]StageBottleneck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT stage::text,
		       COUNT(*) FILTER (WHERE status='pending') AS pending_count,
		       COUNT(*) FILTER (WHERE status='running') AS running_count,
		       COUNT(*) FILTER (WHERE status='failed') AS failed_count
		FROM file_stages
		GROUP BY stage
		ORDER BY pending_count DESC, running_count DESC, stage ASC`)
	if err != nil {
		return nil, fmt.Errorf("stage bottlenecks: %w", err)
	}
	defer rows.Close()
	out := make([]StageBottleneck, 0, 8)
	for rows.Next() {
		var item StageBottleneck
		if err := rows.Scan(&item.Stage, &item.PendingCount, &item.RunningCount, &item.FailedCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *StageRepository) PipelineConsistency(
	ctx context.Context,
	expectedTerminalStatus domain.OverallStatus,
	requireDownload bool,
	requireCSV bool,
	requireParquet bool,
) (PipelineConsistency, error) {
	q := `
		WITH ` + pipelineStageFlagsAndEvalCTEs(2, 3, 4) + `
		SELECT
			COUNT(*) FILTER (WHERE pipeline_completed) AS pipeline_completed_count,
			COUNT(*) FILTER (
				WHERE (pipeline_completed AND overall_status <> $1)
				   OR (NOT pipeline_completed AND overall_status = $1)
			) AS status_stage_mismatch_count
		FROM eval`
	var out PipelineConsistency
	if err := r.db.QueryRow(
		ctx,
		q,
		string(expectedTerminalStatus),
		requireDownload,
		requireCSV,
		requireParquet,
	).Scan(&out.PipelineCompletedCount, &out.StatusStageMismatchCount); err != nil {
		return PipelineConsistency{}, fmt.Errorf("pipeline consistency: %w", err)
	}
	return out, nil
}

func (r *StageRepository) FailureCountBetween(ctx context.Context, from, to time.Time) (int64, error) {
	var count int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM file_stages
		WHERE status='failed' AND updated_at >= $1 AND updated_at < $2`, from, to).Scan(&count); err != nil {
		return 0, fmt.Errorf("failure count between: %w", err)
	}
	return count, nil
}

func (r *StageRepository) PendingOldestAgeSeconds(ctx context.Context) (int64, error) {
	var seconds int64
	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MIN(updated_at))), 0)::bigint
		FROM file_stages
		WHERE status='pending'`).Scan(&seconds); err != nil {
		return 0, fmt.Errorf("pending oldest age: %w", err)
	}
	return seconds, nil
}

func scanStage(row scannable) (*domain.Stage, error) {
	s := &domain.Stage{}
	var errMsg *string
	err := row.Scan(
		&s.ID, &s.FileID, &s.Stage, &s.Status, &s.Attempts,
		&s.StartedAt, &s.FinishedAt, &errMsg, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.ErrorMessage = errMsg
	return s, nil
}
