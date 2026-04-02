ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS claimed_by TEXT,
    ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_error TEXT;

CREATE INDEX IF NOT EXISTS idx_tasks_status_window_start
    ON tasks (status, window_start);

CREATE INDEX IF NOT EXISTS idx_candles_1h_pair_bucket_desc
    ON candles_1h (pair_symbol, bucket_start DESC);

CREATE INDEX IF NOT EXISTS idx_candles_1d_pair_bucket_desc
    ON candles_1d (pair_symbol, bucket_start DESC);
