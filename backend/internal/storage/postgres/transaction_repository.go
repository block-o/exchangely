package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

// TransactionRepository implements portfolio.TransactionRepository using PostgreSQL.
type TransactionRepository struct {
	db *sql.DB
}

// Compile-time check that TransactionRepository satisfies the portfolio.TransactionRepository interface.
var _ portfolio.TransactionRepository = (*TransactionRepository)(nil)

// NewTransactionRepository returns a new TransactionRepository backed by the given database.
func NewTransactionRepository(db *sql.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

// Create inserts a new transaction record. CreatedAt and UpdatedAt are set
// server-side by database defaults and scanned back into the struct.
func (r *TransactionRepository) Create(ctx context.Context, tx *portfolio.Transaction) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO portfolio_transactions (
			id, user_id, asset_symbol, quantity, tx_type, tx_timestamp,
			source, source_ref, reference_value, reference_currency,
			resolution, manually_edited, notes, fee, fee_currency
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at, updated_at
	`,
		tx.ID, tx.UserID, tx.AssetSymbol, tx.Quantity, tx.Type, tx.Timestamp,
		tx.Source, tx.SourceRef, tx.ReferenceValue, tx.ReferenceCurrency,
		tx.Resolution, tx.ManuallyEdited, tx.Notes, tx.Fee, tx.FeeCurrency,
	).Scan(&tx.CreatedAt, &tx.UpdatedAt)
}

// Upsert inserts a transaction or updates it on conflict, but skips the update
// if the existing row has manually_edited = true.
func (r *TransactionRepository) Upsert(ctx context.Context, tx *portfolio.Transaction) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO portfolio_transactions (
			id, user_id, asset_symbol, quantity, tx_type, tx_timestamp,
			source, source_ref, reference_value, reference_currency,
			resolution, manually_edited, notes, fee, fee_currency
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (user_id, source, source_ref, asset_symbol, tx_timestamp)
		DO UPDATE SET
			quantity = EXCLUDED.quantity,
			tx_type = EXCLUDED.tx_type,
			reference_value = EXCLUDED.reference_value,
			reference_currency = EXCLUDED.reference_currency,
			resolution = EXCLUDED.resolution,
			notes = EXCLUDED.notes,
			fee = EXCLUDED.fee,
			fee_currency = EXCLUDED.fee_currency,
			updated_at = now()
		WHERE NOT portfolio_transactions.manually_edited
		RETURNING created_at, updated_at
	`,
		tx.ID, tx.UserID, tx.AssetSymbol, tx.Quantity, tx.Type, tx.Timestamp,
		tx.Source, tx.SourceRef, tx.ReferenceValue, tx.ReferenceCurrency,
		tx.Resolution, tx.ManuallyEdited, tx.Notes, tx.Fee, tx.FeeCurrency,
	).Scan(&tx.CreatedAt, &tx.UpdatedAt)
}

// Update modifies an existing transaction. Only the owning user's transaction
// is updated (user_id filter ensures data isolation).
func (r *TransactionRepository) Update(ctx context.Context, tx *portfolio.Transaction) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE portfolio_transactions
		SET reference_value = $3, reference_currency = $4, resolution = $5,
			manually_edited = $6, notes = $7, updated_at = $8
		WHERE id = $1 AND user_id = $2
	`,
		tx.ID, tx.UserID, tx.ReferenceValue, tx.ReferenceCurrency,
		tx.Resolution, tx.ManuallyEdited, tx.Notes, now,
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
	tx.UpdatedAt = now
	return nil
}

// FindByID returns the transaction with the given ID owned by the given user,
// or nil if not found.
func (r *TransactionRepository) FindByID(ctx context.Context, userID, txID uuid.UUID) (*portfolio.Transaction, error) {
	var t portfolio.Transaction
	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, asset_symbol, quantity, tx_type, tx_timestamp,
			source, source_ref, reference_value, reference_currency,
			resolution, manually_edited, notes, fee, fee_currency,
			created_at, updated_at
		FROM portfolio_transactions
		WHERE id = $1 AND user_id = $2
	`, txID, userID).Scan(
		&t.ID, &t.UserID, &t.AssetSymbol, &t.Quantity, &t.Type, &t.Timestamp,
		&t.Source, &t.SourceRef, &t.ReferenceValue, &t.ReferenceCurrency,
		&t.Resolution, &t.ManuallyEdited, &t.Notes, &t.Fee, &t.FeeCurrency,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByUser returns a paginated list of transactions for the given user,
// with optional filtering by asset, type, and date range. Returns the
// transactions and the total count matching the filters.
func (r *TransactionRepository) ListByUser(ctx context.Context, userID uuid.UUID, opts portfolio.ListOptions) ([]portfolio.Transaction, int, error) {
	query := `
		SELECT id, user_id, asset_symbol, quantity, tx_type, tx_timestamp,
			source, source_ref, reference_value, reference_currency,
			resolution, manually_edited, notes, fee, fee_currency,
			created_at, updated_at,
			COUNT(*) OVER() AS total_count
		FROM portfolio_transactions
		WHERE user_id = $1`

	args := []interface{}{userID}
	argPos := 2

	if opts.Asset != "" {
		query += ` AND asset_symbol = $` + strconv.Itoa(argPos)
		args = append(args, opts.Asset)
		argPos++
	}

	if opts.Type != "" {
		query += ` AND tx_type = $` + strconv.Itoa(argPos)
		args = append(args, opts.Type)
		argPos++
	}

	if opts.StartDate != nil {
		query += ` AND tx_timestamp >= $` + strconv.Itoa(argPos)
		args = append(args, *opts.StartDate)
		argPos++
	}

	if opts.EndDate != nil {
		query += ` AND tx_timestamp <= $` + strconv.Itoa(argPos)
		args = append(args, *opts.EndDate)
		argPos++
	}

	query += ` ORDER BY tx_timestamp DESC`

	// Pagination defaults
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	query += ` LIMIT $` + strconv.Itoa(argPos) + ` OFFSET $` + strconv.Itoa(argPos+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var transactions []portfolio.Transaction
	var totalCount int

	for rows.Next() {
		var t portfolio.Transaction
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.AssetSymbol, &t.Quantity, &t.Type, &t.Timestamp,
			&t.Source, &t.SourceRef, &t.ReferenceValue, &t.ReferenceCurrency,
			&t.Resolution, &t.ManuallyEdited, &t.Notes, &t.Fee, &t.FeeCurrency,
			&t.CreatedAt, &t.UpdatedAt,
			&totalCount,
		); err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, t)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if len(transactions) == 0 {
		totalCount = 0
	}

	return transactions, totalCount, nil
}

// DeleteBySourceRef removes all transactions for a user matching the given
// source and source reference. Used for cascade deletion when credentials
// or wallets are removed.
func (r *TransactionRepository) DeleteBySourceRef(ctx context.Context, userID uuid.UUID, source, sourceRef string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM portfolio_transactions
		WHERE user_id = $1 AND source = $2 AND source_ref = $3
	`, userID, source, sourceRef)
	return err
}

// CountByUser returns the total number of transactions for the given user.
func (r *TransactionRepository) CountByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM portfolio_transactions WHERE user_id = $1
	`, userID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// DistinctCurrencies returns the unique reference currencies used across a user's transactions.
func (r *TransactionRepository) DistinctCurrencies(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT reference_currency FROM portfolio_transactions WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var currencies []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		currencies = append(currencies, c)
	}
	return currencies, rows.Err()
}
