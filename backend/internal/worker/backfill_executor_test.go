package worker

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

func TestBackfillExecutorWritesCandlesAndProgress(t *testing.T) {
	candleRepo := &fakeCandleWriter{}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo)

	item := task.Task{
		ID:          "backfill:BTCEUR:1",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 4, 1, 3, 0, 0, 0, time.UTC),
	}

	if err := executor.Execute(context.Background(), item); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(candleRepo.items) != 3 {
		t.Fatalf("expected 3 candles, got %d", len(candleRepo.items))
	}
	if syncRepo.lastPair != "BTCEUR" {
		t.Fatalf("expected sync progress for BTCEUR, got %s", syncRepo.lastPair)
	}
}

type fakeCandleWriter struct {
	items []candle.Candle
}

func (f *fakeCandleWriter) UpsertCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.items = append(f.items, candles...)
	return nil
}

type fakeSyncWriter struct {
	lastPair string
}

func (f *fakeSyncWriter) UpsertProgress(_ context.Context, pairSymbol string, _ time.Time, _ bool) error {
	f.lastPair = pairSymbol
	return nil
}
