-- Download policy: per-catalog opt-in, plus whitelist (year, month) when a row exists.

CREATE TABLE download_catalog_policy (
    catalog         CHAR(2) PRIMARY KEY,
    download_enabled BOOLEAN NOT NULL,
    mode            TEXT NOT NULL DEFAULT 'allow_periods' CHECK (mode IN ('allow_all', 'deny_all', 'allow_periods')),
    valid_until     DATE
);

CREATE TABLE download_active_periods (
    catalog CHAR(2)  NOT NULL REFERENCES download_catalog_policy (catalog) ON DELETE CASCADE,
    year    SMALLINT  NOT NULL CHECK (year >= 1990 AND year <= 2100),
    month   SMALLINT  NOT NULL CHECK (month >= 1 AND month <= 12),
    PRIMARY KEY (catalog, year, month)
);

CREATE INDEX download_active_periods_catalog_idx
    ON download_active_periods (catalog);

CREATE TABLE manual_action_audit (
    id           BIGSERIAL PRIMARY KEY,
    action       TEXT NOT NULL,
    stage        stage_name,
    actor        TEXT NOT NULL DEFAULT 'ui',
    details_json JSONB,
    created_at   TIMESTAMP NOT NULL DEFAULT now()
);
