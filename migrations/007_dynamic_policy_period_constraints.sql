DO $$
DECLARE
    c RECORD;
BEGIN
    FOR c IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'download_policy_years'::regclass
          AND contype = 'c'
    LOOP
        EXECUTE format('ALTER TABLE download_policy_years DROP CONSTRAINT %I', c.conname);
    END LOOP;

    ALTER TABLE download_policy_years
        ADD CONSTRAINT download_policy_years_year_check
        CHECK (year >= 0 AND year <= 9999);
END $$;

DO $$
DECLARE
    c RECORD;
BEGIN
    FOR c IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'download_policy_months'::regclass
          AND contype = 'c'
    LOOP
        EXECUTE format('ALTER TABLE download_policy_months DROP CONSTRAINT %I', c.conname);
    END LOOP;

    ALTER TABLE download_policy_months
        ADD CONSTRAINT download_policy_months_year_check
        CHECK (year >= 0 AND year <= 9999);

    ALTER TABLE download_policy_months
        ADD CONSTRAINT download_policy_months_month_check
        CHECK (month >= 1 AND month <= 12);
END $$;
