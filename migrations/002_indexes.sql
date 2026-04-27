-- 002_indexes.sql: performance indexes for hot query paths

-- Queue polling: partial index over pending rows only — keeps scans fast under load
CREATE INDEX idx_job_queue_claim
    ON job_queue (stage, available_at ASC)
    WHERE status = 'pending';

-- File listing filters
CREATE INDEX idx_files_catalog_state  ON files (catalog, state);
CREATE INDEX idx_files_year_month     ON files (year, month);
CREATE INDEX idx_files_overall_status ON files (overall_status);
CREATE INDEX idx_files_last_seen_at   ON files (last_seen_at DESC);
CREATE INDEX idx_files_ftp_dir        ON files (ftp_dir);
CREATE INDEX idx_files_catalog_status_period ON files (catalog, overall_status, year, month);
CREATE INDEX idx_files_updated_at     ON files (updated_at DESC);
CREATE INDEX idx_files_filename_lower ON files (LOWER(filename));

-- Stage lookups
CREATE INDEX idx_file_stages_file_id ON file_stages (file_id);
CREATE INDEX idx_file_stages_status  ON file_stages (stage, status);

-- Log queries (most recent first per file)
CREATE INDEX idx_download_logs_file_id ON download_logs (file_id, created_at DESC);
CREATE INDEX idx_file_stages_failed_updated ON file_stages (updated_at DESC) WHERE status='failed';
