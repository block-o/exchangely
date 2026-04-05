CREATE TABLE IF NOT EXISTS dismissed_warnings (
    warning_id TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    dismissed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
