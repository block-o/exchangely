package router_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	"github.com/block-o/exchangely/backend/internal/service"
)

type streamRepo struct {
	mu           sync.Mutex
	tickerValues map[string]ticker.Ticker
	tickers      []ticker.Ticker
	tickerCalls  int
	tickersCalls int
}

func (r *streamRepo) Historical(context.Context, string, string, time.Time, time.Time) ([]candle.Candle, error) {
	return nil, nil
}

func (r *streamRepo) Ticker(_ context.Context, pairSymbol string) (ticker.Ticker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickerCalls++
	return r.tickerValues[pairSymbol], nil
}

func (r *streamRepo) Tickers(context.Context) ([]ticker.Ticker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickersCalls++
	return append([]ticker.Ticker(nil), r.tickers...), nil
}

func (r *streamRepo) setTicker(pair string, t ticker.Ticker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickerValues[pair] = t
}

func TestTickerStreamEmitsDeltaTickersOnly(t *testing.T) {
	repo := &streamRepo{
		tickers: []ticker.Ticker{
			{Pair: "BTCEUR", Price: 50000},
			{Pair: "ETHEUR", Price: 3000},
		},
		tickerValues: map[string]ticker.Ticker{
			"BTCEUR": {Pair: "BTCEUR", Price: 50100, Source: "stream", LastUpdateUnix: 1711933200},
		},
	}
	nr := &noopRepo{}
	market := service.NewMarketService(repo, 100, 30*time.Second)
	handler := router.New(router.Services{
		Catalog: service.NewCatalogService(nr, nil),
		Market:  market,
		System:  service.NewSystemService(nr, nr, nr, nr, nr, nr, "leader", 0),
		News:    service.NewNewsService(nr),
	}, router.Options{})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/tickers/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		market.NotifyUpdate("BTCEUR")
	}()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read sse line: %v", err)
	}

	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected sse data line, got %q", line)
	}

	payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
	var event struct {
		Tickers []ticker.Ticker `json:"tickers"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatalf("unmarshal sse payload: %v", err)
	}

	if len(event.Tickers) != 1 {
		t.Fatalf("expected one ticker delta, got %+v", event.Tickers)
	}
	if got := event.Tickers[0].Pair; got != "BTCEUR" {
		t.Fatalf("expected BTCEUR delta, got %q", got)
	}
	if got := event.Tickers[0].Price; got != 50100 {
		t.Fatalf("expected refreshed price 50100, got %v", got)
	}
	if repo.tickersCalls != 0 {
		t.Fatalf("expected stream to avoid global ticker snapshots, repo Tickers calls=%d", repo.tickersCalls)
	}
	if repo.tickerCalls != 1 {
		t.Fatalf("expected one per-pair ticker lookup, got %d", repo.tickerCalls)
	}
}

func TestTickerStreamSuppressesDuplicateEvents(t *testing.T) {
	repo := &streamRepo{
		tickerValues: map[string]ticker.Ticker{
			"BTCEUR": {Pair: "BTCEUR", Price: 50000, Source: "binance", LastUpdateUnix: 1711929600},
		},
	}
	nr := &noopRepo{}
	market := service.NewMarketService(repo, 100, 30*time.Second)
	handler := router.New(router.Services{
		Catalog: service.NewCatalogService(nr, nil),
		Market:  market,
		System:  service.NewSystemService(nr, nr, nr, nr, nr, nr, "leader", 0),
		News:    service.NewNewsService(nr),
	}, router.Options{})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/tickers/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Helper: read the next SSE data payload, skipping blank lines.
	readNextPayload := func() string {
		t.Helper()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				return strings.TrimPrefix(line, "data: ")
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error: %v", err)
		}
		t.Fatal("stream ended without data line")
		return ""
	}

	// First notify — should emit because no previous state.
	go func() {
		time.Sleep(50 * time.Millisecond)
		market.NotifyUpdate("BTCEUR")
	}()

	payload1 := readNextPayload()
	var event1 struct {
		Tickers []ticker.Ticker `json:"tickers"`
	}
	if err := json.Unmarshal([]byte(payload1), &event1); err != nil {
		t.Fatalf("unmarshal first payload: %v", err)
	}
	if len(event1.Tickers) != 1 || event1.Tickers[0].Price != 50000 {
		t.Fatalf("expected initial ticker with price 50000, got %+v", event1.Tickers)
	}

	// Second notify with identical ticker — should be suppressed.
	// Then a third with a changed price to prove the stream is still alive.
	go func() {
		time.Sleep(50 * time.Millisecond)
		market.NotifyUpdate("BTCEUR") // duplicate — suppressed
		time.Sleep(100 * time.Millisecond)
		repo.setTicker("BTCEUR", ticker.Ticker{Pair: "BTCEUR", Price: 50100, Source: "binance", LastUpdateUnix: 1711933200})
		market.NotifyUpdate("BTCEUR") // changed — should emit
	}()

	payload2 := readNextPayload()
	var event2 struct {
		Tickers []ticker.Ticker `json:"tickers"`
	}
	if err := json.Unmarshal([]byte(payload2), &event2); err != nil {
		t.Fatalf("unmarshal second payload: %v", err)
	}

	// The next event must be the changed price (50100), not the duplicate (50000).
	if len(event2.Tickers) != 1 || event2.Tickers[0].Price != 50100 {
		t.Fatalf("expected changed ticker with price 50100, got %+v", event2.Tickers)
	}
}

func TestTickerStreamFiltersbyQuoteCurrency(t *testing.T) {
	repo := &streamRepo{
		tickerValues: map[string]ticker.Ticker{
			"BTCEUR": {Pair: "BTCEUR", Price: 50000, Source: "binance", LastUpdateUnix: 1711929600},
			"BTCUSD": {Pair: "BTCUSD", Price: 55000, Source: "binance", LastUpdateUnix: 1711929600},
		},
	}
	nr := &noopRepo{}
	market := service.NewMarketService(repo, 100, 30*time.Second)
	handler := router.New(router.Services{
		Catalog: service.NewCatalogService(nr, nil),
		Market:  market,
		System:  service.NewSystemService(nr, nr, nr, nr, nr, nr, "leader", 0),
		News:    service.NewNewsService(nr),
	}, router.Options{})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Connect with ?quote=EUR — should only receive EUR pairs.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/tickers/stream?quote=EUR", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Notify both EUR and USD pairs.
	go func() {
		time.Sleep(50 * time.Millisecond)
		market.NotifyUpdate("BTCUSD")
		market.NotifyUpdate("BTCEUR")
	}()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		var event struct {
			Tickers []ticker.Ticker `json:"tickers"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatalf("unmarshal sse payload: %v", err)
		}

		// Only EUR pairs should be present.
		for _, tk := range event.Tickers {
			if !strings.HasSuffix(tk.Pair, "EUR") {
				t.Fatalf("expected only EUR pairs, got %q", tk.Pair)
			}
		}
		if len(event.Tickers) == 1 && event.Tickers[0].Pair == "BTCEUR" {
			return // success
		}
		t.Fatalf("unexpected tickers: %+v", event.Tickers)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	t.Fatal("stream ended without receiving filtered EUR ticker")
}
