CREATE TABLE IF NOT EXISTS data_coverage (
    pair_symbol TEXT NOT NULL,
    day DATE NOT NULL,
    is_complete BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (pair_symbol, day)
);

CREATE INDEX IF NOT EXISTS idx_data_coverage_active ON data_coverage (pair_symbol, day) WHERE is_complete = TRUE;
