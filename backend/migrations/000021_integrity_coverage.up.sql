CREATE TABLE IF NOT EXISTS integrity_coverage (
    pair_symbol TEXT NOT NULL,
    day DATE NOT NULL,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (pair_symbol, day)
);

CREATE INDEX IF NOT EXISTS idx_integrity_coverage_active ON integrity_coverage (pair_symbol, day) WHERE verified = TRUE;
