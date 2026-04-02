UPDATE sync_status
SET daily_backfill_completed = FALSE,
    daily_last_synced_at = NULL,
    backfill_completed = FALSE,
    updated_at = NOW()
WHERE hourly_backfill_completed = FALSE
  AND daily_backfill_completed = TRUE;
