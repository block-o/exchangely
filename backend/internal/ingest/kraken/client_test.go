package kraken

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestFetchCandlesParsesKrakenOHLC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"error": [],
			"result": {
				"XBTEUR": [
					[1711929600, "59000.0", "59200.0", "58950.0", "59150.0", "59110.0", "21.5", 101],
					[1711933200, "59150.0", "59300.0", "59100.0", "59280.0", "59240.0", "18.2", 88]
				],
				"last": "1711933200"
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
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
	if items[0].Source != "kraken" || items[0].Close != 59150 {
		t.Fatalf("unexpected candle: %+v", items[0])
	}
}
