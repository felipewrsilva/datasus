# Standalone Go service: DATASUS `.dbc` FTP download + SQL Server audit

## Goal

- **FTP:** Download `.dbc` files for catalogs **AM, AQ, PA, SP, RD** (two-letter `catalog` in standard SIH-style names), for the **current calendar year and the previous three years** (four years inclusive).
- **Configuration:** Everything the program needs must come from an **`appsettings.json`** file placed **next to the executable** (resolve with `filepath.Dir` of `os.Executable()`). No reliance on Postgres for runtime config.
- **SQL Server:** Log every download and FTP metadata to **`azwladatasus01d` / `BR_DATASUS_DB`**. Tables use **NDRS naming** (`LOG_*`, column prefixes — see `.cursor/rules/ndrs-sqlserver-conventions.mdc`).
- **Re-download:** If **FTP listing metadata** for a file no longer matches what was stored for the last successful download, **download again** (see change detection).
- **Download organization:** All downloaded files must be saved in subfolders by year (e.g., `<localRoot>/2024/`). The tool must organize files so that each year in scope has its own subdirectory under the configured `download.localRoot`. For instance, files for the year 2024 are placed under `2024`, 2023 under `2023`, and so forth.

---

## Configuration actually read from this environment (reference only)

These values were **queried from the local DATASUS Postgres** (`app_runtime_config` where `id = 1`) so you can pre-fill `appsettings.json`. **Production should use the JSON file**, not Postgres.

| Field | Value (as observed) |
|--------|----------------------|
| `ftp_host` | `ftp.datasus.gov.br` |
| `ftp_paths` | `/dissemin/publicos/SIASUS/200801_/Dados` (comma-separated list in DB; here a single path) |
| `ftp_conn_pool` | `2` |
| `ftp_pool_noop` | `true` |
| `ftp_scan_batch_size` | `1000` |
| `ftp_scan_legacy` | `false` |
| `ftp_scan_max_depth` | `0` |
| `storage_root` | `C:/Users/fsilva2/AppData/Local/DatasusPipeline` |
| `ftp_scan_timeout` | `30m` |

FTP anonymous login in the existing Go client: user `anonymous`, password `anonymous@datasus` (see `internal/ftp/client.go`).

---

## `appsettings.json` (required next to the `.exe`)

At runtime the program loads **`appsettings.json` from the same directory as the executable** (not the working directory).

**Template (values match the Postgres snapshot above):**  
[`downloader/appsettings.example.json`](downloader/appsettings.example.json) — copy to `appsettings.json` next to `downloader.exe` and adjust secrets/paths. **Do not commit SQL passwords**; prefer Windows integrated auth in `connectionString` where possible.

### Field meanings

- **`ftp.paths`:** Array of FTP root directories to scan (replaces comma-separated `ftp_paths` from Postgres).
- **`ftp.scanMaxDepth`:** Same semantics as `app_runtime_config.ftp_scan_max_depth` (0 = only listed paths).
- **`ftp.scanTimeout`:** Max time for a single scan/list operation if you implement per-operation timeouts.
- **`download.localRoot`:** Directory where `.dbc` files are written. **All downloads must be organized in subfolders by year, e.g., `<localRoot>/2024/`, `<localRoot>/2023/`, etc.** Do not flatten all files in a single directory—partition by year as parsed from the filename (or derived as needed).
- **`download.catalogs`:** Uppercase two-letter systems to keep.
- **`download.yearsBackInclusive`:** `3` means years **`[now.Year()-3, now.Year]`** inclusive.
- **`download.parallelWorkers`:** Concurrent downloads (independent of pipeline’s `download_workers` in Postgres).
- **`sqlServer.connectionString`:** Standard ADO-style string for **`database/sql`** with the Microsoft driver (`github.com/microsoft/go-mssqldb`).

---

## Filename rules (align with main repo)

Use the same parsing as **`internal/domain/file.go`** — `ParseFilename`:

- Standard: `[catalog:2][state:2][yy:2][mm:2][optional segment A–Z].dbc`
- SIASUS 9-char bases use a **three-letter** catalog; the five systems **AM, AQ, PA, SP, RD** are **two-letter** catalogs on the standard layout.

When saving files, extract the year from the filename to determine the proper subfolder under `download.localRoot`. For example, a file with year 2022 should be saved in the `<localRoot>/2022/` directory.

FTP listing row shape in this codebase (`internal/ftp/client.go` `Entry`): **`Name`, `Size`, `ModTime`, `RemotePath`**. Map into SQL columns below (remote size → `QT_REMOTE_SIZE_BYTES`, mtime → `DT_REMOTE_MODIFIED_UTC`, path → `EX_FTP_REMOTE_PATH`, etc.).

---

## SQL Server schema (BR_DATASUS_DB) — NDRS-aligned

**Script:** [`downloader/sql/Create-Tables-Datasus-Dbc-Download-Registry.sql`](downloader/sql/Create-Tables-Datasus-Dbc-Download-Registry.sql) (copy also under `scripts/sqlserver/` in this repo)  
**Conventions:** `.cursor/rules/ndrs-sqlserver-conventions.mdc` (summary of NDRS `CONVENTIONS.md` + `sql-style.mdc`).  
**NDRS source repo:** `C:\Users\fsilva2\source\repos\ndrs\.cursor\docs\database\CONVENTIONS.md`

