package worker

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

func TestRealtimeExecutorPublishesCandlesToEvents(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 102, Low: 99, Close: 101, Volume: 4, Volume24H: 400000, Source: "coingecko", Finalized: false},
		},
	}
	publisher := &fakeMarketPublisher{}
	executor := NewRealtimeExecutor(candleRepo, syncRepo, source, publisher, nil)

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

func TestRealtimeExecutorConsolidatesMultipleCandles(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 101, Low: 99, Close: 100, Volume: 1, Volume24H: 100000, Source: "coingecko", Finalized: false},
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 103, Low: 98, Close: 102, Volume: 2, Volume24H: 150000, Source: "coingecko", Finalized: false},
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 104, Low: 97, Close: 103, Volume: 3, Volume24H: 200000, Source: "coingecko", Finalized: true},
		},
	}
	publisher := &fakeMarketPublisher{}
	executor := NewRealtimeExecutor(candleRepo, syncRepo, source, publisher, nil)
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
	if syncRepo.realtimeStartedPair != "BTCUSD" {
		t.Fatalf("expected realtime cutover to be recorded for BTCUSD, got %s", syncRepo.realtimeStartedPair)
	}
}

func TestRealtimeExecutorWithoutPublisher(t *testing.T) {
	candleRepo := &fakeCandleStore{}
	syncRepo := &fakeSyncWriter{}
	source := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCUSD", Interval: "1h", Timestamp: 1711929600, Open: 100, High: 105, Low: 95, Close: 102, Volume: 5, Volume24H: 250000, Source: "s1", Finalized: true},
		},
	}
	executor := NewRealtimeExecutor(candleRepo, syncRepo, source, nil, nil)

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

func TestRealtimeExecutorRejectsNonRealtimeTask(t *testing.T) {
	executor := NewRealtimeExecutor(&fakeCandleStore{}, &fakeSyncWriter{}, &fakeMarketSource{}, nil, nil)

	err := executor.Execute(context.Background(), task.Task{
		Type: task.TypeBackfill,
		Pair: "BTCUSD",
	})
	if err == nil {
		t.Fatal("expected error when handling non-realtime task type")
	}
}
