ALTER TABLE portfolio_transactions
    DROP COLUMN IF EXISTS fee,
    DROP COLUMN IF EXISTS fee_currency;
