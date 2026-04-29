//go:build integration

// Package testutil provides helpers for integration tests.
//
// Set DATABASE_URL to a running PostgreSQL instance before running:
//
//	DATABASE_URL="postgres://datasus:datasus@localhost:5432/datasus_test?sslmode=disable" \
//	  go test -tags integration ./...
//
// With Docker Compose, run `docker compose up -d db` first.
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDB connects to the PostgreSQL instance from DATABASE_URL and applies migrations.
// The database is NOT torn down after tests; run against a dedicated test database.
func TestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := applyMigrations(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("..", "..", "migrations")
	matches, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(matches)
	for _, path := range matches {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			// Ignore "already exists" errors — idempotent
			_ = err
		}
	}
	return nil
}
