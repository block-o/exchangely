package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/consolidate"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

var ErrHourlyDataUnavailable = errors.New("hourly data unavailable for daily consolidation")
var ErrMarketSourceUnavailable = errors.New("market source unavailable")
var ErrIncompleteSourceCoverage = errors.New("incomplete source coverage")

type CandleStore interface {
	UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error
	UpsertRawCandles(ctx context.Context, interval string, candles []candle.Candle) error
	RawCandles(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
	HourlyCandles(ctx context.Context, pairSymbol string, start, end time.Time) ([]candle.Candle, error)
}

type SyncProgressWriter interface {
	UpsertProgress(ctx context.Context, pairSymbol, interval string, lastSynced time.Time, backfillCompleted bool) error
	MarkRealtimeStarted(ctx context.Context, pairSymbol string, startedAt time.Time) error
}

// BackfillExecutor materializes task windows into persisted candles and sync progress updates.
// It handles historical backfill and daily consolidation:
//  1. Historical Backfill: Fetches source items, consolidates them, and saves to both raw and
//     consolidated Postgres tables.
//  2. Daily Consolidation: Rolls up stored hourly candles into daily candles.
type BackfillExecutor struct {
	candles  CandleStore
	sync     SyncProgressWriter
	sources  MarketSource
	notifier MarketNotifier
}

type MarketSource interface {
	FetchCandles(ctx context.Context, request provider.Request) ([]candle.Candle, error)
}

// MarketNotifier pushes a signal to the SSE EventBus when new data is committed to Postgres.
// The executor calls it after every successful candle write.
type MarketNotifier interface {
	NotifyUpdate(pairSymbol string)
}

// NewBackfillExecutor returns the worker-side executor for backfill and daily consolidation tasks.
func NewBackfillExecutor(candles CandleStore, sync SyncProgressWriter, sources MarketSource, notifier MarketNotifier) *BackfillExecutor {
	return &BackfillExecutor{
		candles:  candles,
		sync:     sync,
		sources:  sources,
		notifier: notifier,
	}
}

// Execute routes the task window to its specific materialization path (historical, daily,
// or realtime) and updates the sync progress or realtime cutover marks on success.
// Historical results are updated in both Postgres and the SSE notification bus.
func (e *BackfillExecutor) Execute(ctx context.Context, item task.Task) error {
	startedAt := time.Now()
	switch item.Type {
	case task.TypeBackfill, task.TypeConsolidate:
		slog.DebugContext(ctx, "task execution active",
			"task_id", item.ID,
			"type", item.Type,
			"pair", item.Pair,
			"interval", item.Interval,
		)
	default:
		return fmt.Errorf("unsupported task type %q", item.Type)
	}

	candles, err := e.materializeCandles(ctx, item)
	if err != nil {
		slog.Warn("task execution failed",
			"task_id", item.ID,
			"type", item.Type,
			"pair", item.Pair,
			"interval", item.Interval,
			"window_start", item.WindowStart.UTC().Format(time.RFC3339),
			"window_end", item.WindowEnd.UTC().Format(time.RFC3339),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return err
	}
	if len(candles) == 0 {
		if err := e.updateSyncProgress(ctx, item); err != nil {
			return err
		}
		slog.Warn("task execution produced no candles",
			"task_id", item.ID,
			"type", item.Type,
			"pair", item.Pair,
			"interval", item.Interval,
			"window_start", item.WindowStart.UTC().Format(time.RFC3339),
			"window_end", item.WindowEnd.UTC().Format(time.RFC3339),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
		return nil
	}

	if err := e.candles.UpsertCandles(ctx, item.Interval, candles); err != nil {
		slog.Warn("task candle upsert failed",
			"task_id", item.ID,
			"type", item.Type,
			"pair", item.Pair,
			"interval", item.Interval,
			"candle_count", len(candles),
			"error", err,
		)
		return err
	}

	if err := e.updateSyncProgress(ctx, item); err != nil {
		return err
	}

	if e.notifier != nil {
		e.notifier.NotifyUpdate(item.Pair)
	}

	return nil
}

func (e *BackfillExecutor) updateSyncProgress(ctx context.Context, item task.Task) error {
	if err := e.sync.UpsertProgress(ctx, item.Pair, item.Interval, item.WindowEnd.UTC(), backfillComplete(item)); err != nil {
		slog.Warn("task sync progress update failed",
			"task_id", item.ID,
			"type", item.Type,
			"pair", item.Pair,
			"interval", item.Interval,
			"error", err,
		)
		return err
	}
	return nil
}

// materializeCandles determines the specific fetch/consolidation strategy based on
// task type and interval (hourly or daily).
func (e *BackfillExecutor) materializeCandles(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	switch item.Interval {
	case "1d":
		return e.materializeDaily(ctx, item)
	default:
		return e.materializeHourly(ctx, item)
	}
}

func (e *BackfillExecutor) materializeHourly(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	sourceCandles, err := e.fetchSourceCandles(ctx, item)
	if err != nil {
		return nil, err
	}
	if len(sourceCandles) == 0 {
		return nil, nil
	}
	if err := validateCoverage(item.Interval, item.WindowStart, item.WindowEnd, sourceCandles); err != nil {
		slog.Warn("incomplete source coverage, continuing with available candles",
			"pair", item.Pair,
			"interval", item.Interval,
			"window_start", item.WindowStart.UTC().Format(time.RFC3339),
			"window_end", item.WindowEnd.UTC().Format(time.RFC3339),
			"error", err,
		)
	}

	if err := e.candles.UpsertRawCandles(ctx, item.Interval, sourceCandles); err != nil {
		return nil, err
	}

	rawCandles, err := e.candles.RawCandles(ctx, item.Pair, item.Interval, item.WindowStart, item.WindowEnd)
	if err != nil {
		return nil, err
	}

	return consolidate.FromRaw(item.Interval, rawCandles)
}

func (e *BackfillExecutor) materializeDaily(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	hourlyCandles, err := e.candles.HourlyCandles(ctx, item.Pair, item.WindowStart, item.WindowEnd)
	if err != nil {
		return nil, err
	}
	if len(hourlyCandles) == 0 {
		return nil, ErrHourlyDataUnavailable
	}

	slog.Info("daily consolidation started",
		"task_id", item.ID,
		"pair", item.Pair,
		"interval", item.Interval,
		"hourly_candle_count", len(hourlyCandles),
	)
	return consolidate.DailyFromHourly(hourlyCandles)
}

func (e *BackfillExecutor) fetchSourceCandles(ctx context.Context, item task.Task) ([]candle.Candle, error) {
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
		Interval:  item.Interval,
		StartTime: item.WindowStart.UTC(),
		EndTime:   item.WindowEnd.UTC(),
	})
	if err == nil {
		return items, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrMarketSourceUnavailable, err)
}

func validateCoverage(interval string, start, end time.Time, items []candle.Candle) error {
	step, err := intervalDuration(interval)
	if err != nil {
		return err
	}

	expected := make(map[int64]struct{})
	for cursor := start.UTC(); cursor.Before(end.UTC()); cursor = cursor.Add(step) {
		expected[cursor.Unix()] = struct{}{}
	}

	for _, item := range items {
		delete(expected, item.Timestamp)
	}

	if len(expected) == 0 {
		return nil
	}

	missing := make([]int64, 0, len(expected))
	for ts := range expected {
		missing = append(missing, ts)
	}
	return fmt.Errorf("%w: missing %d candles between %s and %s", ErrIncompleteSourceCoverage, len(missing), start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
}

func intervalDuration(interval string) (time.Duration, error) {
	switch interval {
	case "1h":
		return time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported interval %q", interval)
	}
}

func backfillComplete(item task.Task) bool {
	if item.Type == task.TypeConsolidate {
		return false
	}
	now := time.Now().UTC()
	switch item.Interval {
	case "1d":
		return !item.WindowEnd.Before(now.Truncate(24 * time.Hour))
	default:
		return !item.WindowEnd.Before(now.Truncate(time.Hour))
	}
}
