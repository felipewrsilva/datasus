DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_enum
        WHERE enumtypid = 'overall_status'::regtype
          AND enumlabel = 'new'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_enum
        WHERE enumtypid = 'overall_status'::regtype
          AND enumlabel = 'pending'
    ) THEN
        ALTER TYPE overall_status RENAME VALUE 'new' TO 'pending';
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_enum
        WHERE enumtypid = 'overall_status'::regtype
          AND enumlabel = 'ignored'
    ) THEN
        ALTER TYPE overall_status ADD VALUE 'ignored' AFTER 'pending';
    END IF;
END
$$;

ALTER TABLE files
    ALTER COLUMN overall_status SET DEFAULT 'pending';
