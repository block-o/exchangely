ALTER TABLE integrity_coverage
    DROP COLUMN IF EXISTS gap_count,
    DROP COLUMN IF EXISTS divergence_count,
    DROP COLUMN IF EXISTS sources_checked,
    DROP COLUMN IF EXISTS error_message;
