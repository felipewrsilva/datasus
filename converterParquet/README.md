# converterParquet (standalone)

Converts DATASUS `.dbc` files directly to consolidated **Parquet** (UTF-8 string columns, PLAIN encoding for faster writes on high-cardinality data, Snappy compression). Output layout is **`{output_folder}/{YYYY}/{logicalBase}.parquet`**, where **`YYYY` is the expanded calendar year from the parsed `.dbc` basename** (not the path on disk).

## Build

```powershell
cd converterParquet
go build -o converterParquet.exe .
```

## Configuration

Copy `appsettings.example.json` to `appsettings.json` next to the executable (or pass `-config`).

| Key | Meaning |
|-----|---------|
| `source_folder` | Root directory containing `.dbc` files |
| `output_folder` | Root; each file is written under `{output_folder}/{year}/` |
| `scan_subfolders` | When false, only files directly under `source_folder` |
| `max_scan_depth` | `0` = no subfolders; `N` = up to N levels; `-1` = unlimited (if `scan_subfolders` is true) |
| `sqlServer.connectionString` | SQL Server ADO/.NET style connection string (required unless `-dry-run`) |
| `logging.level` | `debug`, `info`, `warn`, `error` |
| `strict_segments` | When true, multi-part sets must have every letter between min and max segment (e.g. a+c without b fails) |
| `conflict_policy` | Only `error` is supported (unsegmented + segmented for same base is rejected) |
| `convert_timeout_seconds` | Per-merge timeout (default 600) |
| `parquet_row_group_mb` | Parquet row group size in MB (default 128) |
| `parquet_page_kb` | Parquet page size in KB (default 64) |
| `parquet_parallel_writers` | `parquet-go` CSV writer parallelism (default 4) |

Environment variables are not read by this tool; put settings in JSON.

## CLI flags

- `-config path` — alternate `appsettings.json` location
- `-dry-run` — scan, group, and print planned outputs and fingerprints; **no** Parquet writes and **no** SQL (connection string optional)
- `-verbose` — debug logging

## SQL

Run `sql/Create-Tables-ConverterParquet.sql` on the target database before first use. Tables:

- `dbo.LOG_DATASUS_DBC_PARQUET_ARTIFACT` — idempotency (`input_fingerprint`, `parquet_sha256`, status)
- `dbo.LOG_DATASUS_DBC_PARQUET_RUN` — audit log per attempt (sources JSON, row counts, errors)

## Edge cases

- **Empty or corrupt `.dbc`:** conversion fails; run row is inserted with `IC_SUCCESS = 0` and `DC_ERROR_MESSAGE` set; artifact row updated with `failed` status.
- **Duplicate output from multiple sources:** two unsegmented files for the same logical base → error before conversion.
- **Partial segment sets:** non-strict mode logs a warning; `strict_segments` → error if letters between min and max are missing.
- **Base + segmented both present:** error (e.g. `CAES2401.dbc` and `CAES2401a.dbc`).
- **Skip when fingerprint matches:** if artifact row matches current fingerprint, output file exists, and on-disk SHA-256 matches stored `BN_PARQUET_SHA256_HEX`, the group is skipped (logged at debug).
- **Artifact row exists but Parquet missing:** skip check fails (stat or hash mismatch) → file is rebuilt and hashes updated.

## Input fingerprint

Same formula as `converterCSV`: SHA-256 (hex, lowercase) of UTF-8 text built from contributing `.dbc` files sorted by segment order (then relative path): each line is `relativePath + NUL + sizeBytes + NUL + fileSha256Hex + LF`. See `internal/hash/fingerprint.go`.

## Module layout

This folder is a **separate Go module** (`go.mod`). It does not import packages from `converterCSV`, `legacy`, or other monorepo trees—only standard library and published modules—so you can copy the built executable, config, and SQL scripts elsewhere without the rest of the repo.
