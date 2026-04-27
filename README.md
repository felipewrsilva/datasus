# DATASUS Pipeline

ETL pipeline for Brazilian DATASUS datasets, focused on scanning FTP folders, downloading `.dbc` files, converting them to CSV and Parquet, and exposing operational controls through an API and web UI.

## What this project includes

- Go API (`cmd/api`) for health, metrics, file queries, and pipeline triggers.
- Go worker (`cmd/worker`) with async processing queues.
- PostgreSQL for metadata, stages, logs, and job orchestration.
- Next.js web app (`web`) to inspect files and statuses.
- Docker Compose stack for local full environment.

## Architecture (high level)

1. API process scans configured DATASUS FTP paths on startup and daily schedule.
2. New files are registered in Postgres and jobs are enqueued.
3. Worker pools process stages:
   - download `.dbc`
   - convert to CSV
   - convert to Parquet
4. API and web app expose status, logs, and manual actions.

## Requirements

- Docker + Docker Compose

## Quick start (Docker, recommended)

1. Create local environment file:

```bash
cp .env.example .env
```

2. Start everything:

```bash
docker compose up --build -d
```

3. Open services:

- API: <http://localhost:8080/api/health>
- Web: <http://localhost:3002>
- Metabase: <http://localhost:3001>

## Useful commands

From repository root:

- `make build` - Build all Go packages.
- `make test` - Run unit tests.
- `make test-integration` - Run integration tests.
- `make up` - Start Docker stack.
- `make down` - Stop Docker stack.
- `make logs` - Follow compose logs.

## Main API endpoints

Base path: `/api`

- `GET /health`
- `GET /metrics`
- `GET /files`
- `GET /files/{id}`
- `GET /files/{id}/stages`
- `GET /stats`
- `POST /scan` (manual trigger, async)
- `GET /scan/status`
- `POST /download`
- `POST /download/mask`
- `POST /convert/csv`
- `POST /convert/csv/mask`
- `POST /convert/parquet`
- `POST /convert/parquet/mask`
- `POST /purge`

## Configuration

Environment variables are documented in `.env.example`.
Important ones:

- `FTP_HOST`, `FTP_PATHS`, `FTP_CONN_POOL`
- `DATABASE_URL`
- `STORAGE_ROOT`
- `CRON_SCHEDULE`, `FTP_SCAN_TIMEOUT`
- `DOWNLOAD_WORKERS`, `CSV_WORKERS`, `PARQUET_WORKERS`
- `RETRY_BASE_DELAY`, `RETRY_MAX_DELAY`, `STUCK_JOB_TIMEOUT`
- `LOG_LEVEL`, `API_PORT`

## Notes

- The Go module currently uses a local replace directive:
  - `github.com/felipewrsilva/datasusdbc => ../datasusdbc`
- Ensure that sibling repository exists locally when building/running.
