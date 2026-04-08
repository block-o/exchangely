package router_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	"github.com/block-o/exchangely/backend/internal/service"
)

type streamRepo struct {
	tickerValues map[string]ticker.Ticker
	tickers      []ticker.Ticker
	tickerCalls  int
	tickersCalls int
}

func (r *streamRepo) Historical(context.Context, string, string, time.Time, time.Time) ([]candle.Candle, error) {
	return nil, nil
}

func (r *streamRepo) Ticker(_ context.Context, pairSymbol string) (ticker.Ticker, error) {
	r.tickerCalls++
	return r.tickerValues[pairSymbol], nil
}

func (r *streamRepo) Tickers(context.Context) ([]ticker.Ticker, error) {
	r.tickersCalls++
	return append([]ticker.Ticker(nil), r.tickers...), nil
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
