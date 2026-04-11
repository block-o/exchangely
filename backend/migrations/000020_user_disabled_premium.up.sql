-- Drop the existing CHECK constraint to allow 'premium' role.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

-- Add disabled column for soft-disable functionality.
ALTER TABLE users ADD COLUMN IF NOT EXISTS disabled BOOLEAN NOT NULL DEFAULT false;
