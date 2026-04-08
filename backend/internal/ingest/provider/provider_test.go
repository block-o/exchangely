package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"pgregory.net/rapid"
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

func TestPropertyRegistryFirstSuccessSemantics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numSources := rapid.IntRange(1, 5).Draw(t, "numSources")

		sources := make([]Source, 0, numSources)
		var firstSuccessIdx = -1

		for i := 0; i < numSources; i++ {
			// 0=success, 1=error, 2=empty
			outcome := rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("outcome%d", i))

			switch outcome {
			case 0: // success — returns candles
				if firstSuccessIdx == -1 {
					firstSuccessIdx = i
				}
				sources = append(sources, fakeSource{
					name:  fmt.Sprintf("source%d", i),
					items: []candle.Candle{{Pair: fmt.Sprintf("FROM_SOURCE_%d", i)}},
				})
			case 1: // error
				sources = append(sources, fakeSource{
					name: fmt.Sprintf("source%d", i),
					err:  fmt.Errorf("source%d failed", i),
				})
			case 2: // empty — returns nil candles, no error
				sources = append(sources, fakeSource{
					name:  fmt.Sprintf("source%d", i),
					items: nil,
				})
			}
		}

		registry := NewRegistry(sources...)
		items, err := registry.FetchCandles(context.Background(), Request{
			Pair: "TEST", Base: "T", Quote: "EST", Interval: "1h",
		})

		if firstSuccessIdx >= 0 {
			// Should return candles from the first successful source
			if err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			expectedPair := fmt.Sprintf("FROM_SOURCE_%d", firstSuccessIdx)
			if len(items) != 1 || items[0].Pair != expectedPair {
				t.Fatalf("expected candles from source%d (pair=%s), got %+v",
					firstSuccessIdx, expectedPair, items)
			}
		} else {
			// No successful source — should return an error
			if err == nil {
				t.Fatal("expected error when no source succeeds")
			}
		}
	})
}

type fakeSource struct {
	name     string
	supports bool
	cap      Capability
	items    []candle.Candle
	err      error
}

func (f fakeSource) Name() string {
	return f.name
}

func (f fakeSource) Capabilities() Capability {
	if f.cap != 0 {
		return f.cap
	}
	return CapHistorical | CapRealtime
}

func (f fakeSource) Supports(_ Request) bool {
	return f.supports || f.items != nil || f.err != nil
}

func (f fakeSource) FetchCandles(_ context.Context, _ Request) ([]candle.Candle, error) {
	return f.items, f.err
}

func TestWithCapabilityFiltersHistoricalOnly(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "archive", cap: CapHistorical, items: []candle.Candle{{Pair: "BTCUSD", Source: "archive"}}},
		fakeSource{name: "live", cap: CapRealtime, items: []candle.Candle{{Pair: "BTCUSD", Source: "live"}}},
	)

	filtered := registry.WithCapability(CapHistorical)
	items, err := filtered.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Interval: "1h"})
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	if len(items) != 1 || items[0].Source != "archive" {
		t.Fatalf("expected archive source, got %+v", items)
	}
}

func TestWithCapabilityFiltersRealtimeOnly(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "archive", cap: CapHistorical, items: []candle.Candle{{Pair: "BTCUSD", Source: "archive"}}},
		fakeSource{name: "live", cap: CapRealtime, items: []candle.Candle{{Pair: "BTCUSD", Source: "live"}}},
	)

	filtered := registry.WithCapability(CapRealtime)
	items, err := filtered.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Interval: "1h"})
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	if len(items) != 1 || items[0].Source != "live" {
		t.Fatalf("expected live source, got %+v", items)
	}
}

func TestWithCapabilityIncludesDualCapSources(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "archive-only", cap: CapHistorical, items: nil, supports: true},
		fakeSource{name: "dual", cap: CapHistorical | CapRealtime, items: []candle.Candle{{Pair: "BTCUSD", Source: "dual"}}},
	)

	filtered := registry.WithCapability(CapRealtime)
	items, err := filtered.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Interval: "1h"})
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	if len(items) != 1 || items[0].Source != "dual" {
		t.Fatalf("expected dual source, got %+v", items)
	}
}

func TestWithCapabilityReturnsErrNoSourceWhenNoneMatch(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "archive", cap: CapHistorical, items: []candle.Candle{{Pair: "BTCUSD"}}},
	)

	filtered := registry.WithCapability(CapRealtime)
	_, err := filtered.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Interval: "1h"})
	if !errors.Is(err, ErrNoSource) {
		t.Fatalf("expected ErrNoSource, got %v", err)
	}
}

func TestUnfilteredRegistryIgnoresCapabilities(t *testing.T) {
	registry := NewRegistry(
		fakeSource{name: "archive", cap: CapHistorical, items: []candle.Candle{{Pair: "BTCUSD", Source: "archive"}}},
		fakeSource{name: "live", cap: CapRealtime, items: []candle.Candle{{Pair: "BTCUSD", Source: "live"}}},
	)

	items, err := registry.FetchCandles(context.Background(), Request{Pair: "BTCUSD", Interval: "1h"})
	if err != nil {
		t.Fatalf("fetch candles: %v", err)
	}
	// Unfiltered registry should return the first source that works (archive).
	if len(items) != 1 || items[0].Source != "archive" {
		t.Fatalf("expected first matching source, got %+v", items)
	}
}
