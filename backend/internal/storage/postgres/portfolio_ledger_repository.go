package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

// LedgerCredentialRepository implements portfolio.LedgerCredentialRepository using PostgreSQL.
type LedgerCredentialRepository struct {
	db *sql.DB
}

// Compile-time check that LedgerCredentialRepository satisfies the portfolio.LedgerCredentialRepository interface.
var _ portfolio.LedgerCredentialRepository = (*LedgerCredentialRepository)(nil)

// NewLedgerCredentialRepository returns a new LedgerCredentialRepository backed by the given database.
func NewLedgerCredentialRepository(db *sql.DB) *LedgerCredentialRepository {
	return &LedgerCredentialRepository{db: db}
}

// Create inserts a new Ledger credential. The credential's CreatedAt and UpdatedAt
// fields are set server-side by the database defaults and scanned back.
// The ledger_credentials table has a UNIQUE constraint on user_id (one Ledger per user).
func (r *LedgerCredentialRepository) Create(ctx context.Context, c *portfolio.LedgerCredential) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO ledger_credentials (id, user_id, token_cipher, token_nonce)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at
	`,
		c.ID, c.UserID, c.TokenCipher, c.TokenNonce,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

// FindByUserID returns the Ledger credential for the given user, or nil if not found.
func (r *LedgerCredentialRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*portfolio.LedgerCredential, error) {
	var c portfolio.LedgerCredential

	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_cipher, token_nonce, last_sync_at,
			created_at, updated_at
		FROM ledger_credentials
		WHERE user_id = $1
	`, userID).Scan(
		&c.ID, &c.UserID, &c.TokenCipher, &c.TokenNonce, &c.LastSyncAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Delete removes the Ledger credential for the given user.
func (r *LedgerCredentialRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM ledger_credentials
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateSyncTime updates the last_sync_at and updated_at timestamps for a Ledger credential.
func (r *LedgerCredentialRepository) UpdateSyncTime(ctx context.Context, id uuid.UUID, syncTime time.Time) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE ledger_credentials
		SET last_sync_at = $2, updated_at = $3
		WHERE id = $1
	`, id, syncTime, now)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
