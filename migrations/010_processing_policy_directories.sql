ALTER TABLE processing_policy_config
    ADD COLUMN IF NOT EXISTS download_dir TEXT NULL,
    ADD COLUMN IF NOT EXISTS csv_dir TEXT NULL,
    ADD COLUMN IF NOT EXISTS parquet_dir TEXT NULL;
