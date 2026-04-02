package binancevision

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestFetchCandlesReadsMonthlyArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/monthly/klines/BTCUSDT/1h/") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeZipCSV(t, w, "BTCUSDT-1h-2024-01.csv", strings.Join([]string{
			"1704067200000,42283.58,42554.57,42261.02,42475.23,1271.68108,1704070799999,0,0,0,0,0",
			"1704070800000,42475.23,42775.00,42431.65,42613.56,1196.37856,1704074399999,0,0,0,0,0",
		}, "\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Unix(1704067200, 0).UTC(),
		EndTime:   time.Unix(1704074400, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(items))
	}
	if items[0].Source != "binancevision" || items[0].Close != 42475.23 {
		t.Fatalf("unexpected first candle: %+v", items[0])
	}
}

func TestFetchCandlesFallsBackToDailyArchives(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/monthly/klines/BTCUSDT/1h/"):
			http.NotFound(w, r)
		case strings.Contains(r.URL.Path, "BTCUSDT-1h-2024-01-01.zip"):
			writeZipCSV(t, w, "BTCUSDT-1h-2024-01-01.csv", "1704067200000,42283.58,42554.57,42261.02,42475.23,1271.68108,1704070799999,0,0,0,0,0")
		case strings.Contains(r.URL.Path, "BTCUSDT-1h-2024-01-02.zip"):
			writeZipCSV(t, w, "BTCUSDT-1h-2024-01-02.csv", "1704153600000000,42800.00,42900.00,42700.00,42850.00,999.0,1704157199999999,0,0,0,0,0")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	items, err := client.FetchCandles(context.Background(), ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Unix(1704067200, 0).UTC(),
		EndTime:   time.Unix(1704157200, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(items))
	}
	if items[1].Timestamp != 1704153600 {
		t.Fatalf("expected microsecond timestamp normalization, got %+v", items[1])
	}
}

func TestSupportsOnlyHistoricalWindows(t *testing.T) {
	client := NewClient("https://data.binance.vision", http.DefaultClient)
	client.now = func() time.Time {
		return time.Date(2026, 4, 2, 15, 0, 0, 0, time.UTC)
	}

	if !client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected historical window to be supported")
	}

	if client.Supports(ingest.Request{
		Pair:      "BTCUSDT",
		Base:      "BTC",
		Quote:     "USDT",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 1, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected current-day window to bypass binance vision")
	}
}

func writeZipCSV(t *testing.T, w http.ResponseWriter, name, content string) {
	t.Helper()

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	fileWriter, err := zipWriter.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := fileWriter.Write([]byte(content)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	w.Header().Set("Content-Type", "application/zip")
	_, _ = w.Write(buf.Bytes())
}
