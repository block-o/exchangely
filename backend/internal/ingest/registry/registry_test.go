package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestParsePairSymbol(t *testing.T) {
	base, quote, err := ParsePairSymbol("BTCEUR")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if base != "BTC" || quote != "EUR" {
		t.Fatalf("unexpected parsed pair: %s/%s", base, quote)
	}
}

func TestRegistryPrefersSupportingSource(t *testing.T) {
	request := ingest.Request{Pair: "BTCUSDT", Base: "BTC", Quote: "USDT", Interval: "1h"}
	registry := New(
		fakeSource{name: "unsupported"},
		fakeSource{
			name:       "supported",
			supports:   true,
			candleData: []candle.Candle{{Pair: "BTCUSDT", Interval: "1h", Timestamp: 1}},
		},
	)

	items, err := registry.FetchCandles(context.Background(), request)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 || items[0].Pair != "BTCUSDT" {
		t.Fatalf("unexpected candles: %+v", items)
	}
}

func TestRegistryReturnsNoSourceError(t *testing.T) {
	registry := New(fakeSource{name: "unsupported"})
	_, err := registry.FetchCandles(context.Background(), ingest.Request{Pair: "ABCXYZ"})
	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("expected ErrNoSource, got %v", err)
	}
}

func TestRegistrySkipsEmptyResultsAndFallsThrough(t *testing.T) {
	request := ingest.Request{Pair: "BTCEUR", Base: "BTC", Quote: "EUR", Interval: "1h"}
	registry := New(
		fakeSource{name: "empty", supports: true},
		fakeSource{
			name:       "supported",
			supports:   true,
			candleData: []candle.Candle{{Pair: "BTCEUR", Interval: "1h", Timestamp: 1}},
		},
	)

	items, err := registry.FetchCandles(context.Background(), request)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 || items[0].Pair != "BTCEUR" {
		t.Fatalf("unexpected candles: %+v", items)
	}
}

func TestRegistryReturnsNoDataWhenAllSourcesAreEmpty(t *testing.T) {
	request := ingest.Request{Pair: "BTCEUR", Base: "BTC", Quote: "EUR", Interval: "1h"}
	registry := New(
		fakeSource{name: "empty-a", supports: true},
		fakeSource{name: "empty-b", supports: true},
	)

	_, err := registry.FetchCandles(context.Background(), request)
	if !errors.Is(err, ErrNoData) {
		t.Fatalf("expected ErrNoData, got %v", err)
	}
}

type fakeSource struct {
	name       string
	supports   bool
	candleData []candle.Candle
	err        error
}

func (f fakeSource) Name() string {
	return f.name
}

func (f fakeSource) Supports(_ ingest.Request) bool {
	return f.supports
}

func (f fakeSource) FetchCandles(_ context.Context, _ ingest.Request) ([]candle.Candle, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.candleData, nil
}
