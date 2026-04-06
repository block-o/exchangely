package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

func TestIngestServiceStoresRawAndMaterializesHourly(t *testing.T) {
	store := &fakeRealtimeStore{}
	service := NewIngestService(store, nil)

	err := service.IngestRealtimeCandles(context.Background(), []candle.Candle{
		{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC).Unix(), Open: 100, High: 102, Low: 99, Close: 101, Volume: 3, Source: "coingecko"},
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	if len(store.rawItems) != 1 {
		t.Fatalf("expected 1 raw candle, got %d", len(store.rawItems))
	}
	if len(store.consolidatedItems) != 1 {
		t.Fatalf("expected 1 consolidated candle, got %d", len(store.consolidatedItems))
	}
	if store.consolidatedItems[0].Source != "consolidated" {
		t.Fatalf("expected consolidated source, got %+v", store.consolidatedItems[0])
	}
}

func TestIngestServiceReturnsNilForEmptyInput(t *testing.T) {
	store := &fakeRealtimeStore{}
	service := NewIngestService(store, nil)

	if err := service.IngestRealtimeCandles(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for empty input, got %v", err)
	}
	if store.upsertRawCalls != 0 || store.upsertConsolidatedCalls != 0 {
		t.Fatalf("expected no store writes for empty input, got raw=%d consolidated=%d", store.upsertRawCalls, store.upsertConsolidatedCalls)
	}
}

func TestIngestServiceNotifiesAfterSuccessfulIngest(t *testing.T) {
	store := &fakeRealtimeStore{}
	notifier := &fakeNotifier{}
	service := NewIngestService(store, notifier)

	err := service.IngestRealtimeCandles(context.Background(), []candle.Candle{
		{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC).Unix(), Open: 100, High: 102, Low: 99, Close: 101, Volume: 3, Source: "coingecko"},
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if notifier.notifyCalls != 1 {
		t.Fatalf("expected 1 notification, got %d", notifier.notifyCalls)
	}
}

func TestIngestServicePropagatesRawUpsertError(t *testing.T) {
	store := &fakeRealtimeStore{upsertRawErr: errors.New("raw write failed")}
	service := NewIngestService(store, nil)

	err := service.IngestRealtimeCandles(context.Background(), []candle.Candle{
		{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC).Unix(), Open: 100, High: 102, Low: 99, Close: 101, Volume: 3, Source: "coingecko"},
	})
	if !errors.Is(err, store.upsertRawErr) {
		t.Fatalf("expected raw upsert error, got %v", err)
	}
	if store.loadRawCalls != 0 || store.upsertConsolidatedCalls != 0 {
		t.Fatalf("expected ingest to stop after raw upsert failure, got load=%d consolidated=%d", store.loadRawCalls, store.upsertConsolidatedCalls)
	}
}

func TestIngestServicePropagatesRawLoadError(t *testing.T) {
	store := &fakeRealtimeStore{loadRawErr: errors.New("read failed")}
	service := NewIngestService(store, nil)

	err := service.IngestRealtimeCandles(context.Background(), []candle.Candle{
		{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC).Unix(), Open: 100, High: 102, Low: 99, Close: 101, Volume: 3, Source: "coingecko"},
	})
	if !errors.Is(err, store.loadRawErr) {
		t.Fatalf("expected raw load error, got %v", err)
	}
	if store.upsertConsolidatedCalls != 0 {
		t.Fatalf("expected no consolidated write after load failure, got %d", store.upsertConsolidatedCalls)
	}
}

func TestIngestServiceSkipsConsolidatedWriteWhenNoOutputIsProduced(t *testing.T) {
	store := &fakeRealtimeStore{rawCandles: []candle.Candle{}}
	notifier := &fakeNotifier{}
	service := NewIngestService(store, notifier)

	err := service.IngestRealtimeCandles(context.Background(), []candle.Candle{
		{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC).Unix(), Open: 100, High: 102, Low: 99, Close: 101, Volume: 3, Source: "coingecko"},
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if store.upsertConsolidatedCalls != 0 {
		t.Fatalf("expected no consolidated writes, got %d", store.upsertConsolidatedCalls)
	}
	if notifier.notifyCalls != 0 {
		t.Fatalf("expected no notification when no consolidated output is produced, got %d", notifier.notifyCalls)
	}
}

func TestIngestServicePropagatesConsolidatedUpsertError(t *testing.T) {
	store := &fakeRealtimeStore{upsertConsolidatedErr: errors.New("hourly write failed")}
	service := NewIngestService(store, nil)

	err := service.IngestRealtimeCandles(context.Background(), []candle.Candle{
		{Pair: "BTCUSD", Interval: "1h", Timestamp: time.Date(2024, 4, 1, 10, 0, 0, 0, time.UTC).Unix(), Open: 100, High: 102, Low: 99, Close: 101, Volume: 3, Source: "coingecko"},
	})
	if !errors.Is(err, store.upsertConsolidatedErr) {
		t.Fatalf("expected consolidated upsert error, got %v", err)
	}
}

type fakeRealtimeStore struct {
	rawItems                []candle.Candle
	rawCandles              []candle.Candle
	consolidatedItems       []candle.Candle
	upsertRawErr            error
	loadRawErr              error
	upsertConsolidatedErr   error
	upsertRawCalls          int
	loadRawCalls            int
	upsertConsolidatedCalls int
}

func (f *fakeRealtimeStore) UpsertRawCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.upsertRawCalls++
	if f.upsertRawErr != nil {
		return f.upsertRawErr
	}
	f.rawItems = append(f.rawItems, candles...)
	return nil
}

func (f *fakeRealtimeStore) RawCandles(_ context.Context, _ string, _ string, _ time.Time, _ time.Time) ([]candle.Candle, error) {
	f.loadRawCalls++
	if f.loadRawErr != nil {
		return nil, f.loadRawErr
	}
	if f.rawCandles != nil {
		return append([]candle.Candle{}, f.rawCandles...), nil
	}
	return append([]candle.Candle{}, f.rawItems...), nil
}

func (f *fakeRealtimeStore) UpsertCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.upsertConsolidatedCalls++
	if f.upsertConsolidatedErr != nil {
		return f.upsertConsolidatedErr
	}
	f.consolidatedItems = append(f.consolidatedItems, candles...)
	return nil
}

type fakeNotifier struct {
	notifyCalls int
}

func (f *fakeNotifier) NotifyUpdate() {
	f.notifyCalls++
}
