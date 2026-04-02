package worker

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
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
}

func NewBackfillExecutor(candles CandleWriter, sync SyncProgressWriter) *BackfillExecutor {
	return &BackfillExecutor{
		candles: candles,
		sync:    sync,
	}
}

func (e *BackfillExecutor) Execute(ctx context.Context, item task.Task) error {
	switch item.Type {
	case task.TypeBackfill, task.TypeConsolidate, task.TypeRealtime:
	default:
		return fmt.Errorf("unsupported task type %q", item.Type)
	}

	candles := generateCandles(item)
	if len(candles) == 0 {
		return e.sync.UpsertProgress(ctx, item.Pair, item.WindowEnd.UTC(), backfillComplete(item))
	}

	if err := e.candles.UpsertCandles(ctx, item.Interval, candles); err != nil {
		return err
	}

	return e.sync.UpsertProgress(ctx, item.Pair, item.WindowEnd.UTC(), backfillComplete(item))
}

func generateCandles(item task.Task) []candle.Candle {
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
			Source:    "bootstrap-executor",
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
