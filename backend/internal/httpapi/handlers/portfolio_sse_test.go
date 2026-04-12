package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/block-o/exchangely/backend/internal/auth"
	candleDomain "github.com/block-o/exchangely/backend/internal/domain/candle"
	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	tickerDomain "github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/block-o/exchangely/backend/internal/portfolio"
	"github.com/block-o/exchangely/backend/internal/service"
)

// --- Mocks for SSE handler tests ---

type sseHoldingRepo struct {
	holdings []domain.Holding
}

func (r *sseHoldingRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]domain.Holding, error) {
	return r.holdings, nil
}
func (r *sseHoldingRepo) Create(_ context.Context, _ *domain.Holding) error { return nil }
func (r *sseHoldingRepo) Update(_ context.Context, _ *domain.Holding) error { return nil }
func (r *sseHoldingRepo) Delete(_ context.Context, _, _ uuid.UUID) error    { return nil }
func (r *sseHoldingRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.Holding, error) {
	return nil, nil
}
func (r *sseHoldingRepo) UpsertBySource(_ context.Context, _ uuid.UUID, _, _ string, _ []domain.Holding) error {
	return nil
}
func (r *sseHoldingRepo) DeleteBySourceRef(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *sseHoldingRepo) DeleteBySource(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

type sseMarketRepo struct {
	prices map[string]float64
}

func (m *sseMarketRepo) Historical(_ context.Context, _, _ string, _, _ time.Time) ([]candleDomain.Candle, error) {
	return nil, nil
}

func (m *sseMarketRepo) Ticker(_ context.Context, pairSymbol string) (tickerDomain.Ticker, error) {
	p, ok := m.prices[pairSymbol]
	if !ok {
		return tickerDomain.Ticker{}, fmt.Errorf("no ticker for %s", pairSymbol)
	}
	return tickerDomain.Ticker{Pair: pairSymbol, Price: p}, nil
}

func (m *sseMarketRepo) Tickers(_ context.Context) ([]tickerDomain.Ticker, error) {
	return nil, nil
}

func (m *sseMarketRepo) TickersWithSparklines(_ context.Context) ([]tickerDomain.TickerWithSparkline, error) {
	return nil, nil
}

// --- Test helpers ---

func newSSETestSetup(holdings []domain.Holding, prices map[string]float64) (
	*PortfolioHandler,
	*service.MarketService,
) {
	holdingRepo := &sseHoldingRepo{holdings: holdings}
	marketRepo := &sseMarketRepo{prices: prices}
	marketAdapter := service.NewMarketDataAdapter(marketRepo)
	ve := portfolio.NewValuationEngine(marketAdapter, holdingRepo)

	svc := portfolio.NewPortfolioService(
		holdingRepo, nil, nil, nil, nil, nil, nil, nil,
	)

	marketSvc := service.NewMarketService(marketRepo, 100, 5*time.Second)
	ph := NewPortfolioHandler(svc, ve).WithMarketService(marketSvc)

	return ph, marketSvc
}

func sseAuthenticatedRequest(method, url string, userID uuid.UUID, expiry time.Duration) *http.Request {
	r := httptest.NewRequest(method, url, nil)
	claims := &auth.Claims{
		Sub:   userID.String(),
		Email: "test@example.com",
		Role:  "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
		},
	}
	ctx := middleware.ContextWithClaims(r.Context(), claims)
	ctx = middleware.ContextWithAuthMethod(ctx, "jwt")
	return r.WithContext(ctx)
}

// --- Unit tests ---

