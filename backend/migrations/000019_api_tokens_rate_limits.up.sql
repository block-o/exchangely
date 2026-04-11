CREATE TABLE IF NOT EXISTS api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE NOT NULL,
    label TEXT NOT NULL,
    prefix TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens (token_hash);

CREATE TABLE IF NOT EXISTS api_rate_limit_log (
    id BIGSERIAL PRIMARY KEY,
    token_id UUID REFERENCES api_tokens(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ip_address TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_token_requested ON api_rate_limit_log (token_id, requested_at);
CREATE INDEX IF NOT EXISTS idx_rate_limit_ip_requested ON api_rate_limit_log (ip_address, requested_at);
CREATE INDEX IF NOT EXISTS idx_rate_limit_user_requested ON api_rate_limit_log (user_id, requested_at);
