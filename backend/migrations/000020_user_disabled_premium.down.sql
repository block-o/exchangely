-- Remove disabled column.
ALTER TABLE users DROP COLUMN IF EXISTS disabled;

-- Restore original CHECK constraint (only admin/user).
-- First update any premium users back to user so the constraint can be applied.
UPDATE users SET role = 'user' WHERE role = 'premium';
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'user'));
