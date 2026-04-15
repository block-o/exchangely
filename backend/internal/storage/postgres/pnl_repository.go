package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/google/uuid"
)

// PnLRepository implements portfolio.PnLRepository using PostgreSQL.
type PnLRepository struct {
	db *sql.DB
}

// Compile-time check that PnLRepository satisfies the portfolio.PnLRepository interface.
var _ portfolio.PnLRepository = (*PnLRepository)(nil)

// NewPnLRepository returns a new PnLRepository backed by the given database.
func NewPnLRepository(db *sql.DB) *PnLRepository {
	return &PnLRepository{db: db}
}

// Upsert inserts or updates a P&L snapshot. The unique constraint on
// (user_id, reference_currency) ensures one snapshot per user per currency.
func (r *PnLRepository) Upsert(ctx context.Context, snapshot *portfolio.PnLSnapshot) error {
	assetsJSON, err := json.Marshal(snapshot.Assets)
	if err != nil {
		return fmt.Errorf("marshal assets: %w", err)
	}

	return r.db.QueryRowContext(ctx, `
		INSERT INTO pnl_snapshots (
			id, user_id, reference_currency, total_realized, total_unrealized,
			total_pnl, has_approximate, excluded_count, assets_json, computed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, reference_currency) DO UPDATE SET
			total_realized   = EXCLUDED.total_realized,
			total_unrealized = EXCLUDED.total_unrealized,
			total_pnl        = EXCLUDED.total_pnl,
			has_approximate  = EXCLUDED.has_approximate,
			excluded_count   = EXCLUDED.excluded_count,
			assets_json      = EXCLUDED.assets_json,
			computed_at      = EXCLUDED.computed_at
		RETURNING id
	`,
		snapshot.ID, snapshot.UserID, snapshot.ReferenceCurrency,
		snapshot.TotalRealized, snapshot.TotalUnrealized, snapshot.TotalPnL,
		snapshot.HasApproximate, snapshot.ExcludedCount, assetsJSON, snapshot.ComputedAt,
	).Scan(&snapshot.ID)
}

// FindByUser returns the latest P&L snapshot for the given user and reference
// currency, or nil if no snapshot exists.
func (r *PnLRepository) FindByUser(ctx context.Context, userID uuid.UUID, referenceCurrency string) (*portfolio.PnLSnapshot, error) {
	var s portfolio.PnLSnapshot
	var assetsJSON []byte

	err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, reference_currency, total_realized, total_unrealized,
			total_pnl, has_approximate, excluded_count, assets_json, computed_at
		FROM pnl_snapshots
		WHERE user_id = $1 AND reference_currency = $2
	`, userID, referenceCurrency).Scan(
		&s.ID, &s.UserID, &s.ReferenceCurrency, &s.TotalRealized, &s.TotalUnrealized,
		&s.TotalPnL, &s.HasApproximate, &s.ExcludedCount, &assetsJSON, &s.ComputedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(assetsJSON, &s.Assets); err != nil {
		return nil, fmt.Errorf("unmarshal assets: %w", err)
	}

	return &s, nil
}
