package kraken

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestFetchCandlesParsesKrakenOHLC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/0/public/AssetPairs" {
			_, _ = w.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZEUR": {"altname":"XBTEUR","wsname":"XBT/EUR"}
				}
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"error": [],
			"result": {
				"XXBTZEUR": [
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

func TestFetchCandlesResolvesWsnamePairs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/0/public/AssetPairs" {
			_, _ = w.Write([]byte(`{
				"error": [],
				"result": {
					"XETHZEUR": {"altname":"ETHEUR","wsname":"ETH/EUR"},
					"XXRPZEUR": {"altname":"XRPEUR","wsname":"XRP/EUR"},
					"XDG/EUR": {"altname":"XDGEUR","wsname":"DOGE/EUR"}
				}
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"error": [],
			"result": {
				"XETHZEUR": [
					[1711929600, "3000.0", "3010.0", "2990.0", "3005.0", "3002.0", "10.5", 101]
				],
				"last": "1711929600"
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "ETHEUR",
		Base:      "ETH",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Unix(1711929600, 0).UTC(),
		EndTime:   time.Unix(1711933200, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(items))
	}
	if items[0].Close != 3005 {
		t.Fatalf("unexpected candle: %+v", items[0])
	}
}

func TestFetchCandlesEntersCooldownOnRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/0/public/AssetPairs" {
			_, _ = w.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZEUR": {"altname":"XBTEUR","wsname":"XBT/EUR"}
				}
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"error": ["EGeneral:Too many requests"],
			"result": {}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Unix(1711929600, 0).UTC(),
		EndTime:   time.Unix(1711933200, 0).UTC(),
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	_, err = client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Unix(1711929600, 0).UTC(),
		EndTime:   time.Unix(1711933200, 0).UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "cooldown active") {
		t.Fatalf("expected cooldown error, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected asset pair lookup and first ohlc call only, got %d requests", requests)
	}
}
