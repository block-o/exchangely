package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestFetchCandlesParsesBinanceKlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			[1711929600000,"64000.0","64500.0","63900.0","64400.0","120.5"],
			[1711933200000,"64400.0","64650.0","64350.0","64600.0","98.1"]
		]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	client.now = func() time.Time {
		return time.Date(2024, 4, 1, 3, 0, 0, 0, time.UTC)
	}
	items, err := client.FetchCandles(context.Background(), ingest.Request{
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
	if items[0].Source != "binance" || items[0].Close != 64400 {
		t.Fatalf("unexpected candle: %+v", items[0])
	}
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

	request := ingest.Request{
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
	if requests != 1 {
		t.Fatalf("expected only the first request to hit binance, got %d", requests)
	}
}

func TestSupportsOnlyRecentWindows(t *testing.T) {
	client := NewClient("https://api.binance.com", http.DefaultClient)
	client.now = func() time.Time {
		return time.Date(2026, 4, 2, 15, 0, 0, 0, time.UTC)
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 1, 23, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 1, 0, 0, 0, time.UTC),
	}) == false {
		t.Fatal("expected current-day window to be supported")
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected fully historical window to bypass live binance")
	}
}
