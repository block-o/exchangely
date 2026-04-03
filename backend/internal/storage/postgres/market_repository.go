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
	defer tx.Rollback()

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
	defer tx.Rollback()

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
	defer rows.Close()

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
	defer rows.Close()

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
	defer rows.Close()

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

func (r *MarketRepository) Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error) {
	var latest ticker.Ticker
	err := r.db.QueryRowContext(ctx, `
		SELECT pair_symbol,
		       close::DOUBLE PRECISION,
		       EXTRACT(EPOCH FROM bucket_start)::BIGINT,
		       source
		FROM candles_1h
		WHERE pair_symbol = $1
		ORDER BY bucket_start DESC
		LIMIT 1
	`, pairSymbol).Scan(&latest.Pair, &latest.Price, &latest.LastUpdateUnix, &latest.Source)
	if err != nil {
		return ticker.Ticker{}, err
	}

	var previousPrice float64
	err = r.db.QueryRowContext(ctx, `
		SELECT close::DOUBLE PRECISION
		FROM candles_1h
		WHERE pair_symbol = $1
		  AND bucket_start <= to_timestamp($2) - INTERVAL '24 hours'
		ORDER BY bucket_start DESC
		LIMIT 1
	`, pairSymbol, latest.LastUpdateUnix).Scan(&previousPrice)
	if err == nil && previousPrice != 0 {
		latest.Variation24H = ((latest.Price - previousPrice) / previousPrice) * 100
	}

	return latest, nil
}

// Tickers returns the latest price, 24h variation, 24h high, and 24h low for every traded pair.
// The query uses three CTEs to compute all analytics in a single round-trip to Postgres:
//
//   - "latest":     DISTINCT ON (pair_symbol) picks the most recent hourly candle per pair.
//   - "past":       DISTINCT ON (pair_symbol) picks the candle closest to (but not after) 24h ago,
//     used as the baseline for percentage-change calculation.
//   - "window_24h": GROUP BY pair_symbol aggregates MAX(high) and MIN(low) over all candles
//     within the trailing 24-hour window, powering the 24h High/Low columns.
//
// The final SELECT LEFT JOINs all three CTEs so that pairs with fewer than 24h of data
// still appear (old_price falls back to the current price, high/low may be NULL).
func (r *MarketRepository) Tickers(ctx context.Context) ([]ticker.Ticker, error) {
	rows, err := r.db.QueryContext(ctx, `
		-- CTE 1: Pick the single most-recent hourly candle per pair.
		WITH latest AS (
			SELECT DISTINCT ON (pair_symbol)
			       pair_symbol, close::DOUBLE PRECISION as price, EXTRACT(EPOCH FROM bucket_start)::BIGINT as last_unix
			FROM candles_1h
			ORDER BY pair_symbol, bucket_start DESC
		),
		-- CTE 2: Find the closing price ~24h ago for percentage-change calculation.
		-- Uses the latest candle's timestamp as the anchor so the lookback is always relative.
		past AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol, c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '24 hours'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		-- CTE 3: Aggregate the highest high and lowest low within the trailing 24h window.
		window_24h AS (
			SELECT c.pair_symbol,
			       MAX(c.high) as max_24h,
			       MIN(c.low) as min_24h
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start > to_timestamp(l.last_unix) - INTERVAL '24 hours'
			GROUP BY c.pair_symbol
		)
		SELECT l.pair_symbol, l.price, l.last_unix, COALESCE(p.old_price, l.price) as old_price, w.max_24h, w.min_24h
		FROM latest l
		LEFT JOIN past p ON l.pair_symbol = p.pair_symbol
		LEFT JOIN window_24h w ON l.pair_symbol = w.pair_symbol;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ticker.Ticker
	for rows.Next() {
		var t ticker.Ticker
		var oldPrice float64
		var max24, min24 sql.NullFloat64 // nullable because pairs with <24h of data won't have a window_24h row
		if err := rows.Scan(&t.Pair, &t.Price, &t.LastUpdateUnix, &oldPrice, &max24, &min24); err != nil {
			return nil, err
		}
		// Compute 24h percentage change: ((current - old) / old) * 100.
		// If there is no 24h-old candle, old_price equals current price (via COALESCE), so variation is 0.
		if oldPrice != 0 {
			t.Variation24H = ((t.Price - oldPrice) / oldPrice) * 100
		}
		t.High24H = max24.Float64
		t.Low24H = min24.Float64
		items = append(items, t)
	}

	return items, rows.Err()
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
