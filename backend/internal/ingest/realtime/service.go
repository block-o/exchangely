// Package realtime contains the live market-event ingestion path that rebuilds
// canonical hourly candles from Kafka-delivered raw samples.
package realtime

import (
	"context"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/consolidate"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

// MarketStore abstracts the candle persistence layer used during realtime Kafka ingestion.
type MarketStore interface {
	UpsertRawCandles(ctx context.Context, interval string, candles []candle.Candle) error
	RawCandles(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
	UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error
}

// MarketNotifier is satisfied by MarketService. It pushes a signal to the SSE EventBus
// so that connected browser clients are notified immediately when new data is persisted.
type MarketNotifier interface {
	NotifyUpdate(pairSymbol string)
}

// IngestService consumes market events and rolls them into the latest hourly candle state.
type IngestService struct {
	store    MarketStore
	notifier MarketNotifier
}

// NewIngestService returns the market-event consumer service for realtime hourly consolidation.
func NewIngestService(store MarketStore, notifier MarketNotifier) *IngestService {
	return &IngestService{store: store, notifier: notifier}
}

// IngestRealtimeCandles persists raw market events, rebuilds the affected hour, and upserts the result.
// The consumer batches events by pair/hour before calling this method, so the first candle can be
// used to derive the consolidation window for the full batch.
func (s *IngestService) IngestRealtimeCandles(ctx context.Context, candles []candle.Candle) error {
	if len(candles) == 0 {
		return nil
	}

	first := candles[0]
	startedAt := time.Now()
	slog.Info("realtime ingest started",
		"pair", first.Pair,
		"interval", "1h",
		"candle_count", len(candles),
	)

	if err := s.store.UpsertRawCandles(ctx, "1h", candles); err != nil {
		slog.Warn("realtime ingest failed",
			"pair", first.Pair,
			"interval", "1h",
			"step", "upsert_raw",
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return err
	}

	windowStart := time.Unix(first.Timestamp, 0).UTC().Truncate(time.Hour)
	windowEnd := windowStart.Add(time.Hour)

	rawCandles, err := s.store.RawCandles(ctx, first.Pair, "1h", windowStart, windowEnd)
	if err != nil {
		slog.Warn("realtime ingest failed",
			"pair", first.Pair,
			"interval", "1h",
			"step", "load_raw",
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return err
	}

	consolidated, err := consolidate.FromRaw("1h", rawCandles)
	if err != nil {
		slog.Warn("realtime ingest failed",
			"pair", first.Pair,
			"interval", "1h",
			"step", "consolidate",
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return err
	}
	if len(consolidated) == 0 {
		slog.Info("realtime ingest completed",
			"pair", first.Pair,
			"interval", "1h",
			"raw_count", len(rawCandles),
			"consolidated_count", 0,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"status", "no_output",
		)
		return nil
	}

	if err := s.store.UpsertCandles(ctx, "1h", consolidated); err != nil {
		slog.Warn("realtime ingest failed",
			"pair", first.Pair,
			"interval", "1h",
			"step", "upsert_consolidated",
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return err
	}

	slog.Info("realtime ingest completed",
		"pair", first.Pair,
		"interval", "1h",
		"raw_count", len(rawCandles),
		"consolidated_count", len(consolidated),
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"status", "ok",
	)

	// Signal connected SSE clients that new data is available.
	// This is the critical bridge between the Kafka consumer pipeline and the frontend:
	// Kafka event -> IngestRealtimeCandles -> Postgres upsert -> NotifyUpdate -> SSE push.
	if s.notifier != nil {
		s.notifier.NotifyUpdate(first.Pair)
	}

	return nil
}
