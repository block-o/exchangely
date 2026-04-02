CREATE TABLE IF NOT EXISTS raw_candles (
    pair_symbol TEXT NOT NULL,
    interval TEXT NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL,
    open NUMERIC NOT NULL,
    high NUMERIC NOT NULL,
    low NUMERIC NOT NULL,
    close NUMERIC NOT NULL,
    volume NUMERIC NOT NULL,
    finalized BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (pair_symbol, interval, bucket_start, source)
);

SELECT create_hypertable('raw_candles', 'bucket_start', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_raw_candles_pair_interval_bucket
    ON raw_candles (pair_symbol, interval, bucket_start DESC);
