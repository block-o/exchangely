package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest/backfill"
)

func TestBackfillExecutorWritesCandlesAndProgress(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCEUR", Interval: "1h", Timestamp: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Unix(), Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1, Source: "kraken"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC).Unix(), Open: 10.5, High: 11.5, Low: 10, Close: 11, Volume: 2, Source: "kraken"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC).Unix(), Open: 11, High: 12, Low: 10.5, Close: 11.6, Volume: 3, Source: "kraken"},
		},
	}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, nil, nil)

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

func TestBackfillExecutorFailsWhenSourceUnavailable(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, &fakeMarketSource{err: context.DeadlineExceeded}, nil, nil)

	item := task.Task{
		ID:          "backfill:BTCUSD:1",
		Type:        task.TypeBackfill,
		Pair:        "BTCUSD",
		Interval:    "1h",
		WindowStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC),
	}

	err := executor.Execute(context.Background(), item)
	if err == nil {
		t.Fatal("expected execute to fail when sources are unavailable")
	}
	if !errors.Is(err, ErrMarketSourceUnavailable) {
		t.Fatalf("expected ErrMarketSourceUnavailable, got %v", err)
	}
	if len(candleRepo.items) != 0 {
		t.Fatalf("expected no consolidated candles, got %d", len(candleRepo.items))
	}
}

func TestBackfillExecutorFailsWhenSourceRegistryMissing(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, nil, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "backfill:BTCEUR:missing-source",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected execute to fail without a source registry")
	}
	if !errors.Is(err, ErrMarketSourceUnavailable) {
		t.Fatalf("expected ErrMarketSourceUnavailable, got %v", err)
	}
}

func TestBackfillExecutorFailsWhenSourceCoverageIsIncomplete(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Unix(), Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1, Source: "coingecko"},
			{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC).Unix(), Open: 11, High: 12, Low: 10, Close: 11.5, Volume: 1, Source: "coingecko"},
		},
	}, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "backfill:BTCUSD:coverage-gap",
		Type:        task.TypeBackfill,
		Pair:        "BTCUSD",
		Interval:    "1h",
		WindowStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 4, 1, 3, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected execute to fail when source coverage is incomplete")
	}
	if !errors.Is(err, ErrIncompleteSourceCoverage) {
		t.Fatalf("expected ErrIncompleteSourceCoverage, got %v", err)
	}
	if len(candleRepo.rawItems) != 0 {
		t.Fatalf("expected no raw candles to be persisted, got %d", len(candleRepo.rawItems))
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
	executor := NewBackfillExecutor(candleRepo, syncRepo, nil, nil, nil)

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
	executor := NewBackfillExecutor(&fakeCandleStore{}, &fakeSyncWriter{}, nil, nil, nil)

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
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 102, Low: 99, Close: 101, Volume: 4, Volume24H: 400000, Source: "coingecko", Finalized: false},
		},
	}
	publisher := &fakeMarketPublisher{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, publisher, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "realtime:BTCUSD",
		Type:        task.TypeRealtime,
		Pair:        "BTCUSD",
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
	got := publisher.items[0]
	if got.Finalized {
		t.Fatalf("expected realtime candle to be marked non-finalized, got %+v", got)
	}
	if got.Source != "coingecko" || got.Volume24H != 400000 {
		t.Fatalf("expected realtime publisher to preserve source metadata, got %+v", got)
	}
	if len(candleRepo.rawItems) != 0 {
		t.Fatalf("expected realtime path to rely on event publishing, got raw items %+v", candleRepo.rawItems)
	}
	if syncRepo.lastPair != "" || syncRepo.interval != "" {
		t.Fatalf("expected realtime path not to advance historical sync cursor, got pair=%s interval=%s", syncRepo.lastPair, syncRepo.interval)
	}
	if syncRepo.realtimeStartedPair != "BTCUSD" {
		t.Fatalf("expected realtime cutover to be recorded for BTCUSD, got %s", syncRepo.realtimeStartedPair)
	}
}

func TestBackfillExecutorConsolidatesMultipleRealtimeCandles(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			// two non-finalized updates for the same window, followed by a finalized one
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 101, Low: 99, Close: 100, Volume: 1, Volume24H: 100000, Source: "coingecko", Finalized: false},
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 103, Low: 98, Close: 102, Volume: 2, Volume24H: 150000, Source: "coingecko", Finalized: false},
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 104, Low: 97, Close: 103, Volume: 3, Volume24H: 200000, Source: "coingecko", Finalized: true},
		},
	}
	publisher := &fakeMarketPublisher{}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, publisher, nil)
	err := executor.Execute(context.Background(), task.Task{
		ID:          "realtime:BTCUSD",
		Type:        task.TypeRealtime,
		Pair:        "BTCUSD",
		Interval:    "1h",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 1, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(publisher.items) != 3 {
		t.Fatalf("expected 3 raw published candles, got %d", len(publisher.items))
	}
	got := publisher.items[len(publisher.items)-1]
	if !got.Finalized {
		t.Fatalf("expected final realtime snapshot to be marked finalized, got %+v", got)
	}
	if got.High != 104 || got.Low != 97 || got.Close != 103 || got.Volume != 3 || got.Volume24H != 200000 {
		t.Fatalf("unexpected published candle values: %+v", got)
	}
	if got.Pair != "BTCUSD" || got.Interval != "1h" || got.Timestamp != 1711929600 {
		t.Fatalf("unexpected candle identity: %+v", got)
	}
	if len(candleRepo.rawItems) != 0 {
		t.Fatalf("expected realtime path to rely on event publishing, got raw items %+v", candleRepo.rawItems)
	}
	if syncRepo.lastPair != "" || syncRepo.interval != "" {
		t.Fatalf("expected realtime path not to advance historical sync cursor, got pair=%s interval=%s", syncRepo.lastPair, syncRepo.interval)
	}
	if syncRepo.realtimeStartedPair != "BTCUSD" {
		t.Fatalf("expected realtime cutover to be recorded for BTCUSD, got %s", syncRepo.realtimeStartedPair)
	}
}

func TestBackfillExecutorRealtimeWithoutPublisher(t *testing.T) {
	// Tests the path when TypeRealtime is used but no Kafka publisher is configured.
	// It should fall back to the raw->consolidated DB path used by the realtime consumer.
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 105, Low: 95, Close: 102, Volume: 5, Volume24H: 250000, Source: "s1", Finalized: true},
		},
	}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "realtime:BTCUSD:no-publisher",
		Type:        task.TypeRealtime,
		Pair:        "BTCUSD",
		Interval:    "1h",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 1, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(candleRepo.items) != 1 {
		t.Fatalf("expected 1 consolidated candle in DB, got %d", len(candleRepo.items))
	}
	if len(candleRepo.rawItems) != 1 {
		t.Fatalf("expected 1 raw realtime candle in DB, got %d", len(candleRepo.rawItems))
	}
	if candleRepo.items[0].Volume != 5 {
		t.Fatalf("unexpected candle volume: %v", candleRepo.items[0].Volume)
	}
	if candleRepo.rawItems[0].Volume24H != 250000 {
		t.Fatalf("expected raw realtime metadata to be preserved, got %+v", candleRepo.rawItems[0])
	}
}

