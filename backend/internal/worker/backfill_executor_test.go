package worker

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestBackfillExecutorWritesCandlesAndProgress(t *testing.T) {
	candleRepo := &fakeCandleWriter{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711929600, Source: "kraken"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711933200, Source: "kraken"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711936800, Source: "kraken"},
		},
	}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source)

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
	if candleRepo.items[0].Source != "kraken" {
		t.Fatalf("expected source candles to be used, got %+v", candleRepo.items[0])
	}
	if syncRepo.lastPair != "BTCEUR" {
		t.Fatalf("expected sync progress for BTCEUR, got %s", syncRepo.lastPair)
	}
}

func TestBackfillExecutorFallsBackWhenSourceUnavailable(t *testing.T) {
	candleRepo := &fakeCandleWriter{}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, &fakeMarketSource{err: context.DeadlineExceeded})

	item := task.Task{
		ID:          "backfill:BTCUSDT:1",
		Type:        task.TypeBackfill,
		Pair:        "BTCUSDT",
		Interval:    "1h",
		WindowStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC),
	}

	if err := executor.Execute(context.Background(), item); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(candleRepo.items) != 2 {
		t.Fatalf("expected fallback candles, got %d", len(candleRepo.items))
	}
	if candleRepo.items[0].Source != "bootstrap-fallback" {
		t.Fatalf("expected fallback source, got %+v", candleRepo.items[0])
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

type fakeMarketSource struct {
	items []candle.Candle
	err   error
}

func (f *fakeMarketSource) FetchCandles(_ context.Context, _ ingest.Request) ([]candle.Candle, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}
