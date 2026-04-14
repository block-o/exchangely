-- Remove duplicate exchange credentials per user, keeping only the most recent one.
DELETE FROM exchange_credentials
WHERE id NOT IN (
    SELECT DISTINCT ON (user_id, exchange) id
    FROM exchange_credentials
    ORDER BY user_id, exchange, created_at DESC
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_exchange_credentials_user_exchange
    ON exchange_credentials(user_id, exchange);
