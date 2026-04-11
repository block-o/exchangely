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
			INSERT INTO raw_candles (pair_symbol, interval, bucket_start, source, open, high, low, close, volume, v24h, finalized, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
			ON CONFLICT (pair_symbol, interval, bucket_start, source) DO UPDATE
			SET open = EXCLUDED.open,
			    high = EXCLUDED.high,
			    low = EXCLUDED.low,
			    close = EXCLUDED.close,
			    volume = EXCLUDED.volume,
			    v24h = EXCLUDED.v24h,
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
			item.Volume24H,
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
		       v24h::DOUBLE PRECISION,
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
			&item.Volume24H,
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

	// For 1h queries we merge consolidated candles with raw_candles so that
	// recently-ingested realtime data is visible even before the consolidation
	// pipeline has run. raw_candles rows are only used for hour buckets that
	// have no consolidated entry yet.
	if interval == "1h" {
		return r.historicalHourlyMerged(ctx, pairSymbol, start, end)
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

// historicalHourlyMerged returns 1h candles by unioning candles_1h with
// raw_candles. For each hour bucket the consolidated row wins; raw_candles
// fills gaps where consolidation hasn't run yet.
func (r *MarketRepository) historicalHourlyMerged(ctx context.Context, pairSymbol string, start, end time.Time) ([]candle.Candle, error) {
	query := `
		WITH consolidated AS (
			SELECT pair_symbol,
			       bucket_start,
			       open::DOUBLE PRECISION,
			       high::DOUBLE PRECISION,
			       low::DOUBLE PRECISION,
			       close::DOUBLE PRECISION,
			       volume::DOUBLE PRECISION,
			       source,
			       finalized
			FROM candles_1h
			WHERE pair_symbol = $1
			  AND ($2::TIMESTAMPTZ IS NULL OR bucket_start >= $2)
			  AND bucket_start <= $3
		),
		raw_hourly AS (
			SELECT DISTINCT ON (date_trunc('hour', bucket_start))
			       pair_symbol,
			       date_trunc('hour', bucket_start) AS bucket_start,
			       open::DOUBLE PRECISION,
			       high::DOUBLE PRECISION,
			       low::DOUBLE PRECISION,
			       close::DOUBLE PRECISION,
			       volume::DOUBLE PRECISION,
			       source,
			       finalized
			FROM raw_candles
			WHERE pair_symbol = $1
			  AND interval = '1h'
			  AND ($2::TIMESTAMPTZ IS NULL OR bucket_start >= $2)
			  AND bucket_start <= $3
			ORDER BY date_trunc('hour', bucket_start), updated_at DESC
		)
		SELECT pair_symbol,
		       EXTRACT(EPOCH FROM bucket_start)::BIGINT,
		       open, high, low, close, volume, source, finalized
		FROM consolidated
		UNION ALL
		SELECT r.pair_symbol,
		       EXTRACT(EPOCH FROM r.bucket_start)::BIGINT,
		       r.open, r.high, r.low, r.close, r.volume, r.source, r.finalized
		FROM raw_hourly r
		LEFT JOIN consolidated c ON c.bucket_start = r.bucket_start
		WHERE c.bucket_start IS NULL
		ORDER BY 2
	`

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

// TickersWithSparklines returns the latest ticker snapshot for every pair together
// with the last 24 hourly candle points (merged from candles_1h + raw_candles).
// This allows the frontend to render sparklines without issuing per-pair historical requests.
func (r *MarketRepository) TickersWithSparklines(ctx context.Context) ([]ticker.TickerWithSparkline, error) {
	rows, err := r.db.QueryContext(ctx, tickerSnapshotQuery(""))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []ticker.TickerWithSparkline
	for rows.Next() {
		var tw ticker.TickerWithSparkline
		var oldPrice, oldPrice1H, oldPrice7D float64
		var volume24H float64
		var circulatingSupply sql.NullFloat64
		var max24, min24 sql.NullFloat64
		if err := rows.Scan(
			&tw.Pair,
			&tw.Price,
			&tw.LastUpdateUnix,
			&tw.Source,
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
		if oldPrice != 0 {
			tw.Variation24H = ((tw.Price - oldPrice) / oldPrice) * 100
		}
		if oldPrice1H != 0 {
			tw.Variation1H = ((tw.Price - oldPrice1H) / oldPrice1H) * 100
		}
		if oldPrice7D != 0 {
			tw.Variation7D = ((tw.Price - oldPrice7D) / oldPrice7D) * 100
		}
		tw.MarketCap = tw.Price * circulatingSupply.Float64
		tw.High24H = max24.Float64
		tw.Low24H = min24.Float64
		tw.Volume24H = volume24H
		items = append(items, tw)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batch-fetch 24h sparkline candles for all pairs in a single query.
	sparklines, err := r.sparklines24h(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if pts, ok := sparklines[items[i].Pair]; ok {
			items[i].Sparkline = pts
		}
	}

	return items, nil
}

// sparklines24h returns the last 24 hourly candle points for every pair,
// merging consolidated candles_1h with recent raw_candles (same strategy as
// historicalHourlyMerged but for all pairs at once).
func (r *MarketRepository) sparklines24h(ctx context.Context) (map[string][]ticker.SparklinePoint, error) {
	query := `
		WITH consolidated AS (
			SELECT pair_symbol,
			       bucket_start,
			       open::DOUBLE PRECISION,
			       high::DOUBLE PRECISION,
			       low::DOUBLE PRECISION,
			       close::DOUBLE PRECISION,
			       volume::DOUBLE PRECISION
			FROM candles_1h
			WHERE bucket_start >= NOW() - INTERVAL '25 hours'
		),
		raw_hourly AS (
			SELECT DISTINCT ON (pair_symbol, date_trunc('hour', bucket_start))
			       pair_symbol,
			       date_trunc('hour', bucket_start) AS bucket_start,
			       open::DOUBLE PRECISION,
			       high::DOUBLE PRECISION,
			       low::DOUBLE PRECISION,
			       close::DOUBLE PRECISION,
			       volume::DOUBLE PRECISION
			FROM raw_candles
			WHERE interval = '1h'
			  AND bucket_start >= NOW() - INTERVAL '25 hours'
			ORDER BY pair_symbol, date_trunc('hour', bucket_start), updated_at DESC
		),
		merged AS (
			SELECT pair_symbol,
			       EXTRACT(EPOCH FROM bucket_start)::BIGINT AS ts,
			       open, high, low, close, volume
			FROM consolidated
			UNION ALL
			SELECT r.pair_symbol,
			       EXTRACT(EPOCH FROM r.bucket_start)::BIGINT AS ts,
			       r.open, r.high, r.low, r.close, r.volume
			FROM raw_hourly r
			LEFT JOIN consolidated c ON c.pair_symbol = r.pair_symbol AND c.bucket_start = r.bucket_start
			WHERE c.bucket_start IS NULL
		)
		SELECT pair_symbol, ts, open, high, low, close, volume
		FROM merged
		ORDER BY pair_symbol, ts
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make(map[string][]ticker.SparklinePoint)
	for rows.Next() {
		var pair string
		var pt ticker.SparklinePoint
		if err := rows.Scan(&pair, &pt.Timestamp, &pt.Open, &pt.High, &pt.Low, &pt.Close, &pt.Volume); err != nil {
			return nil, err
		}
		result[pair] = append(result[pair], pt)
	}

	// Keep only the last 24 points per pair.
	for pair, pts := range result {
		if len(pts) > 24 {
			result[pair] = pts[len(pts)-24:]
		}
	}

	return result, rows.Err()
}

// tickerSnapshotQuery generates a complex SQL query to calculate real-time market stats.
//
// It uses Common Table Expressions (CTEs) for:
//   - latest:     The most recent price, preferring raw_candles over candles_1h if newer.
//   - past:       The closest hourly candle to 24h ago.
//   - past_1h:    The closest hourly candle to 1h ago.
//   - past_7d:    The closest hourly candle to 7d ago.
//   - window_24h: Aggregated high, low, and sum of volume over the last 24h.
//
// Performance notes:
//   - latest_raw and native_v24h use partial indexes on raw_candles WHERE interval='1h'.
//   - All CTEs use time-bounded scans (lower bound) so the planner can skip chunks
//     older than the lookback window instead of scanning the full hypertable.
//     latest_hourly and latest_raw use a 30-day window since ticker data is always
//     recent; pairs with no data in the last 30 days are intentionally excluded.
//   - past_1h avoids a UNION ALL between candles_1h and raw_candles; it queries only
//     candles_1h (the consolidated source) which is sufficient for a 1h lookback.
func tickerSnapshotQuery(filter string) string {
	return fmt.Sprintf(`
		WITH latest_hourly AS (
			SELECT DISTINCT ON (pair_symbol)
			       pair_symbol,
			       close::DOUBLE PRECISION as hourly_price,
			       bucket_start as hourly_bucket,
			       EXTRACT(EPOCH FROM bucket_start)::BIGINT as hourly_unix,
			       source as hourly_source
			FROM candles_1h
			WHERE bucket_start >= NOW() - INTERVAL '30 days'
			ORDER BY pair_symbol, bucket_start DESC
		),
		latest_raw AS (
			SELECT DISTINCT ON (pair_symbol)
			       pair_symbol,
			       close::DOUBLE PRECISION as raw_price,
			       bucket_start as raw_bucket,
			       EXTRACT(EPOCH FROM updated_at)::BIGINT as raw_unix,
			       source as raw_source
			FROM raw_candles
			WHERE interval = '1h'
			  AND bucket_start >= NOW() - INTERVAL '30 days'
			ORDER BY pair_symbol, bucket_start DESC, updated_at DESC
		),
		latest AS (
			SELECT COALESCE(h.pair_symbol, r.pair_symbol) as pair_symbol,
			       CASE
			           WHEN r.raw_bucket IS NOT NULL AND (h.hourly_bucket IS NULL OR r.raw_bucket > h.hourly_bucket) THEN r.raw_price
			           ELSE h.hourly_price
			       END as price,
			       CASE
			           WHEN r.raw_bucket IS NOT NULL AND (h.hourly_bucket IS NULL OR r.raw_bucket > h.hourly_bucket) THEN r.raw_unix
			           ELSE h.hourly_unix
			       END as last_unix,
			       CASE
			           WHEN r.raw_bucket IS NOT NULL AND (h.hourly_bucket IS NULL OR r.raw_bucket > h.hourly_bucket) THEN r.raw_source
			           ELSE h.hourly_source
			       END as source
			FROM latest_hourly h
			FULL OUTER JOIN latest_raw r ON r.pair_symbol = h.pair_symbol
		),
		native_v24h AS (
			SELECT DISTINCT ON (pair_symbol, source)
			       pair_symbol,
			       v24h::DOUBLE PRECISION as source_v24h
			FROM raw_candles
			WHERE interval = '1h'
			  AND bucket_start >= NOW() - INTERVAL '7 days'
			ORDER BY pair_symbol, source, bucket_start DESC, updated_at DESC
		),
		consolidated_native_v24h AS (
			SELECT pair_symbol,
			       SUM(source_v24h) as total_native_v24h
			FROM native_v24h
			GROUP BY pair_symbol
		),
		past AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol,
			       c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start >= NOW() - INTERVAL '25 hours'
			  AND c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '24 hours'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		past_1h AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol,
			       c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start >= NOW() - INTERVAL '2 hours'
			  AND c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '1 hour'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		past_7d AS (
			SELECT DISTINCT ON (c.pair_symbol)
			       c.pair_symbol,
			       c.close::DOUBLE PRECISION as old_price
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start >= NOW() - INTERVAL '8 days'
			  AND c.bucket_start <= to_timestamp(l.last_unix) - INTERVAL '7 days'
			ORDER BY c.pair_symbol, c.bucket_start DESC
		),
		window_24h AS (
			SELECT c.pair_symbol,
			       MAX(c.high) as max_24h,
			       MIN(c.low) as min_24h,
			       SUM(c.volume * c.close) as bucket_sum_v24h
			FROM candles_1h c
			JOIN latest l ON c.pair_symbol = l.pair_symbol
			WHERE c.bucket_start >= NOW() - INTERVAL '25 hours'
			  AND c.bucket_start > to_timestamp(l.last_unix) - INTERVAL '24 hours'
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
		       COALESCE(NULLIF(nv.total_native_v24h, 0), w.bucket_sum_v24h, 0) as volume_24h,
		       COALESCE(asset.circulating_supply, 0)::DOUBLE PRECISION
		FROM latest l
		LEFT JOIN past p ON l.pair_symbol = p.pair_symbol
		LEFT JOIN past_1h p1h ON l.pair_symbol = p1h.pair_symbol
		LEFT JOIN past_7d p7d ON l.pair_symbol = p7d.pair_symbol
		LEFT JOIN window_24h w ON l.pair_symbol = w.pair_symbol
		LEFT JOIN consolidated_native_v24h nv ON l.pair_symbol = nv.pair_symbol
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
