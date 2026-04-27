//go:build integration

package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InsertFile inserts a minimal file row and returns its ID.
func InsertFile(t *testing.T, db *pgxpool.Pool, filename string) string {
	t.Helper()
	ctx := context.Background()

	var id string
	err := db.QueryRow(ctx, `
		INSERT INTO files (filename, catalog, state, year, month, ftp_dir, ftp_path, root_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		filename, filename[:2], filename[2:4],
		2026, 1,
		"/dissemin/publicos/SIHSUS/200801_/Dados",
		"/dissemin/publicos/SIHSUS/200801_/Dados/"+filename,
		"/data",
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert file %q: %v", filename, err)
	}
	return id
}

// InsertStages inserts all three stage rows in pending state.
func InsertStages(t *testing.T, db *pgxpool.Pool, fileID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO file_stages (file_id, stage, status)
		VALUES ($1, 'download', 'pending'),
		       ($1, 'csv_conversion', 'pending'),
		       ($1, 'parquet_conversion', 'pending')
		ON CONFLICT DO NOTHING`, fileID)
	if err != nil {
		t.Fatalf("insert stages for %q: %v", fileID, err)
	}
}

// EnqueueJob inserts a job_queue row and returns its ID.
func EnqueueJob(t *testing.T, db *pgxpool.Pool, fileID, stage string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := db.QueryRow(ctx, `
		INSERT INTO job_queue (file_id, stage, status, available_at)
		VALUES ($1, $2, 'pending', $3)
		RETURNING id`, fileID, stage, time.Now()).Scan(&id)
	if err != nil {
		t.Fatalf("enqueue job %s/%s: %v", fileID, stage, err)
	}
	return id
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("readFile %q: %w", path, err)
	}
	return string(b), nil
}
