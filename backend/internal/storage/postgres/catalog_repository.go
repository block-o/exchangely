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

func (r *CatalogRepository) UpsertAssets(ctx context.Context, assets []asset.Asset) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

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

	return tx.Commit()
}

func (r *CatalogRepository) UpsertPairs(ctx context.Context, pairs []pair.Pair) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range pairs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pairs (symbol, base_asset, quote_asset)
			VALUES ($1, $2, $3)
			ON CONFLICT (symbol) DO UPDATE
			SET base_asset = EXCLUDED.base_asset,
			    quote_asset = EXCLUDED.quote_asset
		`, item.Symbol, item.Base, item.Quote); err != nil {
			return err
		}
	}

	return tx.Commit()
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
	defer rows.Close()

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
		SELECT symbol, base_asset, quote_asset
		FROM pairs
		ORDER BY symbol
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []pair.Pair
	for rows.Next() {
		var item pair.Pair
		if err := rows.Scan(&item.Symbol, &item.Base, &item.Quote); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
