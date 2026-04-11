-- Optimise the tickerSnapshotQuery hot path.
--
-- 1. Partial index for the two DISTINCT ON scans on raw_candles WHERE interval='1h'.
--    Covers latest_raw (pair_symbol, bucket_start DESC, updated_at DESC)
--    and native_v24h (pair_symbol, source, bucket_start DESC, updated_at DESC).
CREATE INDEX IF NOT EXISTS idx_raw_candles_1h_pair_bucket_updated
    ON raw_candles (pair_symbol, bucket_start DESC, updated_at DESC)
    WHERE interval = '1h';

CREATE INDEX IF NOT EXISTS idx_raw_candles_1h_pair_source_bucket_updated
    ON raw_candles (pair_symbol, source, bucket_start DESC, updated_at DESC)
    WHERE interval = '1h';
