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
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711929600, Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1, Source: "kraken"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711933200, Open: 10.5, High: 11.5, Low: 10, Close: 11, Volume: 2, Source: "kraken"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711936800, Open: 11, High: 12, Low: 10.5, Close: 11.6, Volume: 3, Source: "kraken"},
		},
	}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, nil)

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
	if len(candleRepo.rawItems) != 3 {
		t.Fatalf("expected raw candles to be stored, got %d", len(candleRepo.rawItems))
	}
	if candleRepo.items[0].Source != "consolidated" || candleRepo.items[0].Open != 10 || candleRepo.items[0].Close != 10.5 {
		t.Fatalf("expected consolidated hourly candles, got %+v", candleRepo.items[0])
	}
	if syncRepo.lastPair != "BTCEUR" {
		t.Fatalf("expected sync progress for BTCEUR, got %s", syncRepo.lastPair)
	}
}

func TestBackfillExecutorFallsBackWhenSourceUnavailable(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, &fakeMarketSource{err: context.DeadlineExceeded}, nil)

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
	if candleRepo.items[0].Source != "consolidated" {
		t.Fatalf("expected consolidated fallback candle, got %+v", candleRepo.items[0])
	}
}

func TestBackfillExecutorBuildsDailyFromHourly(t *testing.T) {
	candleRepo := &fakeCandleStore{
		hourlyItems: []candle.Candle{
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711929600, Open: 10, High: 12, Low: 9, Close: 11, Volume: 1, Source: "consolidated", Finalized: true},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711933200, Open: 11, High: 13, Low: 10, Close: 12, Volume: 2, Source: "consolidated", Finalized: true},
		},
	}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, nil, nil)

	item := task.Task{
		ID:          "backfill:BTCEUR:daily",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1d",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 2, 0, 0, 0, 0, time.UTC),
	}

	if err := executor.Execute(context.Background(), item); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(candleRepo.items) != 1 {
		t.Fatalf("expected 1 daily candle, got %d", len(candleRepo.items))
	}
	if candleRepo.items[0].Interval != "1d" || candleRepo.items[0].Open != 10 || candleRepo.items[0].Close != 12 {
		t.Fatalf("unexpected daily candle: %+v", candleRepo.items[0])
	}
}

func TestBackfillExecutorFailsDailyWithoutHourlyCoverage(t *testing.T) {
	executor := NewBackfillExecutor(&fakeCandleStore{}, &fakeSyncWriter{}, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "backfill:BTCEUR:daily-missing",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1d",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 2, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected daily execution to fail when hourly data is missing")
	}
	if err != ErrHourlyDataUnavailable {
		t.Fatalf("expected ErrHourlyDataUnavailable, got %v", err)
	}
}

func TestBackfillExecutorPublishesRealtimeCandlesToEvents(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCUSDT", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 102, Low: 99, Close: 101, Volume: 4, Source: "binance", Finalized: true},
		},
	}
	publisher := &fakeMarketPublisher{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, publisher)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "realtime:BTCUSDT",
		Type:        task.TypeRealtime,
		Pair:        "BTCUSDT",
		Interval:    "1h",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 1, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(publisher.items) != 1 {
		t.Fatalf("expected 1 published candle, got %d", len(publisher.items))
	}
	if publisher.items[0].Finalized {
		t.Fatalf("expected realtime candle to be marked non-finalized, got %+v", publisher.items[0])
	}
	if len(candleRepo.rawItems) != 0 {
		t.Fatalf("expected realtime path to rely on event publishing, got raw items %+v", candleRepo.rawItems)
	}
}

type fakeCandleStore struct {
	items       []candle.Candle
	rawItems    []candle.Candle
	hourlyItems []candle.Candle
}

func (f *fakeCandleStore) UpsertCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.items = append(f.items, candles...)
	return nil
}

func (f *fakeCandleStore) UpsertRawCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.rawItems = append(f.rawItems, candles...)
	return nil
}

func (f *fakeCandleStore) RawCandles(_ context.Context, _ string, _ string, _ time.Time, _ time.Time) ([]candle.Candle, error) {
	if len(f.rawItems) > 0 {
		return append([]candle.Candle{}, f.rawItems...), nil
	}
	return append([]candle.Candle{}, f.items...), nil
}

func (f *fakeCandleStore) HourlyCandles(_ context.Context, _ string, _ time.Time, _ time.Time) ([]candle.Candle, error) {
	return append([]candle.Candle{}, f.hourlyItems...), nil
}

type fakeSyncWriter struct {
	lastPair string
	interval string
}

func (f *fakeSyncWriter) UpsertProgress(_ context.Context, pairSymbol, interval string, _ time.Time, _ bool) error {
	f.lastPair = pairSymbol
	f.interval = interval
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

type fakeMarketPublisher struct {
	items []candle.Candle
}

func (f *fakeMarketPublisher) PublishCandles(_ context.Context, candles []candle.Candle) error {
	f.items = append(f.items, candles...)
	return nil
}
