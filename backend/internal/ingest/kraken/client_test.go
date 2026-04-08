package kraken

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/ingest/backfill"
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
		if r.URL.Path == "/0/public/Ticker" {
			_, _ = w.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZEUR": {
						"v": ["10.0", "327000000.0"],
						"p": ["60000.0", "60000.0"]
					}
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
	client.now = func() time.Time {
		return time.Unix(1711933200, 0).UTC()
	}
	items, err := client.FetchCandles(context.Background(), backfill.Request{
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
	client.now = func() time.Time {
		return time.Unix(1711933200, 0).UTC()
	}
	items, err := client.FetchCandles(context.Background(), backfill.Request{
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
		if r.URL.Path == "/0/public/Ticker" {
			_, _ = w.Write([]byte(`{
				"error": ["EGeneral:Too many requests"],
				"result": {}
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
	client.now = func() time.Time {
		return time.Unix(1711933200, 0).UTC()
	}
	_, err := client.FetchCandles(context.Background(), backfill.Request{
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

	_, err = client.FetchCandles(context.Background(), backfill.Request{
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
	if requests != 3 {
		t.Fatalf("expected asset pair lookup, ticker, and ohlc calls, got %d requests", requests)
	}
}

func TestFetchCandlesEntersCooldownWhenAssetPairsRateLimited(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	client.now = func() time.Time {
		return time.Unix(1711933200, 0).UTC()
	}

	request := backfill.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Unix(1711929600, 0).UTC(),
		EndTime:   time.Unix(1711933200, 0).UTC(),
	}

	_, err := client.FetchCandles(context.Background(), request)
	if err == nil {
		t.Fatal("expected asset pairs rate limit error")
	}

	_, err = client.FetchCandles(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "cooldown active") {
		t.Fatalf("expected cooldown error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected only the first request to hit kraken, got %d requests", requests)
	}
}

func TestSupportsOnlyRecentWindows(t *testing.T) {
	client := NewClient("https://api.kraken.com", http.DefaultClient)
	client.now = func() time.Time {
		return time.Date(2026, 4, 2, 15, 0, 0, 0, time.UTC)
	}

	if !client.Supports(backfill.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Date(2026, 4, 1, 23, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 2, 1, 0, 0, 0, time.UTC),
	}) {
		t.Fatal("expected current-day window to be supported")
	}

	if client.Supports(backfill.Request{
		Pair:      "BTCEUR",
		Base:      "BTC",
		Quote:     "EUR",
		Interval:  "1h",
		StartTime: time.Unix(0, 0),
		EndTime:   time.Unix(0, 0),
	}) {
		t.Fatal("expected ancient historical window to be unsupported")
	}
}

func TestFetchCandlesUsesCorrectIntervalParameters(t *testing.T) {
	tests := []struct {
		interval string
		expected string
	}{
		{"1h", "60"},
		{"1d", "1440"},
	}

	for _, tt := range tests {
		t.Run(tt.interval, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/0/public/AssetPairs" {
					_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR","wsname":"XBT/EUR"}}}`))
					return
				}
				val := r.URL.Query().Get("interval")
				if val != tt.expected {
					t.Errorf("expected interval=%s, got %s", tt.expected, val)
				}
				_, _ = fmt.Fprintf(w, `{"error": [], "result": {"XXBTZEUR": [[%d, "1", "1", "1", "1", "1", "1", 1]]}}`, time.Now().Unix())
			}))
			defer server.Close()

			client := NewClient(server.URL, server.Client())
			client.now = func() time.Time { return time.Now().Add(24 * time.Hour) }
			_, _ = client.FetchCandles(context.Background(), backfill.Request{
				Pair:      "BTCEUR",
				Base:      "BTC",
				Quote:     "EUR",
				Interval:  tt.interval,
				StartTime: time.Now().Add(-2 * time.Hour),
				EndTime:   time.Now().Add(-1 * time.Hour),
			})
		})
	}
}

