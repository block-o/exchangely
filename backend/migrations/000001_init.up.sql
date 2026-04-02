CREATE TABLE IF NOT EXISTS assets (
    symbol TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    asset_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pairs (
    symbol TEXT PRIMARY KEY,
    base_asset TEXT NOT NULL REFERENCES assets(symbol),
    quote_asset TEXT NOT NULL REFERENCES assets(symbol),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS service_leases (
    lease_name TEXT PRIMARY KEY,
    holder_id TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    task_type TEXT NOT NULL,
    pair_symbol TEXT NOT NULL,
    interval TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sync_status (
    pair_symbol TEXT PRIMARY KEY,
    last_synced_at TIMESTAMPTZ,
    backfill_completed BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS candles_1h (
    pair_symbol TEXT NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,
    open NUMERIC NOT NULL,
    high NUMERIC NOT NULL,
    low NUMERIC NOT NULL,
    close NUMERIC NOT NULL,
    volume NUMERIC NOT NULL,
    source TEXT NOT NULL,
    finalized BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (pair_symbol, bucket_start)
);

CREATE TABLE IF NOT EXISTS candles_1d (
    pair_symbol TEXT NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,
    open NUMERIC NOT NULL,
    high NUMERIC NOT NULL,
    low NUMERIC NOT NULL,
    close NUMERIC NOT NULL,
    volume NUMERIC NOT NULL,
    source TEXT NOT NULL,
    finalized BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (pair_symbol, bucket_start)
);

SELECT create_hypertable('candles_1h', 'bucket_start', if_not_exists => TRUE);
SELECT create_hypertable('candles_1d', 'bucket_start', if_not_exists => TRUE);
