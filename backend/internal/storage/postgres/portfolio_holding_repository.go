package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

// HoldingRepository implements portfolio.HoldingRepository using PostgreSQL.
type HoldingRepository struct {
	db *sql.DB
}

// Compile-time check that HoldingRepository satisfies the portfolio.HoldingRepository interface.
var _ portfolio.HoldingRepository = (*HoldingRepository)(nil)

// NewHoldingRepository returns a new HoldingRepository backed by the given database.
func NewHoldingRepository(db *sql.DB) *HoldingRepository {
	return &HoldingRepository{db: db}
}

// Create inserts a new portfolio holding. The holding's CreatedAt and UpdatedAt
// fields are set server-side by the database defaults and scanned back.
func (r *HoldingRepository) Create(ctx context.Context, h *portfolio.Holding) error {
	var notesCipher []byte
	if h.Notes != "" {
		notesCipher = []byte(h.Notes)
	}

	var sourceRef *uuid.UUID
	if h.SourceRef != nil {
		parsed, err := uuid.Parse(*h.SourceRef)
		if err != nil {
			return err
		}
		sourceRef = &parsed
	}

	return r.db.QueryRowContext(ctx, `
		INSERT INTO portfolio_holdings (id, user_id, asset_symbol, quantity, avg_buy_price,
			quote_currency, source, source_ref, notes_cipher)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at
	`,
		h.ID, h.UserID, h.AssetSymbol, h.Quantity, h.AvgBuyPrice,
		h.QuoteCurrency, h.Source, sourceRef, notesCipher,
	).Scan(&h.CreatedAt, &h.UpdatedAt)
}

// Update modifies an existing portfolio holding. Only the owning user's holding
// is updated (user_id filter ensures data isolation).
func (r *HoldingRepository) Update(ctx context.Context, h *portfolio.Holding) error {
	var notesCipher []byte
	if h.Notes != "" {
		notesCipher = []byte(h.Notes)
	}

	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE portfolio_holdings
		SET asset_symbol = $3, quantity = $4, avg_buy_price = $5,
			quote_currency = $6, source = $7, notes_cipher = $8, updated_at = $9
		WHERE id = $1 AND user_id = $2
	`,
		h.ID, h.UserID, h.AssetSymbol, h.Quantity, h.AvgBuyPrice,
		h.QuoteCurrency, h.Source, notesCipher, now,
	)
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
	h.UpdatedAt = now
	return nil
}

// Delete removes a portfolio holding by ID, scoped to the given user.
func (r *HoldingRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM portfolio_holdings
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

// FindByID returns the holding with the given ID owned by the given user,
// or nil if not found.
func (r *HoldingRepository) FindByID(ctx context.Context, id, userID uuid.UUID) (*portfolio.Holding, error) {
	var h portfolio.Holding
	var notesCipher []byte
	var sourceRef *uuid.UUID

	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, asset_symbol, quantity, avg_buy_price, quote_currency,
			source, source_ref, notes_cipher, created_at, updated_at
		FROM portfolio_holdings
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&h.ID, &h.UserID, &h.AssetSymbol, &h.Quantity, &h.AvgBuyPrice,
		&h.QuoteCurrency, &h.Source, &sourceRef, &notesCipher,
		&h.CreatedAt, &h.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if sourceRef != nil {
		s := sourceRef.String()
		h.SourceRef = &s
	}
	if notesCipher != nil {
		h.Notes = string(notesCipher)
	}
	return &h, nil
}

// ListByUserID returns all holdings belonging to the given user.
func (r *HoldingRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]portfolio.Holding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, asset_symbol, quantity, avg_buy_price, quote_currency,
			source, source_ref, notes_cipher, created_at, updated_at
		FROM portfolio_holdings
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var holdings []portfolio.Holding
	for rows.Next() {
		var h portfolio.Holding
		var notesCipher []byte
		var sourceRef *uuid.UUID

		if err := rows.Scan(
			&h.ID, &h.UserID, &h.AssetSymbol, &h.Quantity, &h.AvgBuyPrice,
			&h.QuoteCurrency, &h.Source, &sourceRef, &notesCipher,
			&h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if sourceRef != nil {
			s := sourceRef.String()
			h.SourceRef = &s
		}
		if notesCipher != nil {
			h.Notes = string(notesCipher)
		}
		holdings = append(holdings, h)
	}
	return holdings, rows.Err()
}

// UpsertBySource upserts holdings from an exchange or wallet sync. Uses the
// PostgreSQL ON CONFLICT clause on (user_id, asset_symbol, source, source_ref)
// to insert new holdings or update existing ones.
func (r *HoldingRepository) UpsertBySource(ctx context.Context, userID uuid.UUID, source, sourceRef string, holdings []portfolio.Holding) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	parsedRef, err := uuid.Parse(sourceRef)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO portfolio_holdings (id, user_id, asset_symbol, quantity, avg_buy_price,
			quote_currency, source, source_ref, notes_cipher)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, asset_symbol, source, source_ref) DO UPDATE
		SET quantity = EXCLUDED.quantity,
			avg_buy_price = EXCLUDED.avg_buy_price,
			notes_cipher = EXCLUDED.notes_cipher,
			updated_at = now()
	`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for i := range holdings {
		h := &holdings[i]
		if h.ID == uuid.Nil {
			h.ID = uuid.New()
		}

		var notesCipher []byte
		if h.Notes != "" {
			notesCipher = []byte(h.Notes)
		}

		_, err := stmt.ExecContext(ctx,
			h.ID, userID, h.AssetSymbol, h.Quantity, h.AvgBuyPrice,
			h.QuoteCurrency, source, parsedRef, notesCipher,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteBySourceRef removes all holdings for a user with the given source_ref.
// Used for cascade deletion when credentials or wallets are removed.
func (r *HoldingRepository) DeleteBySourceRef(ctx context.Context, userID uuid.UUID, sourceRef string) error {
	parsedRef, err := uuid.Parse(sourceRef)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, `
		DELETE FROM portfolio_holdings
		WHERE user_id = $1 AND source_ref = $2
	`, userID, parsedRef)
	return err
}

// DeleteBySource removes all holdings for a user with the given source name.
// Used for clearing all holdings from a particular integration (e.g. "ledger").
func (r *HoldingRepository) DeleteBySource(ctx context.Context, userID uuid.UUID, source string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM portfolio_holdings
		WHERE user_id = $1 AND source = $2
	`, userID, source)
	return err
}

// ListDistinctUserIDs returns the unique user IDs that have at least one holding.
func (r *HoldingRepository) ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT user_id FROM portfolio_holdings
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