func TestKrakenInterval(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"1m", 1, false},
		{"1h", 60, false},
		{"1d", 1440, false},
		{"unsupported", 0, true},
	}
	for _, tt := range tests {
		got, err := krakenInterval(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("krakenInterval(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("krakenInterval(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestKrakenFloat(t *testing.T) {
	tests := []struct {
		input   any
		want    float64
		wantErr bool
	}{
		{"1.23", 1.23, false},
		{1.23, 1.23, false},
		{"invalid", 0, true},
		{true, 0, true}, // unsupported type
	}
	for _, tt := range tests {
		got, err := krakenFloat(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("krakenFloat(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("krakenFloat(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestKrakenInt(t *testing.T) {
	tests := []struct {
		input   any
		want    int64
		wantErr bool
	}{
		{123.45, 123, false},
		{int64(123), 123, false},
		{"123", 123, false},
		{"invalid", 0, true},
		{true, 0, true}, // unsupported type
	}
	for _, tt := range tests {
		got, err := krakenInt(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("krakenInt(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("krakenInt(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFetchCandlesFailures(t *testing.T) {
	t.Run("unsupported request", func(t *testing.T) {
		client := NewClient("", nil)
		_, err := client.FetchCandles(context.Background(), backfill.Request{Interval: "1m"})
		if err == nil || !strings.Contains(err.Error(), "unsupported request") {
			t.Errorf("expected unsupported request error, got %v", err)
		}
	})

	t.Run("api error 500", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "kraken status 500") {
			t.Errorf("expected 500 error, got %v", err)
		}
	})

	t.Run("kraken error payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"error": ["EGeneral:Internal Error"], "result": {}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "EGeneral:Internal Error") {
			t.Errorf("expected kraken error, got %v", err)
		}
	})

	t.Run("malformed ohlc rows", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": [[1711929600, "invalid"]]}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		items, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err != nil {
			t.Errorf("expected nil error (skipping malformed row), got %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected 0 items, got %d", len(items))
		}
	})
}

func TestLoadPairMapEdgeCases(t *testing.T) {
	t.Run("cache hit", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
		}))
		defer server.Close()

		client := NewClient(server.URL, server.Client())
		fixedNow := time.Now()
		client.now = func() time.Time { return fixedNow }

		// First call should hit server
		err := client.loadPairMap(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// Second call should hit cache (within 6h)
		err = client.loadPairMap(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if requests != 1 {
			t.Errorf("expected 1 request due to caching, got %d", requests)
		}
	})

	t.Run("api failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		err := client.loadPairMap(context.Background())
		if err == nil || !strings.Contains(err.Error(), "403") {
			t.Errorf("expected 403 error, got %v", err)
		}
	})

	t.Run("kraken error payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"error": ["EAPI:Invalid key"], "result": {}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		err := client.loadPairMap(context.Background())
		if err == nil || !strings.Contains(err.Error(), "EAPI:Invalid key") {
			t.Errorf("expected kraken error, got %v", err)
		}
	})
}

func TestFetchCandlesMoreFailures(t *testing.T) {
	t.Run("missing pair in result", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"error": [], "result": {"OTHER": []}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "kraken result missing pair") {
			t.Errorf("expected missing pair error, got %v", err)
		}
	})

	t.Run("malformed timestamp", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": [[true, "1", "1", "1", "1", "1", "1"]]}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "unsupported int value") {
			t.Errorf("expected timestamp error, got %v", err)
		}
	})

	t.Run("pair not mapped", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"error": [], "result": {"OTHER": {"altname":"OTHER"}}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "is not mapped for kraken") {
			t.Errorf("expected mapping error, got %v", err)
		}
	})

	t.Run("malformed open price", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": [[1711929600, true, "1", "1", "1", "1", "1"]]}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "unsupported float value") {
			t.Errorf("expected float error, got %v", err)
		}
	})

	t.Run("malformed results json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/0/public/AssetPairs" {
				_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": "not-an-array"}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		now := time.Now().UTC()
		client.now = func() time.Time { return now }
		_, err := client.FetchCandles(context.Background(), backfill.Request{Pair: "BTCEUR", Quote: "EUR", Interval: "1h", EndTime: now})
		if err == nil || !strings.Contains(err.Error(), "json: cannot unmarshal") {
			t.Errorf("expected unmarshal error, got %v", err)
		}
	})
}

func TestResolvePairCodeCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"error": [], "result": {"XXBTZEUR": {"altname":"XBTEUR"}}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	client.now = func() time.Time { return time.Now() }

	// First resolution triggers loadPairMap
	code, err := client.resolvePairCode(context.Background(), "BTCEUR")
	if err != nil || code != "XXBTZEUR" {
		t.Fatalf("first resolution failed: %v, %s", err, code)
	}

	// Second resolution should hit memory cache
	code, err = client.resolvePairCode(context.Background(), "BTCEUR")
	if err != nil || code != "XXBTZEUR" {
		t.Fatalf("second resolution failed: %v, %s", err, code)
	}

	if requests != 1 {
		t.Errorf("expected 1 request to server, got %d", requests)
	}
}

func TestLoadPairMapMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{invalid-json`))
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client())
	err := client.loadPairMap(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("expected json error, got %v", err)
	}
}
func TestFetchNativeV24H(t *testing.T) {
	t.Run("malformed json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		_, err := client.fetchNativeV24H(context.Background(), "XXBTZEUR")
		if err == nil {
			t.Error("expected error on malformed json")
		}
	})

	t.Run("missing pair", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"error": [], "result": {"OTHER": {}}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		_, err := client.fetchNativeV24H(context.Background(), "XXBTZEUR")
		if err == nil || !strings.Contains(err.Error(), "missing or invalid") {
			t.Errorf("expected missing pair error, got %v", err)
		}
	})

	t.Run("invalid volume format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"result": {"XXBTZEUR": {"v": ["1.0", "invalid"], "p": ["1.0", "1.0"]}}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		_, err := client.fetchNativeV24H(context.Background(), "XXBTZEUR")
		if err == nil {
			t.Error("expected error on invalid volume format")
		}
	})

	t.Run("invalid vwap format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"result": {"XXBTZEUR": {"v": ["1.0", "1.0"], "p": ["1.0", "invalid"]}}}`))
		}))
		defer server.Close()
		client := NewClient(server.URL, server.Client())
		_, err := client.fetchNativeV24H(context.Background(), "XXBTZEUR")
		if err == nil {
			t.Error("expected error on invalid vwap format")
		}
	})
}
