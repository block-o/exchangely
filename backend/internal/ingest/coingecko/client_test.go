package coingecko

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestFetchCandlesParsesMarketChartRangeSamples(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/coins/bitcoin/market_chart/range" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("vs_currency"); got != "usd" {
			t.Fatalf("expected usd vs_currency, got %q", got)
		}
		if got := r.Header.Get("x-cg-demo-api-key"); got != "demo-key" {
			t.Fatalf("expected api key header, got %q", got)
		}
		_, _ = w.Write([]byte(`{
			"prices": [
				[1711929720000, 60123.4],
				[1711931520000, 60300.1],
				[1711935120000, 60600.9]
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "demo-key", server.Client())
	client.now = func() time.Time {
		return time.Date(2024, 4, 1, 2, 30, 0, 0, time.UTC)
	}

	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCUSD",
		Base:      "BTC",
		Quote:     "USD",
		Interval:  "1h",
		StartTime: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 4, 1, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 raw samples, got %d", len(items))
	}
	if items[0].Timestamp != 1711929720 || items[2].Close != 60600.9 {
		t.Fatalf("unexpected samples: %+v", items)
	}
	if items[0].Source != "coingecko" {
		t.Fatalf("expected coingecko source, got %+v", items[0])
	}
}

func TestSupportsOnlyRealtimeEURAndUSDWindows(t *testing.T) {
	client := NewClient("", "", http.DefaultClient)
	client.now = func() time.Time {
		return time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	}

	if !client.Supports(ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected current-day EUR realtime window to be supported")
	}

	if !client.Supports(ingest.Request{
		Pair:      "BTCUSD",
		Base:      "BTC",
		Quote:     "USD",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected current-day USD realtime window to be supported")
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected USDT quote to be unsupported")
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1d",
		StartTime: time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected daily window to be unsupported")
	}
}
