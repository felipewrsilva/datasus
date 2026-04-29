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

### Metabase (first run)

Compose provisions a dedicated Postgres database `metabase` for Metabase’s own metadata (pipeline data stays in `datasus`). The app DB connection uses `sslmode=disable` so the container can reach Postgres without TLS.

After `docker compose up --build -d`, run the setup helper once (creates an admin user and registers the **DATASUS Pipeline** database pointing at host `db`):

```bash
make metabase-setup
# or: python scripts/metabase_setup.py
```

Defaults (override with env vars if needed):

- URL: `http://localhost:3001` (`METABASE_URL`)
- Admin email: `admin@datasus.local` (`METABASE_ADMIN_EMAIL`)
- Admin password: `MetabaseLocal#2026` (`METABASE_ADMIN_PASSWORD`; Metabase rejects overly simple passwords)

If Metabase was already configured before this change, you may need to run the script again or finish the in-browser wizard; old Metabase tables left inside `datasus` can be ignored or removed manually.

To add the pipeline DB by hand instead: PostgreSQL, host **`db`**, port **5432**, database **`datasus`**, user **`datasus`**, password **`datasus`**, SSL off.

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

## Date display standard (UI)

All dates shown to users in the web UI must use Brazilian format with locale `pt-BR`.

- Date only: `dd/MM/yyyy`
- Date and time default: `dd/MM/yyyy HH:mm`
- Detailed logs and audits: `dd/MM/yyyy HH:mm:ss`

Rules:

- API contracts keep technical timestamp values as ISO/RFC3339.
- UI formatting must always use shared helpers from `web/src/lib/dateFormat.ts`.
- Never render ISO timestamps directly to users.
- Timezone for display follows the local timezone of each user browser.
