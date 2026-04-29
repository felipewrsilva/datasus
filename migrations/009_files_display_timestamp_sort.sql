-- Supports default file list ordering by COALESCE(remote_timestamp, last_seen_at) (aligned with UI).

CREATE INDEX idx_files_coalesce_remote_last_seen
    ON files ((COALESCE(remote_timestamp, last_seen_at)) DESC);
