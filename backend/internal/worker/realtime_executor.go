package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/consolidate"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

// MarketEventPublisher sends candle data to Kafka for downstream consumption (realtime flow).
type MarketEventPublisher interface {
	PublishCandles(ctx context.Context, candles []candle.Candle) error
}

// RealtimeExecutor handles live_ticker tasks by fetching the latest market
// snapshots and forwarding them to Kafka. When no publisher is configured it
// falls back to the raw→hourly DB materialisation path.
type RealtimeExecutor struct {
	candles  CandleStore
	sync     SyncProgressWriter
	sources  MarketSource
	events   MarketEventPublisher
	notifier MarketNotifier
}

// NewRealtimeExecutor returns the worker-side executor for live_ticker tasks.
func NewRealtimeExecutor(candles CandleStore, sync SyncProgressWriter, sources MarketSource, events MarketEventPublisher, notifier MarketNotifier) *RealtimeExecutor {
	return &RealtimeExecutor{
		candles:  candles,
		sync:     sync,
		sources:  sources,
		events:   events,
		notifier: notifier,
	}
}

// Execute publishes realtime candles to Kafka (preferred) or falls back to
// direct DB persistence, then records the realtime cutover timestamp.
func (e *RealtimeExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeRealtime {
		return fmt.Errorf("realtime executor received non-realtime task %q", item.Type)
	}

	startedAt := time.Now()
	slog.DebugContext(ctx, "realtime task execution active",
		"task_id", item.ID,
		"type", item.Type,
		"pair", item.Pair,
		"interval", item.Interval,
	)

	candles, err := e.publishRealtime(ctx, item)
	if err != nil {
		slog.Warn("realtime task execution failed",
			"task_id", item.ID,
			"pair", item.Pair,
			"interval", item.Interval,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return err
	}

	// When the Kafka path is used, candles is nil — progress still needs recording.
	if len(candles) > 0 {
		if err := e.candles.UpsertCandles(ctx, "1h", candles); err != nil {
			return err
		}
		if e.notifier != nil {
			e.notifier.NotifyUpdate(item.Pair)
		}
	}

	if err := e.sync.MarkRealtimeStarted(ctx, item.Pair, item.WindowStart.UTC()); err != nil {
		slog.Warn("realtime cutover update failed",
			"task_id", item.ID,
			"pair", item.Pair,
			"error", err,
		)
		return err
	}

	return nil
}

// publishRealtime fetches the latest per-source snapshots for a live window and
// forwards them to Kafka without pre-consolidation so provider-native metadata
// such as trailing 24h volume survives into raw_candles. When Kafka is
// unavailable it falls back to the same raw→hourly materialisation path used by
// the realtime consumer.
func (e *RealtimeExecutor) publishRealtime(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	sourceCandles, err := e.fetchSourceCandles(ctx, item)
	if err != nil {
		return nil, err
	}

	if e.events != nil {
		if err := e.events.PublishCandles(ctx, sourceCandles); err != nil {
			return nil, err
		}
		slog.Debug("realtime task published market events",
			"task_id", item.ID,
			"pair", item.Pair,
			"interval", item.Interval,
			"candle_count", len(sourceCandles),
		)
		return nil, nil
	}

	if err := e.candles.UpsertRawCandles(ctx, "1h", sourceCandles); err != nil {
		return nil, err
	}

	rawCandles, err := e.candles.RawCandles(ctx, item.Pair, "1h", item.WindowStart, item.WindowEnd)
	if err != nil {
		return nil, err
	}

	return consolidate.FromRaw("1h", rawCandles)
}

func (e *RealtimeExecutor) fetchSourceCandles(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	base, quote, err := provider.ParsePairSymbol(item.Pair)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMarketSourceUnavailable, err)
	}

	if e.sources == nil {
		return nil, fmt.Errorf("%w: no source registry configured", ErrMarketSourceUnavailable)
	}

	items, err := e.sources.FetchCandles(ctx, provider.Request{
		TaskID:    item.ID,
		Pair:      item.Pair,
		Base:      base,
		Quote:     quote,
		Interval:  "ticker",
		StartTime: item.WindowStart.UTC(),
		EndTime:   item.WindowEnd.UTC(),
	})
	if err == nil {
		return items, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrMarketSourceUnavailable, err)
}
