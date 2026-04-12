package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

// CredentialRepository implements portfolio.CredentialRepository using PostgreSQL.
type CredentialRepository struct {
	db *sql.DB
}

// Compile-time check that CredentialRepository satisfies the portfolio.CredentialRepository interface.
var _ portfolio.CredentialRepository = (*CredentialRepository)(nil)

// NewCredentialRepository returns a new CredentialRepository backed by the given database.
func NewCredentialRepository(db *sql.DB) *CredentialRepository {
	return &CredentialRepository{db: db}
}

// Create inserts a new exchange credential. The credential's CreatedAt and UpdatedAt
// fields are set server-side by the database defaults and scanned back.
func (r *CredentialRepository) Create(ctx context.Context, c *portfolio.ExchangeCredential) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO exchange_credentials (id, user_id, exchange, api_key_prefix,
			api_key_cipher, key_nonce, secret_cipher, secret_nonce, status, error_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`,
		c.ID, c.UserID, c.Exchange, c.APIKeyPrefix,
		c.APIKeyCipher, c.KeyNonce, c.SecretCipher, c.Nonce,
		c.Status, c.ErrorReason,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

// FindByID returns the credential with the given ID owned by the given user,
// or nil if not found.
func (r *CredentialRepository) FindByID(ctx context.Context, id, userID uuid.UUID) (*portfolio.ExchangeCredential, error) {
	var c portfolio.ExchangeCredential

	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, exchange, api_key_prefix, api_key_cipher, key_nonce,
			secret_cipher, secret_nonce, status, error_reason, last_sync_at,
			created_at, updated_at
		FROM exchange_credentials
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&c.ID, &c.UserID, &c.Exchange, &c.APIKeyPrefix,
		&c.APIKeyCipher, &c.KeyNonce, &c.SecretCipher, &c.Nonce,
		&c.Status, &c.ErrorReason, &c.LastSyncAt,
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

// ListByUserID returns all exchange credentials belonging to the given user.
func (r *CredentialRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]portfolio.ExchangeCredential, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, exchange, api_key_prefix, api_key_cipher, key_nonce,
			secret_cipher, secret_nonce, status, error_reason, last_sync_at,
			created_at, updated_at
		FROM exchange_credentials
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var creds []portfolio.ExchangeCredential
	for rows.Next() {
		var c portfolio.ExchangeCredential
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.Exchange, &c.APIKeyPrefix,
			&c.APIKeyCipher, &c.KeyNonce, &c.SecretCipher, &c.Nonce,
			&c.Status, &c.ErrorReason, &c.LastSyncAt,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// Delete removes an exchange credential by ID, scoped to the given user.
func (r *CredentialRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM exchange_credentials
		WHERE id = $1 AND user_id = $2
	`, id, userID)
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

// UpdateSyncStatus updates the sync status, error reason, last sync time, and
// updated_at timestamp for a credential.
func (r *CredentialRepository) UpdateSyncStatus(ctx context.Context, id uuid.UUID, status string, errorReason *string, syncTime *time.Time) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE exchange_credentials
		SET status = $2, error_reason = $3, last_sync_at = $4, updated_at = $5
		WHERE id = $1
	`, id, status, errorReason, syncTime, now)
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
