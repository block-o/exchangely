CREATE TABLE IF NOT EXISTS exchange_credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exchange      TEXT NOT NULL CHECK (exchange IN ('binance', 'kraken', 'coinbase')),
    api_key_prefix TEXT NOT NULL,
    api_key_cipher BYTEA NOT NULL,
    key_nonce     BYTEA NOT NULL,
    secret_cipher BYTEA NOT NULL,
    secret_nonce  BYTEA NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'failed')),
    error_reason  TEXT,
    last_sync_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_exchange_credentials_user ON exchange_credentials(user_id);

CREATE TABLE IF NOT EXISTS wallet_addresses (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chain          TEXT NOT NULL CHECK (chain IN ('ethereum', 'solana', 'bitcoin')),
    address_prefix TEXT NOT NULL,
    address_cipher BYTEA NOT NULL,
    address_nonce  BYTEA NOT NULL,
    label_cipher   BYTEA,
    label_nonce    BYTEA,
    last_sync_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wallet_addresses_user ON wallet_addresses(user_id);

CREATE TABLE IF NOT EXISTS ledger_credentials (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    token_cipher BYTEA NOT NULL,
    token_nonce  BYTEA NOT NULL,
    last_sync_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS portfolio_holdings (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    asset_symbol   TEXT NOT NULL,
    quantity       DOUBLE PRECISION NOT NULL CHECK (quantity > 0),
    avg_buy_price  DOUBLE PRECISION,
    quote_currency TEXT NOT NULL DEFAULT 'USD',
    source         TEXT NOT NULL CHECK (source IN ('manual', 'binance', 'kraken', 'coinbase', 'ethereum', 'solana', 'bitcoin', 'ledger')),
    source_ref     UUID,
    notes_cipher   BYTEA,
    notes_nonce    BYTEA,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, asset_symbol, source, source_ref)
);

CREATE INDEX IF NOT EXISTS idx_portfolio_holdings_user ON portfolio_holdings(user_id);
CREATE INDEX IF NOT EXISTS idx_portfolio_holdings_source_ref ON portfolio_holdings(source_ref);
