-- 011_files_segment.sql: optional single-letter segment for multi-part DATASUS .dbc files (e.g. RDSP2401A.DBC)

ALTER TABLE files ADD COLUMN IF NOT EXISTS segment CHAR(1) NULL;

-- Logical uniqueness: one row per (catalog, state, calendar period, segment); non-segmented uses NULL segment coalesced to ''.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_files_logical_key
    ON files (catalog, state, year, month, (COALESCE(btrim(segment::text), '')));

CREATE INDEX IF NOT EXISTS idx_files_year_month_segment
    ON files (year, month, segment);

-- Backfill segment from filename when the 9th character before .dbc is a letter (RDSP2401A.DBC -> A).
UPDATE files
SET segment = UPPER(SUBSTRING(filename FROM 9 FOR 1))::char(1)
WHERE segment IS NULL
  AND LENGTH(filename) >= 9
  AND SUBSTRING(upper(filename) FROM 9 FOR 1) ~ '^[A-Z]$';
