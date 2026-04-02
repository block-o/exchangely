package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
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
