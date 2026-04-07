package postgres

import (
	"context"
	"database/sql"

	"github.com/block-o/exchangely/backend/internal/domain/asset"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

type CatalogRepository struct {
	db *sql.DB
}

func NewCatalogRepository(db *sql.DB) *CatalogRepository {
	return &CatalogRepository{db: db}
}

func (r *CatalogRepository) ReplaceCatalog(ctx context.Context, assets []asset.Asset, pairs []pair.Pair) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, item := range assets {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO assets (symbol, name, asset_type, circulating_supply)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (symbol) DO UPDATE
			SET name = EXCLUDED.name,
			    asset_type = EXCLUDED.asset_type,
			    circulating_supply = EXCLUDED.circulating_supply
		`, item.Symbol, item.Name, item.Type, item.CirculatingSupply); err != nil {
			return err
		}
	}

	for _, item := range pairs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pairs (symbol, base_asset, quote_asset, backfill_start_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (symbol) DO UPDATE
			SET base_asset = EXCLUDED.base_asset,
			    quote_asset = EXCLUDED.quote_asset,
			    backfill_start_at = EXCLUDED.backfill_start_at
		`, item.Symbol, item.Base, item.Quote, item.BackfillStart.UTC()); err != nil {
			return err
		}
	}

	currentPairs, err := existingPairSymbols(ctx, tx)
	if err != nil {
		return err
	}
	desiredPairs := make(map[string]struct{}, len(pairs))
	for _, item := range pairs {
		desiredPairs[item.Symbol] = struct{}{}
	}
	for _, symbol := range currentPairs {
		if _, ok := desiredPairs[symbol]; ok {
			continue
		}
		if err := deletePairData(ctx, tx, symbol); err != nil {
			return err
		}
	}

	currentAssets, err := existingAssetSymbols(ctx, tx)
	if err != nil {
		return err
	}
	desiredAssets := make(map[string]struct{}, len(assets))
	for _, item := range assets {
		desiredAssets[item.Symbol] = struct{}{}
	}
	for _, symbol := range currentAssets {
		if _, ok := desiredAssets[symbol]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE symbol = $1`, symbol); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func existingPairSymbols(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT symbol FROM pairs`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, err
		}
		items = append(items, symbol)
	}

	return items, rows.Err()
}

func existingAssetSymbols(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT symbol FROM assets`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, err
		}
		items = append(items, symbol)
	}

	return items, rows.Err()
}

func deletePairData(ctx context.Context, tx *sql.Tx, pairSymbol string) error {
	statements := []string{
		`DELETE FROM tasks WHERE pair_symbol = $1`,
		`DELETE FROM sync_status WHERE pair_symbol = $1`,
		`DELETE FROM raw_candles WHERE pair_symbol = $1`,
		`DELETE FROM candles_1h WHERE pair_symbol = $1`,
		`DELETE FROM candles_1d WHERE pair_symbol = $1`,
		`DELETE FROM pairs WHERE symbol = $1`,
	}

	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement, pairSymbol); err != nil {
			return err
		}
	}

	return nil
}

func (r *CatalogRepository) ListAssets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT symbol, name, asset_type, circulating_supply
		FROM assets
		ORDER BY asset_type, symbol
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []asset.Asset
	for rows.Next() {
		var item asset.Asset
		if err := rows.Scan(&item.Symbol, &item.Name, &item.Type, &item.CirculatingSupply); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *CatalogRepository) ListPairs(ctx context.Context) ([]pair.Pair, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT symbol, base_asset, quote_asset, backfill_start_at
		FROM pairs
		ORDER BY symbol
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []pair.Pair
	for rows.Next() {
		var item pair.Pair
		if err := rows.Scan(&item.Symbol, &item.Base, &item.Quote, &item.BackfillStart); err != nil {
			return nil, err
		}
		item.BackfillStart = item.BackfillStart.UTC()
		items = append(items, item)
	}

	return items, rows.Err()
}
