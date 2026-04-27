-- 001_initial.sql: core schema for the DATASUS pipeline

CREATE TYPE stage_name AS ENUM ('download', 'csv_conversion', 'parquet_conversion');
CREATE TYPE stage_status AS ENUM ('pending', 'running', 'done', 'failed', 'purged');
CREATE TYPE overall_status AS ENUM (
    'pending', 'ignored', 'downloading', 'downloaded',
    'converting_csv', 'csv_ready',
    'converting_parquet', 'parquet_ready',
    'failed', 'purged'
);
CREATE TYPE queue_status AS ENUM ('pending', 'running', 'done', 'failed');
CREATE TYPE log_event AS ENUM (
    'enqueued', 'started', 'progress', 'completed',
    'failed', 'retrying', 'purged'
);

CREATE TABLE files (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    filename         TEXT         NOT NULL UNIQUE,
    catalog          CHAR(2)      NOT NULL,
    state            CHAR(2)      NOT NULL,
    year             SMALLINT     NOT NULL,
    month            SMALLINT     NOT NULL,
    ftp_dir          TEXT         NOT NULL,
    ftp_path         TEXT         NOT NULL,
    size_bytes       BIGINT,
    remote_checksum  TEXT,
    remote_timestamp TIMESTAMPTZ,
    local_hash       TEXT,
    root_path        TEXT         NOT NULL,
    dbc_path         TEXT,
    csv_path         TEXT,
    parquet_path     TEXT,
    overall_status   overall_status NOT NULL DEFAULT 'pending',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_seen_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE file_stages (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id       UUID         NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    stage         stage_name   NOT NULL,
    status        stage_status NOT NULL DEFAULT 'pending',
    attempts      INTEGER      NOT NULL DEFAULT 0,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    error_message TEXT,
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (file_id, stage)
);

CREATE TABLE download_logs (
    id           BIGSERIAL    PRIMARY KEY,
    file_id      UUID         NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    stage        stage_name   NOT NULL,
    event_type   log_event    NOT NULL,
    message      TEXT,
    payload_json JSONB,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE job_queue (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id      UUID         NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    stage        stage_name   NOT NULL,
    status       queue_status NOT NULL DEFAULT 'pending',
    available_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    locked_at    TIMESTAMPTZ,
    locked_by    TEXT,
    attempts     INTEGER      NOT NULL DEFAULT 0,
    payload_json JSONB,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (file_id, stage)
);

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_files_updated_at
    BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_file_stages_updated_at
    BEFORE UPDATE ON file_stages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_job_queue_updated_at
    BEFORE UPDATE ON job_queue
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
