CREATE TABLE IF NOT EXISTS download_policy_catalogs (
    catalog CHAR(2) PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS download_policy_years (
    year SMALLINT PRIMARY KEY CHECK (year >= 0 AND year <= 9999)
);

CREATE TABLE IF NOT EXISTS download_policy_months (
    year SMALLINT NOT NULL CHECK (year >= 0 AND year <= 9999),
    month SMALLINT NOT NULL CHECK (month >= 1 AND month <= 12),
    PRIMARY KEY (year, month)
);

INSERT INTO download_policy_catalogs (catalog)
SELECT DISTINCT p.catalog
FROM download_catalog_policy p
WHERE p.download_enabled = true
ON CONFLICT (catalog) DO NOTHING;

INSERT INTO download_policy_months (year, month)
SELECT DISTINCT ap.year, ap.month
FROM download_active_periods ap
JOIN download_catalog_policy p ON p.catalog = ap.catalog
WHERE p.download_enabled = true
ON CONFLICT (year, month) DO NOTHING;
