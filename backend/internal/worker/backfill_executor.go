package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/block-o/exchangely/backend/internal/consolidate"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest"
	"github.com/block-o/exchangely/backend/internal/ingest/registry"
)

var ErrHourlyDataUnavailable = errors.New("hourly data unavailable for daily consolidation")
var ErrMarketSourceUnavailable = errors.New("market source unavailable")

type CandleStore interface {
	UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error
	UpsertRawCandles(ctx context.Context, interval string, candles []candle.Candle) error
	RawCandles(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
	HourlyCandles(ctx context.Context, pairSymbol string, start, end time.Time) ([]candle.Candle, error)
}

type SyncProgressWriter interface {
	UpsertProgress(ctx context.Context, pairSymbol, interval string, lastSynced time.Time, backfillCompleted bool) error
}

type BackfillExecutor struct {
	candles CandleStore
	sync    SyncProgressWriter
	sources MarketSource
	events  MarketEventPublisher
}

type MarketSource interface {
	FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error)
}

type MarketEventPublisher interface {
	PublishCandles(ctx context.Context, candles []candle.Candle) error
}

func NewBackfillExecutor(candles CandleStore, sync SyncProgressWriter, sources MarketSource, events MarketEventPublisher) *BackfillExecutor {
	return &BackfillExecutor{
		candles: candles,
		sync:    sync,
		sources: sources,
		events:  events,
	}
}

func (e *BackfillExecutor) Execute(ctx context.Context, item task.Task) error {
	switch item.Type {
	case task.TypeBackfill, task.TypeConsolidate, task.TypeRealtime:
	default:
		return fmt.Errorf("unsupported task type %q", item.Type)
	}

	candles, err := e.materializeCandles(ctx, item)
	if err != nil {
		return err
	}
	if len(candles) == 0 {
		return e.sync.UpsertProgress(ctx, item.Pair, item.Interval, item.WindowEnd.UTC(), backfillComplete(item))
	}

	if err := e.candles.UpsertCandles(ctx, item.Interval, candles); err != nil {
		return err
	}

	return e.sync.UpsertProgress(ctx, item.Pair, item.Interval, item.WindowEnd.UTC(), backfillComplete(item))
}

func (e *BackfillExecutor) materializeCandles(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	if item.Type == task.TypeRealtime {
		return e.publishRealtime(ctx, item)
	}

	switch item.Interval {
	case "1d":
		return e.materializeDaily(ctx, item)
	default:
		return e.materializeHourly(ctx, item)
	}
}

func (e *BackfillExecutor) publishRealtime(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	sourceCandles, err := e.fetchSourceCandles(ctx, item)
	if err != nil {
		return nil, err
	}
	for index := range sourceCandles {
		sourceCandles[index].Finalized = false
	}

	if e.events != nil {
		if err := e.events.PublishCandles(ctx, sourceCandles); err != nil {
			return nil, err
		}
		return nil, nil
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

func (e *BackfillExecutor) materializeHourly(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	sourceCandles, err := e.fetchSourceCandles(ctx, item)
	if err != nil {
		return nil, err
	}
	if len(sourceCandles) == 0 {
		return nil, nil
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

	return consolidate.DailyFromHourly(hourlyCandles)
}

func (e *BackfillExecutor) fetchSourceCandles(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	base, quote, err := registry.ParsePairSymbol(item.Pair)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMarketSourceUnavailable, err)
	}

	if e.sources == nil {
		return nil, fmt.Errorf("%w: no source registry configured", ErrMarketSourceUnavailable)
	}

	items, err := e.sources.FetchCandles(ctx, ingest.Request{
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

	log.Printf("market source fetch failed for %s %s: %v", item.Pair, item.Interval, err)
	return nil, fmt.Errorf("%w: %v", ErrMarketSourceUnavailable, err)
}

func backfillComplete(item task.Task) bool {
	now := time.Now().UTC()
	switch item.Interval {
	case "1d":
		return !item.WindowEnd.Before(now.Truncate(24 * time.Hour))
	default:
		return !item.WindowEnd.Before(now.Truncate(time.Hour))
	}
}
