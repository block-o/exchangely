ALTER TABLE sync_status
    DROP COLUMN IF EXISTS daily_backfill_completed,
    DROP COLUMN IF EXISTS hourly_backfill_completed,
    DROP COLUMN IF EXISTS daily_last_synced_at,
    DROP COLUMN IF EXISTS hourly_last_synced_at;
