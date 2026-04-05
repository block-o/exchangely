package service

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

func TestRealtimeIngestServiceStoresRawAndMaterializesHourly(t *testing.T) {
	store := &fakeRealtimeStore{}
	service := NewRealtimeIngestService(store, nil)

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

type fakeRealtimeStore struct {
	rawItems          []candle.Candle
	consolidatedItems []candle.Candle
}

func (f *fakeRealtimeStore) UpsertRawCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.rawItems = append(f.rawItems, candles...)
	return nil
}

func (f *fakeRealtimeStore) RawCandles(_ context.Context, _ string, _ string, _ time.Time, _ time.Time) ([]candle.Candle, error) {
	return append([]candle.Candle{}, f.rawItems...), nil
}

func (f *fakeRealtimeStore) UpsertCandles(_ context.Context, _ string, candles []candle.Candle) error {
	f.consolidatedItems = append(f.consolidatedItems, candles...)
	return nil
}
