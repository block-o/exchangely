CREATE TABLE IF NOT EXISTS portfolio_transactions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    asset_symbol       TEXT NOT NULL,
    quantity           DOUBLE PRECISION NOT NULL,
    tx_type            TEXT NOT NULL CHECK (tx_type IN ('buy', 'sell', 'transfer', 'fee')),
    tx_timestamp       TIMESTAMPTZ NOT NULL,
    source             TEXT NOT NULL,
    source_ref         TEXT,
    reference_value    DOUBLE PRECISION,
    reference_currency TEXT NOT NULL DEFAULT 'USD',
    resolution         TEXT NOT NULL DEFAULT 'unresolvable'
                       CHECK (resolution IN ('exact', 'hourly', 'daily', 'unresolvable')),
    manually_edited    BOOLEAN NOT NULL DEFAULT FALSE,
    notes              TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, source, source_ref, asset_symbol, tx_timestamp)
);

CREATE INDEX IF NOT EXISTS idx_portfolio_tx_user ON portfolio_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_portfolio_tx_user_asset ON portfolio_transactions(user_id, asset_symbol);
CREATE INDEX IF NOT EXISTS idx_portfolio_tx_user_ts ON portfolio_transactions(user_id, tx_timestamp DESC);

CREATE TABLE IF NOT EXISTS pnl_snapshots (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reference_currency TEXT NOT NULL DEFAULT 'USD',
    total_realized     DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_unrealized   DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_pnl          DOUBLE PRECISION NOT NULL DEFAULT 0,
    has_approximate    BOOLEAN NOT NULL DEFAULT FALSE,
    excluded_count     INTEGER NOT NULL DEFAULT 0,
    assets_json        JSONB NOT NULL DEFAULT '[]',
    computed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, reference_currency)
);

CREATE INDEX IF NOT EXISTS idx_pnl_snapshots_user ON pnl_snapshots(user_id);
