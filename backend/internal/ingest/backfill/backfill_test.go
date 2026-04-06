package backfill

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

func TestParsePairSymbol(t *testing.T) {
	base, quote, err := ParsePairSymbol("BTCUSD")
	if err != nil {
		t.Fatalf("parse pair symbol: %v", err)
	}
	if base != "BTC" || quote != "USD" {
		t.Fatalf("unexpected pair parse result: base=%s quote=%s", base, quote)
	}
}

func TestFetchCandlesReturnsFirstSuccessfulSource(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "first", items: []candle.Candle{{Pair: "BTCUSD"}}},
		fakeSource{name: "second", items: []candle.Candle{{Pair: "ETHUSD"}}},
	)

	request := Request{Pair: "BTCUSD", Base: "BTC", Quote: "USD", Interval: "1h"}
	items, err := registry.FetchCandles(context.Background(), request)
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	if len(items) != 1 || items[0].Pair != "BTCUSD" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestFetchCandlesReturnsErrNoSourceWhenUnsupported(t *testing.T) {
	registry := NewRegistry(fakeSource{name: "first", supports: false})

	_, err := registry.FetchCandles(context.Background(), Request{Pair: "ABCXYZ"})
	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("expected ErrNoSource, got %v", err)
	}
}

func TestFetchCandlesFallsThroughEmptyResults(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "first", items: nil},
		fakeSource{name: "second", items: []candle.Candle{{Pair: "BTCEUR"}}},
	)

	request := Request{Pair: "BTCEUR", Base: "BTC", Quote: "EUR", Interval: "1h"}
	items, err := registry.FetchCandles(context.Background(), request)
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	if len(items) != 1 || items[0].Pair != "BTCEUR" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestFetchCandlesIncludesErrNoDataWhenAllSourcesEmpty(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "first", supports: true, items: nil},
		fakeSource{name: "second", supports: true, items: nil},
	)

	request := Request{Pair: "BTCEUR", Base: "BTC", Quote: "EUR", Interval: "1h"}
	_, err := registry.FetchCandles(context.Background(), request)
	if !errors.Is(err, ErrNoData) {
		t.Fatalf("expected ErrNoData, got %v", err)
	}
}

func TestNewRegistryDropsNilSources(t *testing.T) {
	registry := NewRegistry(nil, fakeSource{name: "usable", supports: true, items: []candle.Candle{{Pair: "BTCUSD"}}})

	items, err := registry.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Base: "BTC", Quote: "USD", Interval: "1h"})
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	if len(items) != 1 || items[0].Pair != "BTCUSD" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestFetchCandlesReturnsJoinedErrorsAfterCompatibleFailures(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "first", supports: true, err: errors.New("timeout")},
		fakeSource{name: "second", supports: true, err: errors.New("boom")},
	)

	_, err := registry.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Base: "BTC", Quote: "USD", Interval: "1h"})
	if err == nil {
		t.Fatal("expected joined error when all compatible sources fail")
	}
	if !strings.Contains(err.Error(), "first") || !strings.Contains(err.Error(), "second") {
		t.Fatalf("expected source names in joined error, got %v", err)
	}
}

type fakeSource struct {
	name     string
	supports bool
	items    []candle.Candle
	err      error
}

func (f fakeSource) Name() string {
	return f.name
}

func (f fakeSource) Supports(_ Request) bool {
	return f.supports || f.items != nil || f.err != nil
}

func (f fakeSource) FetchCandles(_ context.Context, _ Request) ([]candle.Candle, error) {
	return f.items, f.err
}
