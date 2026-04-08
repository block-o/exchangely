-- Add v24h column to raw_candles for native exchange 24h volume snapshots
ALTER TABLE raw_candles ADD COLUMN IF NOT EXISTS v24h NUMERIC NOT NULL DEFAULT 0;
