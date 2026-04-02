DROP INDEX IF EXISTS idx_candles_1d_pair_bucket_desc;
DROP INDEX IF EXISTS idx_candles_1h_pair_bucket_desc;
DROP INDEX IF EXISTS idx_tasks_status_window_start;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS claimed_at,
    DROP COLUMN IF EXISTS claimed_by;
