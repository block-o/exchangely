package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

// --- Mock: CandleFinder ---

type mockCandleFinder struct {
	hourly map[string][]candle.Candle // keyed by pairSymbol
	daily  map[string][]candle.Candle // keyed by pairSymbol
}

func (m *mockCandleFinder) HourlyCandles(_ context.Context, pairSymbol string, _, _ time.Time) ([]candle.Candle, error) {
	return m.hourly[pairSymbol], nil
}

func (m *mockCandleFinder) Historical(_ context.Context, pairSymbol, _ string, _, _ time.Time) ([]candle.Candle, error) {
	return m.daily[pairSymbol], nil
}

func TestPriceResolver_HourlyResolution(t *testing.T) {
	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{
			"BTCUSD": {
				{Pair: "BTCUSD", Interval: "1h", Close: 42000.50},
			},
		},
	}

	resolver := NewPriceResolver(finder)
	ts := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	res, err := resolver.Resolve(context.Background(), "BTC", "USD", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Method != "hourly" {
		t.Errorf("expected method %q, got %q", "hourly", res.Method)
	}
	if res.Price != 42000.50 {
		t.Errorf("expected price %v, got %v", 42000.50, res.Price)
	}
}

func TestPriceResolver_DailyFallback(t *testing.T) {
	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{}, // no hourly data
		daily: map[string][]candle.Candle{
			"ETHEUR": {
				{Pair: "ETHEUR", Interval: "1d", Close: 3100.25},
			},
		},
	}

	resolver := NewPriceResolver(finder)
	ts := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

	res, err := resolver.Resolve(context.Background(), "ETH", "EUR", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Method != "daily" {
		t.Errorf("expected method %q, got %q", "daily", res.Method)
	}
	if res.Price != 3100.25 {
		t.Errorf("expected price %v, got %v", 3100.25, res.Price)
	}
}

func TestPriceResolver_Unresolvable(t *testing.T) {
	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{},
		daily:  map[string][]candle.Candle{},
	}

	resolver := NewPriceResolver(finder)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	res, err := resolver.Resolve(context.Background(), "DOGE", "USD", ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Method != "unresolvable" {
		t.Errorf("expected method %q, got %q", "unresolvable", res.Method)
	}
	if res.Price != 0 {
		t.Errorf("expected price 0, got %v", res.Price)
	}
}
