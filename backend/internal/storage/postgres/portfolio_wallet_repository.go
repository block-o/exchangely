package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

// WalletRepository implements portfolio.WalletRepository using PostgreSQL.
type WalletRepository struct {
	db *sql.DB
}

// Compile-time check that WalletRepository satisfies the portfolio.WalletRepository interface.
var _ portfolio.WalletRepository = (*WalletRepository)(nil)

// NewWalletRepository returns a new WalletRepository backed by the given database.
func NewWalletRepository(db *sql.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

// Create inserts a new wallet address. The wallet's CreatedAt and UpdatedAt
// fields are set server-side by the database defaults and scanned back.
func (r *WalletRepository) Create(ctx context.Context, w *portfolio.WalletAddress) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO wallet_addresses (id, user_id, chain, address_prefix,
			address_cipher, address_nonce, label_cipher, label_nonce)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at
	`,
		w.ID, w.UserID, w.Chain, w.AddressPrefix,
		w.AddressCipher, w.AddressNonce, w.LabelCipher, w.LabelNonce,
	).Scan(&w.CreatedAt, &w.UpdatedAt)
}

// FindByID returns the wallet address with the given ID owned by the given user,
// or nil if not found.
func (r *WalletRepository) FindByID(ctx context.Context, id, userID uuid.UUID) (*portfolio.WalletAddress, error) {
	var w portfolio.WalletAddress

	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, chain, address_prefix, address_cipher, address_nonce,
			label_cipher, label_nonce, last_sync_at, created_at, updated_at
		FROM wallet_addresses
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&w.ID, &w.UserID, &w.Chain, &w.AddressPrefix,
		&w.AddressCipher, &w.AddressNonce,
		&w.LabelCipher, &w.LabelNonce, &w.LastSyncAt,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListByUserID returns all wallet addresses belonging to the given user.
func (r *WalletRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]portfolio.WalletAddress, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, chain, address_prefix, address_cipher, address_nonce,
			label_cipher, label_nonce, last_sync_at, created_at, updated_at
		FROM wallet_addresses
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var wallets []portfolio.WalletAddress
	for rows.Next() {
		var w portfolio.WalletAddress
		if err := rows.Scan(
			&w.ID, &w.UserID, &w.Chain, &w.AddressPrefix,
			&w.AddressCipher, &w.AddressNonce,
			&w.LabelCipher, &w.LabelNonce, &w.LastSyncAt,
			&w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		wallets = append(wallets, w)
	}
	return wallets, rows.Err()
}

// Delete removes a wallet address by ID, scoped to the given user.
func (r *WalletRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM wallet_addresses
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

// UpdateSyncTime updates the last_sync_at and updated_at timestamps for a wallet.
func (r *WalletRepository) UpdateSyncTime(ctx context.Context, id uuid.UUID, syncTime time.Time) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE wallet_addresses
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
