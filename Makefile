GO := go
GOFLAGS := -v

.PHONY: all build test test-integration lint clean up down

all: build

build:
	$(GO) build $(GOFLAGS) ./...

# Unit tests (no Docker required)
test:
	$(GO) test -short ./...

# Integration tests (requires Docker)
test-integration:
	$(GO) test -tags integration ./...

lint:
	golangci-lint run ./...

# Run database migrations manually (useful for local dev outside Docker)
migrate:
	psql "$(DATABASE_URL)" -f migrations/001_initial.sql -f migrations/002_indexes.sql -f migrations/003_policy.sql -f migrations/004_policy_simplification.sql -f migrations/005_global_download_policy.sql -f migrations/006_processing_policy.sql -f migrations/007_dynamic_policy_period_constraints.sql -f migrations/008_status_pending_ignored.sql -f migrations/009_files_display_timestamp_sort.sql -f migrations/010_processing_policy_directories.sql -f migrations/011_files_segment.sql

# Docker Compose helpers
up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

# First-time Metabase wizard (admin user + DATASUS Pipeline DB). Requires Python 3.
metabase-setup:
	python scripts/metabase_setup.py

# Scale workers
scale-workers:
	docker compose up --scale worker=3 -d

# Run api locally (requires .env)
run-api:
	$(GO) run ./cmd/api

# Run worker locally (requires .env)
run-worker:
	$(GO) run ./cmd/worker
