CREATE TABLE IF NOT EXISTS processing_policy_config (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    enable_download BOOLEAN NOT NULL DEFAULT TRUE,
    enable_csv BOOLEAN NOT NULL DEFAULT TRUE,
    enable_parquet BOOLEAN NOT NULL DEFAULT TRUE
);

INSERT INTO processing_policy_config (id, enable_download, enable_csv, enable_parquet)
VALUES (1, TRUE, TRUE, TRUE)
ON CONFLICT (id) DO NOTHING;