func TestBackfillExecutorEmptyWindow(t *testing.T) {
	// Tests that when source returns no data, progress is still updated.
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{items: []candle.Candle{}}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "backfill:BTCUSD:empty",
		Type:        task.TypeBackfill,
		Pair:        "BTCUSD",
		Interval:    "1h",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 1, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(candleRepo.items) != 0 {
		t.Fatalf("expected 0 candles, got %d", len(candleRepo.items))
	}
	if syncRepo.lastPair != "BTCUSD" {
		t.Fatalf("expected sync progress despite no data, got %s", syncRepo.lastPair)
	}
}

func TestBackfillExecutorConsolidatesMultipleHistoricalCandles(t *testing.T) {
	// Tests that historical backfill consolidates snapshots correctly before saving.
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711929600, Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 1, Source: "s1"},
			{Pair: "BTCEUR", Interval: "1h", Timestamp: 1711929600, Open: 10, High: 11.5, Low: 8.5, Close: 11, Volume: 2, Source: "s1"},
		},
	}
	executor := NewBackfillExecutor(candleRepo, syncRepo, source, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		ID:          "backfill:BTCEUR:consolidation",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2024, 4, 1, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(candleRepo.items) != 1 {
		t.Fatalf("expected 1 consolidated candle, got %d", len(candleRepo.items))
	}
	got := candleRepo.items[0]
	// Volume should be 2 (latest snapshot wins) because they have the same source.
	if got.Volume != 2 || got.High != 11.5 || got.Low != 8.5 || got.Close != 11 {
		t.Fatalf("unexpected consolidated values: %+v", got)
	}
}

func TestBackfillExecutorIntervalUtilities(t *testing.T) {
	// Test intervalDuration
	d, err := intervalDuration("1h")
	if err != nil || d != time.Hour {
		t.Errorf("expected 1h duration, got %v (%v)", d, err)
	}
	d, err = intervalDuration("1d")
	if err != nil || d != 24*time.Hour {
		t.Errorf("expected 1d duration, got %v (%v)", d, err)
	}
	_, err = intervalDuration("1m")
	if err == nil {
		t.Error("expected error for unsupported interval")
	}

	// Test backfillComplete for hourly
	now := time.Now().UTC()
	task1h := task.Task{Interval: "1h", WindowEnd: now.Truncate(time.Hour)}
	if !backfillComplete(task1h) {
		t.Error("expected hourly task at now to be complete")
	}
	task1hOld := task.Task{Interval: "1h", WindowEnd: now.Add(-2 * time.Hour)}
	if backfillComplete(task1hOld) {
		t.Error("expected old hourly task to be incomplete")
	}

	// Test backfillComplete for daily
	task1d := task.Task{Interval: "1d", WindowEnd: now.Truncate(24 * time.Hour)}
	if !backfillComplete(task1d) {
		t.Error("expected daily task at now to be complete")
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
	lastPair            string
	interval            string
	realtimeStartedPair string
	realtimeStartedAt   time.Time
}

func (f *fakeSyncWriter) UpsertProgress(_ context.Context, pairSymbol, interval string, _ time.Time, _ bool) error {
	f.lastPair = pairSymbol
	f.interval = interval
	return nil
}

func (f *fakeSyncWriter) MarkRealtimeStarted(_ context.Context, pairSymbol string, startedAt time.Time) error {
	f.realtimeStartedPair = pairSymbol
	f.realtimeStartedAt = startedAt.UTC()
	return nil
}

type fakeMarketSource struct {
	name  string
	items []candle.Candle
	err   error
}

func (f *fakeMarketSource) FetchCandles(_ context.Context, _ backfill.Request) ([]candle.Candle, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeMarketSource) Name() string {
	if f.name != "" {
		return f.name
	}
	return "fake"
}

func (f *fakeMarketSource) Supports(_ backfill.Request) bool {
	return true
}

type fakeMarketPublisher struct {
	items []candle.Candle
}

func (f *fakeMarketPublisher) PublishCandles(_ context.Context, candles []candle.Candle) error {
	f.items = append(f.items, candles...)
	return nil
}
