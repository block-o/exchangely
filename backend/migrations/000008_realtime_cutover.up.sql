ALTER TABLE sync_status
    ADD COLUMN IF NOT EXISTS hourly_realtime_started_at TIMESTAMPTZ;