### `dbo.LOG_DATASUS_DBC_FILE` (one row per logical FTP file)

- **PK:** `ID_LOG_DATASUS_DBC_FILE` (`uniqueidentifier`, `NEWSEQUENTIALID()` default).
- **Unique:** `(DC_FTP_HOST, EX_FTP_REMOTE_PATH)`.
- **FTP / parsed metadata:** `DC_FTP_DIRECTORY`, `DC_FILE_NAME`, `CD_CATALOG`, `CD_STATE`, `NR_FILE_YEAR`, `NR_FILE_MONTH`, `CD_SEGMENT`, `QT_REMOTE_SIZE_BYTES`, `DT_REMOTE_MODIFIED_UTC`.
- **Local:** `DC_LOCAL_PATH`, `QT_LOCAL_SIZE_BYTES`, `BN_LOCAL_SHA256_HEX`.
- **Run metadata:** `DT_LAST_DOWNLOAD_UTC`, `DC_LAST_DOWNLOAD_STATUS`, `NR_DOWNLOAD_COUNT`, `DT_CREATED_UTC`, `DT_UPDATED_UTC`.

### `dbo.LOG_DATASUS_DBC_DOWNLOAD` (one row per download attempt)

- **PK:** `ID_LOG_DATASUS_DBC_DOWNLOAD` (`NEWSEQUENTIALID()` default).
- **FK:** `ID_LOG_DATASUS_DBC_FILE` → registry table.
- **Columns:** `DT_ATTEMPT_STARTED_UTC`, `DT_ATTEMPT_FINISHED_UTC`, `DC_FTP_HOST`, `EX_FTP_REMOTE_PATH`, `QT_REMOTE_SIZE_BYTES`, `DT_REMOTE_MODIFIED_UTC`, `DC_TRIGGER_REASON`, **`IC_SUCCESS`**, `DC_ERROR_MESSAGE`, `DC_LOCAL_PATH`, `QT_LOCAL_SIZE_BYTES`.

Suggested **`DC_TRIGGER_REASON`** values: `new`, `remote_size_changed`, `remote_mtime_changed`, `local_missing`, `local_size_mismatch`, `retry_after_error`.

---

## Change detection (when to re-download)

For each listed `.dbc` that passes catalog/year filters:

1. Load or create **`LOG_DATASUS_DBC_FILE`** for `(DC_FTP_HOST, EX_FTP_REMOTE_PATH)`.
2. Compare FTP listing **`Size`** / **`ModTime`** (UTC) to **`QT_REMOTE_SIZE_BYTES`** / **`DT_REMOTE_MODIFIED_UTC`** stored from the **last successful** download (missing row ⇒ new).
3. If **`QT_REMOTE_SIZE_BYTES` differs → re-download.**
4. If **`DT_REMOTE_MODIFIED_UTC` differs** (and both sides non-null) → re-download; if mtime is unreliable, **size-only** is acceptable.
5. If **`DC_LOCAL_PATH`** points to a **missing** file or **`QT_LOCAL_SIZE_BYTES` ≠ `QT_REMOTE_SIZE_BYTES`** → re-download.
6. On success: **UPDATE** registry (remote + local fields, `DT_LAST_DOWNLOAD_UTC`, `DC_LAST_DOWNLOAD_STATUS`, `NR_DOWNLOAD_COUNT += 1`, `DT_UPDATED_UTC`) and **INSERT** `LOG_DATASUS_DBC_DOWNLOAD` with **`IC_SUCCESS = 1`**. On failure: **INSERT** with **`IC_SUCCESS = 0`**; do not treat failed rows as the baseline for remote metadata unless you explicitly design retries that way.

---

## Implementation notes

- **Go:** Self-contained module [`downloader/`](downloader/) (`downloader.exe`); load config from `appsettings.json` only (path beside executable). Build: `cd downloader && go build -o downloader.exe .`
- **SQL Server driver:** `github.com/microsoft/go-mssqldb` with `sql.Open("sqlserver", cfg.ConnectionString)`.
- **Reuse:** `internal/domain.ParseFilename`, `internal/ftp` client/scanner patterns — or vendor-equivalent if the tool is split out later.
- **No Docker** (repo rule). **No dependency** on Postgres, `job_queue`, or RabbitMQ for this binary.
- **Agent map / NDRS pointers:** `.cursor/AGENTS.md`

---

## Deliverables checklist

- [ ] `appsettings.json` loaded from executable directory.
- [ ] FTP scan + filter + parallel download + atomic write (temp file + rename). **Files must be saved in year-based subfolders under `download.localRoot`.**
- [ ] SQL Server: upsert `LOG_DATASUS_DBC_FILE`, insert `LOG_DATASUS_DBC_DOWNLOAD` per attempt, change detection as above.
- [ ] Operational logging (`logging.level`).
