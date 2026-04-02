package worker

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest"
	"github.com/block-o/exchangely/backend/internal/ingest/registry"
)

type CandleWriter interface {
	UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error
}

type SyncProgressWriter interface {
	UpsertProgress(ctx context.Context, pairSymbol string, lastSynced time.Time, backfillCompleted bool) error
}

type BackfillExecutor struct {
	candles CandleWriter
	sync    SyncProgressWriter
	sources MarketSource
}

type MarketSource interface {
	FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error)
}

func NewBackfillExecutor(candles CandleWriter, sync SyncProgressWriter, sources MarketSource) *BackfillExecutor {
	return &BackfillExecutor{
		candles: candles,
		sync:    sync,
		sources: sources,
	}
}

func (e *BackfillExecutor) Execute(ctx context.Context, item task.Task) error {
	switch item.Type {
	case task.TypeBackfill, task.TypeConsolidate, task.TypeRealtime:
	default:
		return fmt.Errorf("unsupported task type %q", item.Type)
	}

	candles, err := e.fetchCandles(ctx, item)
	if err != nil {
		return err
	}
	if len(candles) == 0 {
		return e.sync.UpsertProgress(ctx, item.Pair, item.WindowEnd.UTC(), backfillComplete(item))
	}

	if err := e.candles.UpsertCandles(ctx, item.Interval, candles); err != nil {
		return err
	}

	return e.sync.UpsertProgress(ctx, item.Pair, item.WindowEnd.UTC(), backfillComplete(item))
}

func (e *BackfillExecutor) fetchCandles(ctx context.Context, item task.Task) ([]candle.Candle, error) {
	base, quote, err := registry.ParsePairSymbol(item.Pair)
	if err != nil {
		return generateCandles(item, "bootstrap-fallback"), nil
	}

	if e.sources == nil {
		return generateCandles(item, "bootstrap-fallback"), nil
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

	log.Printf("market source fallback for %s %s: %v", item.Pair, item.Interval, err)
	return generateCandles(item, "bootstrap-fallback"), nil
}

func generateCandles(item task.Task, source string) []candle.Candle {
	step := time.Hour
	if item.Interval == "1d" {
		step = 24 * time.Hour
	}

	result := make([]candle.Candle, 0)
	index := 0.0
	basePrice := pairSeed(item.Pair)

	for cursor := item.WindowStart.UTC(); cursor.Before(item.WindowEnd.UTC()); cursor = cursor.Add(step) {
		open := basePrice + index
		close := open + 1.75
		result = append(result, candle.Candle{
			Pair:      item.Pair,
			Interval:  item.Interval,
			Timestamp: cursor.Unix(),
			Open:      open,
			High:      close + 0.9,
			Low:       open - 0.85,
			Close:     close,
			Volume:    500 + (index * 5),
			Source:    source,
			Finalized: true,
		})
		index += 1
	}

	return result
}

func pairSeed(pairSymbol string) float64 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(pairSymbol))
	return float64((hasher.Sum32()%5000)+1000) / 10
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
