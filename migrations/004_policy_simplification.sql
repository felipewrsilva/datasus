ALTER TABLE download_catalog_policy
    DROP COLUMN IF EXISTS mode,
    DROP COLUMN IF EXISTS valid_until;
