package service

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
)

type fakeMarketRepository struct {
	tickerValues map[string][]ticker.Ticker
	tickerCalls  map[string]int
	tickers      [][]ticker.Ticker
	tickersCalls int
}

func (f *fakeMarketRepository) Historical(_ context.Context, _, _ string, _, _ time.Time) ([]candle.Candle, error) {
	return nil, nil
}

func (f *fakeMarketRepository) Ticker(_ context.Context, pairSymbol string) (ticker.Ticker, error) {
	if f.tickerCalls == nil {
		f.tickerCalls = make(map[string]int)
	}
	call := f.tickerCalls[pairSymbol]
	f.tickerCalls[pairSymbol] = call + 1

	series := f.tickerValues[pairSymbol]
	if len(series) == 0 {
		return ticker.Ticker{Pair: pairSymbol}, nil
	}
	if call >= len(series) {
		call = len(series) - 1
	}
	return series[call], nil
}

func (f *fakeMarketRepository) Tickers(_ context.Context) ([]ticker.Ticker, error) {
	call := f.tickersCalls
	f.tickersCalls++
	if call >= len(f.tickers) {
		call = len(f.tickers) - 1
	}
	items := append([]ticker.Ticker(nil), f.tickers[call]...)
	return items, nil
}

func TestMarketServiceTickersUsesTTLCache(t *testing.T) {
	repo := &fakeMarketRepository{
		tickers: [][]ticker.Ticker{
			{{Pair: "BTCUSD", Price: 100}},
			{{Pair: "BTCUSD", Price: 101}},
		},
	}
	service := NewMarketService(repo, 10, time.Hour)

	first, err := service.Tickers(context.Background())
	if err != nil {
		t.Fatalf("first tickers call failed: %v", err)
	}
	second, err := service.Tickers(context.Background())
	if err != nil {
		t.Fatalf("second tickers call failed: %v", err)
	}

	if repo.tickersCalls != 1 {
		t.Fatalf("expected cached second call, repo calls=%d", repo.tickersCalls)
	}
	if got := second[0].Price; got != first[0].Price {
		t.Fatalf("expected cached price %v, got %v", first[0].Price, got)
	}
}

func TestMarketServiceNotifyUpdateKeepsGlobalTickersCacheWarm(t *testing.T) {
	repo := &fakeMarketRepository{
		tickers: [][]ticker.Ticker{
			{{Pair: "BTCUSD", Price: 100}},
			{{Pair: "BTCUSD", Price: 101}},
		},
	}
	service := NewMarketService(repo, 10, time.Hour)

	first, err := service.Tickers(context.Background())
	if err != nil {
		t.Fatalf("first tickers call failed: %v", err)
	}

	service.NotifyUpdate("BTCUSD")

	second, err := service.Tickers(context.Background())
	if err != nil {
		t.Fatalf("second tickers call failed: %v", err)
	}

	if repo.tickersCalls != 1 {
		t.Fatalf("expected global cache to remain valid, repo calls=%d", repo.tickersCalls)
	}
	if got := second[0].Price; got != first[0].Price {
		t.Fatalf("expected cached price %v after notify, got %v", first[0].Price, got)
	}
}

func TestMarketServiceNotifyUpdateInvalidatesSingleTickerCache(t *testing.T) {
	repo := &fakeMarketRepository{
		tickerValues: map[string][]ticker.Ticker{
			"BTCUSD": {
				{Pair: "BTCUSD", Price: 100},
				{Pair: "BTCUSD", Price: 101},
			},
		},
	}
	service := NewMarketService(repo, 10, time.Hour)

	first, err := service.Ticker(context.Background(), "BTCUSD")
	if err != nil {
		t.Fatalf("first ticker call failed: %v", err)
	}
	service.NotifyUpdate("BTCUSD")
	second, err := service.Ticker(context.Background(), "BTCUSD")
	if err != nil {
		t.Fatalf("second ticker call failed: %v", err)
	}

	if got := repo.tickerCalls["BTCUSD"]; got != 2 {
		t.Fatalf("expected single ticker cache refresh, ticker calls=%d", got)
	}
	if second.Price == first.Price {
		t.Fatalf("expected refreshed single ticker price after notify, got %v", second.Price)
	}
}

func TestMarketSubscriptionDrainsDistinctPendingPairs(t *testing.T) {
	service := NewMarketService(&fakeMarketRepository{}, 10, time.Hour)
	sub := service.Subscribe()
	defer service.Unsubscribe(sub)

	service.NotifyUpdate("ETHUSD")
	service.NotifyUpdate("BTCUSD")
	service.NotifyUpdate("BTCUSD")

	select {
	case <-sub.Updates():
	case <-time.After(time.Second):
		t.Fatal("expected subscription signal")
	}

	pairs := sub.DrainPendingPairs()
	if len(pairs) != 2 {
		t.Fatalf("expected 2 distinct pairs, got %v", pairs)
	}
	if pairs[0] != "BTCUSD" || pairs[1] != "ETHUSD" {
		t.Fatalf("expected sorted pending pairs, got %v", pairs)
	}

	if drained := sub.DrainPendingPairs(); len(drained) != 0 {
		t.Fatalf("expected pending pairs to clear after drain, got %v", drained)
	}
}
