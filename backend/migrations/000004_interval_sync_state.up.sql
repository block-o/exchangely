ALTER TABLE sync_status
    ADD COLUMN IF NOT EXISTS hourly_last_synced_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS daily_last_synced_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hourly_backfill_completed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS daily_backfill_completed BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE sync_status
SET hourly_last_synced_at = COALESCE(hourly_last_synced_at, last_synced_at),
    hourly_backfill_completed = COALESCE(hourly_backfill_completed, backfill_completed)
WHERE last_synced_at IS NOT NULL
   OR backfill_completed IS NOT NULL;
