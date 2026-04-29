-- Operational checks for pipeline policy vs file_stages vs paths (run against app DB).
-- 1) Global processing toggles
SELECT id, enable_download, enable_csv, enable_parquet
FROM processing_policy_config
WHERE id = 1;

-- 2) Files where parquet stage is done but parquet_path is null (data integrity)
SELECT f.id, f.filename, f.overall_status, f.parquet_path, fs.status AS parquet_stage
FROM files f
JOIN file_stages fs ON fs.file_id = f.id AND fs.stage = 'parquet_conversion'
WHERE fs.status = 'done'
  AND (f.parquet_path IS NULL OR btrim(f.parquet_path::text) = '');

-- 3) Stage done counts (same idea as API stage_done_counts)
SELECT
  COUNT(*) FILTER (WHERE stage = 'download' AND status = 'done') AS download_done,
  COUNT(*) FILTER (WHERE stage = 'csv_conversion' AND status = 'done') AS csv_done,
  COUNT(*) FILTER (WHERE stage = 'parquet_conversion' AND status = 'done') AS parquet_done
FROM file_stages;

-- 4) Per-file stages for one id (replace UUID): compare overall_status to file_stages rows
-- SELECT stage, status FROM file_stages WHERE file_id = '00000000-0000-0000-0000-000000000000'::uuid ORDER BY stage;
