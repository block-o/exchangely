package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest/backfill"
)

func TestFetchCandlesParsesBinanceKlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/klines" {
			_, _ = w.Write([]byte(`[
				[1711929600000,"64000.0","64500.0","63900.0","64400.0","120.5"],
				[1711933200000,"64400.0","64650.0","64350.0","64600.0","98.1"]
			]`))
			return
		}
		if r.URL.Path == "/api/v3/ticker/24hr" {
			_, _ = w.Write([]byte(`{"quoteVolume": "1000000.0"}`))
			return
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	client.now = func() time.Time {
		return time.Unix(1711933200+3600, 0).UTC() // 1 hour after latest candle
	}
	items, err := client.FetchCandles(context.Background(), backfill.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Unix(1711929600, 0).UTC(),
		EndTime:   time.Unix(1711936800, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(items))
	}
	if items[1].Volume24H != 1000000 {
		t.Fatalf("expected 1000000 volume_24h, got %f", items[1].Volume24H)
	}
}

func TestFetchNativeV24H(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"quoteVolume": "123.45"}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		val, err := client.fetchNativeV24H(context.Background(), "BTCEUR")
		if err != nil || val != 123.45 {
			t.Errorf("expected 123.45, got %v (err: %v)", val, err)
		}
	})

	t.Run("rate limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		_, err := client.fetchNativeV24H(context.Background(), "BTCEUR")
		if err == nil || !strings.Contains(err.Error(), "rate limited") {
			t.Errorf("expected rate limited error, got %v", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		_, err := client.fetchNativeV24H(context.Background(), "BTCEUR")
		if err == nil {
			t.Error("expected error on malformed json")
		}
	})
}

func TestFetchCandlesEntersCooldownOnRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	client.now = func() time.Time {
		return time.Date(2024, 4, 1, 3, 0, 0, 0, time.UTC)
	}

	request := backfill.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 4, 1, 2, 0, 0, 0, time.UTC),
	}

	_, err := client.FetchCandles(context.Background(), request)
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	_, err = client.FetchCandles(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "cooldown active") {
		t.Fatalf("expected cooldown error, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected ticker and ohlc calls, got %d", requests)
	}
}

func TestSupportsOnlyRecentWindows(t *testing.T) {
	client := NewClient("https://api.binance.com", http.DefaultClient)
	client.now = func() time.Time {
		return time.Date(2026, 4, 2, 15, 0, 0, 0, time.UTC)
	}

	if client.Supports(backfill.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 1, 23, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 1, 0, 0, 0, time.UTC),
	}) == false {
		t.Fatal("expected current-day window to be supported")
	}

	if client.Supports(backfill.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Unix(0, 0),
		EndTime:   time.Unix(0, 0),
	}) {
		t.Fatal("expected ancient historical window to be unsupported")
	}
}
