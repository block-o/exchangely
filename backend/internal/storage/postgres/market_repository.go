package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
)

type MarketRepository struct {
	db *sql.DB
}

func NewMarketRepository(db *sql.DB) *MarketRepository {
	return &MarketRepository{db: db}
}

func (r *MarketRepository) UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error {
	table, err := candleTable(interval)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	query := fmt.Sprintf(`
		INSERT INTO %s (pair_symbol, bucket_start, open, high, low, close, volume, source, finalized)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (pair_symbol, bucket_start) DO UPDATE
		SET open = EXCLUDED.open,
		    high = EXCLUDED.high,
		    low = EXCLUDED.low,
		    close = EXCLUDED.close,
		    volume = EXCLUDED.volume,
		    source = EXCLUDED.source,
		    finalized = EXCLUDED.finalized
	`, table)

	for _, item := range candles {
		if _, err := tx.ExecContext(ctx, query,
			item.Pair,
			time.Unix(item.Timestamp, 0).UTC(),
			item.Open,
			item.High,
			item.Low,
			item.Close,
			item.Volume,
			item.Source,
			item.Finalized,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *MarketRepository) UpsertRawCandles(ctx context.Context, interval string, candles []candle.Candle) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, item := range candles {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO raw_candles (pair_symbol, interval, bucket_start, source, open, high, low, close, volume, finalized, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
			ON CONFLICT (pair_symbol, interval, bucket_start, source) DO UPDATE
			SET open = EXCLUDED.open,
			    high = EXCLUDED.high,
			    low = EXCLUDED.low,
			    close = EXCLUDED.close,
			    volume = EXCLUDED.volume,
			    finalized = EXCLUDED.finalized,
			    updated_at = NOW()
		`,
			item.Pair,
			interval,
			time.Unix(item.Timestamp, 0).UTC(),
			item.Source,
			item.Open,
			item.High,
			item.Low,
			item.Close,
			item.Volume,
			item.Finalized,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *MarketRepository) RawCandles(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol,
		       EXTRACT(EPOCH FROM bucket_start)::BIGINT,
		       open::DOUBLE PRECISION,
		       high::DOUBLE PRECISION,
		       low::DOUBLE PRECISION,
		       close::DOUBLE PRECISION,
		       volume::DOUBLE PRECISION,
		       source,
		       finalized
		FROM raw_candles
		WHERE pair_symbol = $1
		  AND interval = $2
		  AND bucket_start >= $3
		  AND bucket_start < $4
		ORDER BY bucket_start, source
	`, pairSymbol, interval, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []candle.Candle
	for rows.Next() {
		var item candle.Candle
		item.Interval = interval
		if err := rows.Scan(
			&item.Pair,
			&item.Timestamp,
			&item.Open,
			&item.High,
			&item.Low,
			&item.Close,
			&item.Volume,
			&item.Source,
			&item.Finalized,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *MarketRepository) HourlyCandles(ctx context.Context, pairSymbol string, start, end time.Time) ([]candle.Candle, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pair_symbol,
		       EXTRACT(EPOCH FROM bucket_start)::BIGINT,
		       open::DOUBLE PRECISION,
		       high::DOUBLE PRECISION,
		       low::DOUBLE PRECISION,
		       close::DOUBLE PRECISION,
		       volume::DOUBLE PRECISION,
		       source,
		       finalized
		FROM candles_1h
		WHERE pair_symbol = $1
		  AND bucket_start >= $2
		  AND bucket_start < $3
		ORDER BY bucket_start
	`, pairSymbol, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []candle.Candle
	for rows.Next() {
		var item candle.Candle
		item.Interval = "1h"
		if err := rows.Scan(
			&item.Pair,
			&item.Timestamp,
			&item.Open,
			&item.High,
			&item.Low,
			&item.Close,
			&item.Volume,
			&item.Source,
			&item.Finalized,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *MarketRepository) Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error) {
	table, err := candleTable(interval)
	if err != nil {
		return nil, err
	}

	if end.IsZero() {
		end = time.Now().UTC()
	}

	query := fmt.Sprintf(`
		SELECT pair_symbol,
		       EXTRACT(EPOCH FROM bucket_start)::BIGINT,
		       open::DOUBLE PRECISION,
		       high::DOUBLE PRECISION,
		       low::DOUBLE PRECISION,
		       close::DOUBLE PRECISION,
		       volume::DOUBLE PRECISION,
		       source,
		       finalized
		FROM %s
		WHERE pair_symbol = $1
		  AND ($2::TIMESTAMPTZ IS NULL OR bucket_start >= $2)
		  AND bucket_start <= $3
		ORDER BY bucket_start
	`, table)

	startArg := any(nil)
	if !start.IsZero() {
		startArg = start.UTC()
	}

	rows, err := r.db.QueryContext(ctx, query, pairSymbol, startArg, end.UTC())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []candle.Candle
	for rows.Next() {
		var item candle.Candle
		item.Interval = interval
		if err := rows.Scan(
			&item.Pair,
			&item.Timestamp,
			&item.Open,
			&item.High,
			&item.Low,
			&item.Close,
			&item.Volume,
			&item.Source,
			&item.Finalized,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// Ticker returns the latest market snapshot for a single trading pair.
func (r *MarketRepository) Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error) {
	var latest ticker.Ticker
	var oldPrice, oldPrice1H, oldPrice7D float64
	var volume24H float64
	var circulatingSupply sql.NullFloat64
	var max24, min24 sql.NullFloat64
	err := r.db.QueryRowContext(ctx, tickerSnapshotQuery("WHERE l.pair_symbol = $1"), pairSymbol).Scan(
		&latest.Pair,
		&latest.Price,
		&latest.LastUpdateUnix,
		&latest.Source,
		&oldPrice,
		&oldPrice1H,
		&oldPrice7D,
		&max24,
		&min24,
		&volume24H,
		&circulatingSupply,
	)
	if err != nil {
		return ticker.Ticker{}, err
	}

	if oldPrice != 0 {
		latest.Variation24H = ((latest.Price - oldPrice) / oldPrice) * 100
	}
	if oldPrice1H != 0 {
		latest.Variation1H = ((latest.Price - oldPrice1H) / oldPrice1H) * 100
	}
	if oldPrice7D != 0 {
		latest.Variation7D = ((latest.Price - oldPrice7D) / oldPrice7D) * 100
	}
	latest.MarketCap = latest.Price * circulatingSupply.Float64
	latest.High24H = max24.Float64
	latest.Low24H = min24.Float64
	latest.Volume24H = volume24H
	return latest, nil
}

// Tickers returns the latest market snapshots for all traded pairs, including
// 1h, 24h, and 7d price variations, and 24h aggregated volume.
//
// The query prefers the latest raw realtime point over hourly candles for real-time accuracy,
// then derives baselines by looking back at the closest hourly candles at T-1h, T-24h, and T-7d.
func (r *MarketRepository) Tickers(ctx context.Context) ([]ticker.Ticker, error) {
	rows, err := r.db.QueryContext(ctx, tickerSnapshotQuery(""))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []ticker.Ticker
	for rows.Next() {
		var t ticker.Ticker
		var oldPrice, oldPrice1H, oldPrice7D float64
		var volume24H float64
		var circulatingSupply sql.NullFloat64
		var max24, min24 sql.NullFloat64 // nullable because pairs with <24h of data won't have a window_24h row
		if err := rows.Scan(
			&t.Pair,
			&t.Price,
			&t.LastUpdateUnix,
			&t.Source,
			&oldPrice,
			&oldPrice1H,
			&oldPrice7D,
			&max24,
			&min24,
			&volume24H,
			&circulatingSupply,
		); err != nil {
			return nil, err
		}
		// Compute percentage changes
		if oldPrice != 0 {
			t.Variation24H = ((t.Price - oldPrice) / oldPrice) * 100
		}
		if oldPrice1H != 0 {
			t.Variation1H = ((t.Price - oldPrice1H) / oldPrice1H) * 100
		}
		if oldPrice7D != 0 {
			t.Variation7D = ((t.Price - oldPrice7D) / oldPrice7D) * 100
		}
		t.MarketCap = t.Price * circulatingSupply.Float64
		t.High24H = max24.Float64
		t.Low24H = min24.Float64
		t.Volume24H = volume24H
		items = append(items, t)
	}

	return items, rows.Err()
}

// tickerSnapshotQuery generates a complex SQL query to calculate real-time market stats.
//
// It uses Common Table Expressions (CTEs) for:
//   - latest:     The most recent price, preferring raw_candles over candles_1h if newer.
//   - past:       The closest hourly candle to 24h ago.
//   - past_1h:    The closest hourly candle to 1h ago.
//   - past_7d:    The closest hourly candle to 7d ago.
//   - window_24h: Aggregated high, low, and sum of volume over the last 24h.
func tickerSnapshotQuery(filter string) string {
	return fmt.Sprintf(`
		WITH latest_hourly AS (
			SELECT DISTINCT ON (pair_symbol)
			       pair_symbol,
			       close::DOUBLE PRECISION as hourly_price,
			       EXTRACT(EPOCH FROM bucket_start)::BIGINT as hourly_unix,
			       source as hourly_source
			FROM candles_1h
			ORDER BY pair_symbol, bucket_start DESC
		),
		latest_raw AS (
			SELECT DISTINCT ON (pair_symbol)
			       pair_symbol,
			       close::DOUBLE PRECISION as raw_price,
			       EXTRACT(EPOCH FROM bucket_start)::BIGINT as raw_unix,
			       source as raw_source
			FROM raw_candles
			WHERE interval = '1h'
			ORDER BY pair_symbol, bucket_start DESC, updated_at DESC
		),
		latest AS (
			SELECT h.pair_symbol,
			       CASE
			           WHEN r.raw_unix IS NOT NULL AND r.raw_unix > h.hourly_unix THEN r.raw_price
			           ELSE h.hourly_price
			       END as price,
			       CASE
			           WHEN r.raw_unix IS NOT NULL AND r.raw_unix > h.hourly_unix THEN r.raw_unix
			           ELSE h.hourly_unix
			       END as last_unix,
			       CASE
			           WHEN r.raw_unix IS NOT NULL AND r.raw_unix > h.hourly_unix THEN r.raw_source
			           ELSE h.hourly_source
			       END as source
			FROM latest_hourly h
			LEFT JOIN latest_raw r ON r.pair_symbol = h.pair_symbol
		),
		past AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol,
			       c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '24 hours'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		past_1h AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol,
			       c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '1 hour'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		past_7d AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol,
			       c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '7 days'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		window_24h AS (
			SELECT c.pair_symbol,
			       MAX(c.high) as max_24h,
			       MIN(c.low) as min_24h,
			       SUM(c.volume) as volume_24h
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start > to_timestamp(l.last_unix) - INTERVAL '24 hours'
			GROUP BY c.pair_symbol
		)
		SELECT l.pair_symbol,
		       l.price,
		       l.last_unix,
		       l.source,
		       COALESCE(p.old_price, l.price) as old_price_24h,
		       COALESCE(p1h.old_price, l.price) as old_price_1h,
		       COALESCE(p7d.old_price, l.price) as old_price_7d,
		       w.max_24h,
		       w.min_24h,
		       COALESCE(w.volume_24h, 0) as volume_24h,
		       COALESCE(asset.circulating_supply, 0)::DOUBLE PRECISION
		FROM latest l
		LEFT JOIN past p ON l.pair_symbol = p.pair_symbol
		LEFT JOIN past_1h p1h ON l.pair_symbol = p1h.pair_symbol
		LEFT JOIN past_7d p7d ON l.pair_symbol = p7d.pair_symbol
		LEFT JOIN window_24h w ON l.pair_symbol = w.pair_symbol
		LEFT JOIN pairs pair ON pair.symbol = l.pair_symbol
		LEFT JOIN assets asset ON asset.symbol = pair.base_asset
		%s
		ORDER BY l.pair_symbol
	`, filter)
}

func candleTable(interval string) (string, error) {
	switch interval {
	case "1h":
		return "candles_1h", nil
	case "1d":
		return "candles_1d", nil
	default:
		return "", fmt.Errorf("unsupported interval %q", interval)
	}
}
