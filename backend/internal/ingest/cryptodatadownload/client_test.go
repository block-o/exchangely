package cryptodatadownload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestFetchCandlesParsesHourlyCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cdd/Binance_BTCUSDT_1h.csv" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(strings.Join([]string{
			"Cryptodatadownload.com",
			"Unix,Date,Symbol,Open,High,Low,Close,Volume BTC,Volume USDT,tradecount",
			"1704070800000,2024-01-01 01:00:00,BTCUSDT,42500,42600,42400,42550,12.3,522000,1000",
			"1704067200000,2024-01-01 00:00:00,BTCUSDT,42400,42500,42300,42450,10.5,446000,900",
		}, "\n")))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(items))
	}
	if items[0].Timestamp != 1704067200 || items[1].Timestamp != 1704070800 {
		t.Fatalf("expected ascending timestamps, got %+v", items)
	}
	if items[0].Source != "cryptodatadownload" || items[0].Close != 42450 {
		t.Fatalf("unexpected candle: %+v", items[0])
	}
}

func TestFetchCandlesParsesDailyCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cdd/Bitfinex_BTCEUR_d.csv" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(strings.Join([]string{
			"Cryptodatadownload.com",
			"Unix,Date,Symbol,Open,High,Low,Close,Volume BTC,Volume EUR,tradecount",
			"1704067200000,2024-01-01,BTCEUR,41000,42000,40500,41800,5.25,214000,250",
		}, "\n")))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1d",
		StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 || items[0].Timestamp != 1704067200 || items[0].Volume != 5.25 {
		t.Fatalf("unexpected candles: %+v", items)
	}
}

func TestSupportsOnlyHistoricalHourlyAndDailyWindows(t *testing.T) {
	client := NewClient("", http.DefaultClient)
	client.now = func() time.Time { return time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC) }

	if !client.Supports(ingest.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected historical EUR window to be supported")
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected current-day window to be unsupported")
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1m",
		StartTime: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected unsupported interval to be rejected")
	}
}