// TestSSEInitialSnapshotDelivery verifies that connecting to the portfolio SSE
// stream immediately delivers an initial portfolio valuation snapshot.
func TestSSEInitialSnapshotDelivery(t *testing.T) {
	userID := uuid.New()
	holdings := []domain.Holding{
		{
			ID:            uuid.New(),
			UserID:        userID,
			AssetSymbol:   "BTC",
			Quantity:      1.5,
			QuoteCurrency: "USD",
			Source:        "manual",
		},
		{
			ID:            uuid.New(),
			UserID:        userID,
			AssetSymbol:   "ETH",
			Quantity:      10.0,
			QuoteCurrency: "USD",
			Source:        "manual",
		},
	}
	prices := map[string]float64{
		"BTCUSD": 50000.0,
		"ETHUSD": 3000.0,
	}

	ph, _ := newSSETestSetup(holdings, prices)

	r := sseAuthenticatedRequest(http.MethodGet, "/api/v1/portfolio/stream", userID, 15*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
	defer cancel()
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	ph.StreamPortfolio(w, r)

	resp := w.Result()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: portfolio") {
		t.Fatalf("expected SSE event with 'event: portfolio', got:\n%s", body)
	}

	// Parse the data payload from the SSE event.
	lines := strings.Split(body, "\n")
	var dataLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if dataLine == "" {
		t.Fatalf("no data line found in SSE response:\n%s", body)
	}

	var val domain.Valuation
	if err := json.Unmarshal([]byte(dataLine), &val); err != nil {
		t.Fatalf("failed to parse valuation JSON: %v\ndata: %s", err, dataLine)
	}

	expectedTotal := 1.5*50000.0 + 10.0*3000.0
	if val.TotalValue != expectedTotal {
		t.Fatalf("expected total value %f, got %f", expectedTotal, val.TotalValue)
	}
	if val.QuoteCurrency != "USD" {
		t.Fatalf("expected quote currency USD, got %s", val.QuoteCurrency)
	}
	if len(val.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(val.Assets))
	}
}

// TestSSEEventEmissionOnHeldAssetPriceChange verifies that when a held asset's
// price changes, a new portfolio valuation event is pushed.
func TestSSEEventEmissionOnHeldAssetPriceChange(t *testing.T) {
	userID := uuid.New()
	holdings := []domain.Holding{
		{
			ID:            uuid.New(),
			UserID:        userID,
			AssetSymbol:   "BTC",
			Quantity:      2.0,
			QuoteCurrency: "USD",
			Source:        "manual",
		},
	}
	prices := map[string]float64{
		"BTCUSD": 40000.0,
	}

	ph, marketSvc := newSSETestSetup(holdings, prices)

	r := sseAuthenticatedRequest(http.MethodGet, "/api/v1/portfolio/stream", userID, 15*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ph.StreamPortfolio(w, r)
		close(done)
	}()

	// Give the handler time to subscribe and send initial snapshot.
	time.Sleep(100 * time.Millisecond)

	// Simulate a ticker update for a held asset.
	marketSvc.NotifyUpdate("BTCUSD")

	// Wait for the handler to process the update.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()

	// Count the number of "event: portfolio" occurrences.
	eventCount := strings.Count(body, "event: portfolio")
	if eventCount < 2 {
		t.Fatalf("expected at least 2 portfolio events (initial + update), got %d\nbody:\n%s", eventCount, body)
	}
}

// TestSSEConnectionClosureOnJWTExpiry verifies that the SSE connection closes
// when the JWT session expires.
func TestSSEConnectionClosureOnJWTExpiry(t *testing.T) {
	userID := uuid.New()
	holdings := []domain.Holding{
		{
			ID:            uuid.New(),
			UserID:        userID,
			AssetSymbol:   "BTC",
			Quantity:      1.0,
			QuoteCurrency: "USD",
			Source:        "manual",
		},
	}
	prices := map[string]float64{
		"BTCUSD": 50000.0,
	}

	ph, _ := newSSETestSetup(holdings, prices)

	// Create a request with a JWT that expires in 2 seconds.
	r := sseAuthenticatedRequest(http.MethodGet, "/api/v1/portfolio/stream", userID, 2*time.Second)

	// Use a longer context timeout so the JWT expiry is what closes the connection.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()

	start := time.Now()
	ph.StreamPortfolio(w, r)
	elapsed := time.Since(start)

	// The handler should have exited due to JWT expiry (~2s), not the 10s context timeout.
	if elapsed > 5*time.Second {
		t.Fatalf("handler took %v to exit, expected ~2s (JWT expiry)", elapsed)
	}

	// Verify initial snapshot was delivered before closure.
	body := w.Body.String()
	if !strings.Contains(body, "event: portfolio") {
		t.Fatalf("expected initial portfolio event before JWT expiry, got:\n%s", body)
	}
}
