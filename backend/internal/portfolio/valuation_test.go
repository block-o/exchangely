package portfolio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// --- Mock: MarketDataProvider ---

type mockMarketDataProvider struct {
	// prices maps "SYMBOL/QUOTE" to price. Missing key means price unavailable.
	prices map[string]float64
	// candles maps "SYMBOL/QUOTE" to a slice of candles.
	candles map[string][]candle.Candle
}

func (m *mockMarketDataProvider) GetTickerPrice(_ context.Context, pair string) (float64, error) {
	p, ok := m.prices[pair]
	if !ok {
		return 0, fmt.Errorf("price unavailable for %s", pair)
	}
	return p, nil
}

func (m *mockMarketDataProvider) GetCandles(_ context.Context, pair, interval string, start, end time.Time) ([]candle.Candle, error) {
	c, ok := m.candles[pair]
	if !ok {
		return nil, fmt.Errorf("no candles for %s", pair)
	}
	return c, nil
}

// --- Minimal holding repo for valuation tests ---

type valuationHoldingRepo struct {
	holdings []domain.Holding
}

func (r *valuationHoldingRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]domain.Holding, error) {
	return r.holdings, nil
}

func (r *valuationHoldingRepo) Create(_ context.Context, _ *domain.Holding) error { return nil }
func (r *valuationHoldingRepo) Update(_ context.Context, _ *domain.Holding) error { return nil }
func (r *valuationHoldingRepo) Delete(_ context.Context, _, _ uuid.UUID) error    { return nil }
func (r *valuationHoldingRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.Holding, error) {
	return nil, nil
}
func (r *valuationHoldingRepo) UpsertBySource(_ context.Context, _ uuid.UUID, _, _ string, _ []domain.Holding) error {
	return nil
}
func (r *valuationHoldingRepo) DeleteBySourceRef(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *valuationHoldingRepo) DeleteBySource(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *valuationHoldingRepo) ListDistinctUserIDs(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}

// --- Rapid generators for valuation tests ---

func genPrice(t *rapid.T, label string) float64 {
	return rapid.Float64Range(0.01, 100_000.0).Draw(t, label)
}

func genQuantity(t *rapid.T, label string) float64 {
	return rapid.Float64Range(0.0001, 10_000.0).Draw(t, label)
}

// Feature: portfolio-tracker, Property 13: Valuation invariants
//
// Validates: Requirements 4.1, 4.2, 4.5, 4.6
//
// For any portfolio with priced and unpriced holdings, the ValuationEngine
// satisfies: (a) total value = sum of (qty * price) for priced holdings,
// (b) allocation percentages sum to 100% for priced holdings,
// (c) unpriced holdings are excluded from total and flagged with priced=false.
func TestPropertyValuationInvariants(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := uuid.New()
		quoteCurrency := "USD"

		// Use unique symbols so each holding gets a distinct, known price.
		pricedPool := []string{"BTC", "ETH", "SOL", "USDT", "USDC", "ADA", "DOT", "AVAX"}
		maxPriced := len(pricedPool)
		numPriced := rapid.IntRange(1, maxPriced).Draw(t, "numPriced")
		numUnpriced := rapid.IntRange(0, 4).Draw(t, "numUnpriced")

		var holdings []domain.Holding
		prices := make(map[string]float64)
		expectedTotal := 0.0

		// Generate priced holdings with unique symbols and known prices.
		for i := 0; i < numPriced; i++ {
			sym := pricedPool[i]
			qty := genQuantity(t, fmt.Sprintf("pricedQty_%d", i))
			price := genPrice(t, fmt.Sprintf("price_%d", i))

			pair := sym + "/" + quoteCurrency
			prices[pair] = price
			expectedTotal += qty * price

			holdings = append(holdings, domain.Holding{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   sym,
				Quantity:      qty,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			})
		}

		// Generate unpriced holdings (no price in the mock).
		unpricedSymbols := []string{"XUNK1", "XUNK2", "XUNK3", "XUNK4"}
		for i := 0; i < numUnpriced; i++ {
			qty := genQuantity(t, fmt.Sprintf("unpricedQty_%d", i))
			holdings = append(holdings, domain.Holding{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   unpricedSymbols[i],
				Quantity:      qty,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			})
		}

		repo := &valuationHoldingRepo{holdings: holdings}
		market := &mockMarketDataProvider{prices: prices}
		engine := NewValuationEngine(market, repo)

		val, err := engine.ComputeValuation(context.Background(), userID, quoteCurrency)
		if err != nil {
			t.Fatalf("ComputeValuation failed: %v", err)
		}

		// (a) Total value = sum of (qty * price) for priced holdings.
		if math.Abs(val.TotalValue-expectedTotal) > 1e-6 {
			t.Fatalf("total value mismatch: got %f, want %f", val.TotalValue, expectedTotal)
		}

		// (b) Allocation percentages sum to 100% for priced holdings.
		var allocSum float64
		pricedCount := 0
		unpricedCount := 0
		for _, a := range val.Assets {
			if a.Priced {
				allocSum += a.AllocationPct
				pricedCount++
			} else {
				unpricedCount++
			}
		}
		if pricedCount > 0 {
			if math.Abs(allocSum-100.0) > 0.02 {
				t.Fatalf("allocation sum for priced holdings: got %.4f, want 100.00", allocSum)
			}
		}

		// (c) Unpriced holdings are excluded and flagged.
		if unpricedCount != numUnpriced {
			t.Fatalf("expected %d unpriced holdings, got %d", numUnpriced, unpricedCount)
		}
		for _, a := range val.Assets {
			if !a.Priced {
				if a.CurrentValue != 0 {
					t.Fatalf("unpriced holding %s has non-zero current value: %f", a.AssetSymbol, a.CurrentValue)
				}
				if a.AllocationPct != 0 {
					t.Fatalf("unpriced holding %s has non-zero allocation: %f", a.AssetSymbol, a.AllocationPct)
				}
			}
		}
	})
}

// Feature: portfolio-tracker, Property 14: P&L computation correctness
//
// Validates: Requirements 4.3, 4.4
//
// For any holding with an avg buy price, unrealized P&L = (current - avg) * qty.
// For any holding without an avg buy price, unrealized P&L is nil.
func TestPropertyPnLComputationCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := uuid.New()
		quoteCurrency := "USD"

		// Use unique symbols to avoid price map collisions.
		withAvgPool := []string{"BTC", "ETH", "SOL", "ADA", "DOT", "AVAX"}
		withoutAvgPool := []string{"USDT", "USDC", "LINK", "MATIC"}
		numWithAvg := rapid.IntRange(1, len(withAvgPool)).Draw(t, "numWithAvg")
		numWithoutAvg := rapid.IntRange(0, len(withoutAvgPool)).Draw(t, "numWithoutAvg")

		var holdings []domain.Holding
		prices := make(map[string]float64)

		type expectedPnL struct {
			symbol string
			pnl    float64
			hasAvg bool
		}
		var expectations []expectedPnL

		// Holdings with avg buy price (unique symbols).
		for i := 0; i < numWithAvg; i++ {
			sym := withAvgPool[i]
			qty := genQuantity(t, fmt.Sprintf("avgQty_%d", i))
			currentPrice := genPrice(t, fmt.Sprintf("avgCurPrice_%d", i))
			avgBuyPrice := genPrice(t, fmt.Sprintf("avgBuyPrice_%d", i))

			pair := sym + "/" + quoteCurrency
			prices[pair] = currentPrice

			avg := avgBuyPrice
			holdings = append(holdings, domain.Holding{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   sym,
				Quantity:      qty,
				AvgBuyPrice:   &avg,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			})

			expectations = append(expectations, expectedPnL{
				symbol: sym,
				pnl:    (currentPrice - avgBuyPrice) * qty,
				hasAvg: true,
			})
		}

		// Holdings without avg buy price (unique symbols, no overlap with above).
		for i := 0; i < numWithoutAvg; i++ {
			sym := withoutAvgPool[i]
			qty := genQuantity(t, fmt.Sprintf("noAvgQty_%d", i))
			currentPrice := genPrice(t, fmt.Sprintf("noAvgCurPrice_%d", i))

			pair := sym + "/" + quoteCurrency
			prices[pair] = currentPrice

			holdings = append(holdings, domain.Holding{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   sym,
				Quantity:      qty,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			})

			expectations = append(expectations, expectedPnL{
				symbol: sym,
				hasAvg: false,
			})
		}

		repo := &valuationHoldingRepo{holdings: holdings}
		market := &mockMarketDataProvider{prices: prices}
		engine := NewValuationEngine(market, repo)

		val, err := engine.ComputeValuation(context.Background(), userID, quoteCurrency)
		if err != nil {
			t.Fatalf("ComputeValuation failed: %v", err)
		}

		if len(val.Assets) != len(expectations) {
			t.Fatalf("expected %d assets, got %d", len(expectations), len(val.Assets))
		}

		// Assets are returned in the same order as holdings.
		for i, exp := range expectations {
			a := val.Assets[i]
			if exp.hasAvg {
				if a.UnrealizedPnL == nil {
					t.Fatalf("asset %d (%s): expected P&L but got nil", i, exp.symbol)
				}
				if math.Abs(*a.UnrealizedPnL-exp.pnl) > 1e-6 {
					t.Fatalf("asset %d (%s): P&L mismatch: got %f, want %f", i, exp.symbol, *a.UnrealizedPnL, exp.pnl)
				}
			} else {
				if a.UnrealizedPnL != nil {
					t.Fatalf("asset %d (%s): expected nil P&L, got %f", i, exp.symbol, *a.UnrealizedPnL)
				}
			}
		}
	})
}

// Feature: portfolio-tracker, Property 15: Historical portfolio value computation
//
// Validates: Requirements 5.1
//
// For any set of holdings and a time series of historical close prices, the
// computed historical portfolio value at each data point equals the sum of
// (holding quantity * close price at that timestamp) for each holding with
// available price data.
func TestPropertyHistoricalPortfolioValueComputation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := uuid.New()
		quoteCurrency := "USD"

		numHoldings := rapid.IntRange(1, 5).Draw(t, "numHoldings")
		numPoints := rapid.IntRange(2, 20).Draw(t, "numPoints")

		// Generate unique symbols for each holding to avoid overlap.
		allSymbols := []string{"BTC", "ETH", "SOL", "ADA", "DOT"}
		usedSymbols := allSymbols[:numHoldings]

		var holdings []domain.Holding
		candleMap := make(map[string][]candle.Candle)

		// Generate sorted timestamps.
		baseTS := time.Now().Add(-7 * 24 * time.Hour).Unix()
		timestamps := make([]int64, numPoints)
		for i := 0; i < numPoints; i++ {
			timestamps[i] = baseTS + int64(i)*3600 // hourly spacing
		}

		// For each holding, generate candles at every timestamp (no gaps).
		type priceEntry struct {
			ts    int64
			close float64
		}
		priceData := make(map[string][]priceEntry)

		for i := 0; i < numHoldings; i++ {
			sym := usedSymbols[i]
			qty := genQuantity(t, fmt.Sprintf("histQty_%d", i))

			holdings = append(holdings, domain.Holding{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   sym,
				Quantity:      qty,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			})

			pair := sym + "/" + quoteCurrency
			var candles []candle.Candle
			var entries []priceEntry
			for j := 0; j < numPoints; j++ {
				closePrice := genPrice(t, fmt.Sprintf("histPrice_%d_%d", i, j))
				candles = append(candles, candle.Candle{
					Pair:      pair,
					Interval:  "1h",
					Timestamp: timestamps[j],
					Close:     closePrice,
				})
				entries = append(entries, priceEntry{ts: timestamps[j], close: closePrice})
			}
			candleMap[pair] = candles
			priceData[sym] = entries
		}

		repo := &valuationHoldingRepo{holdings: holdings}
		market := &mockMarketDataProvider{candles: candleMap}
		engine := NewValuationEngine(market, repo)

		points, err := engine.ComputeHistorical(context.Background(), userID, quoteCurrency, "7d")
		if err != nil {
			t.Fatalf("ComputeHistorical failed: %v", err)
		}

		if len(points) != numPoints {
			t.Fatalf("expected %d historical points, got %d", numPoints, len(points))
		}

		// Verify each data point = sum of (qty * close price).
		for pi, pt := range points {
			var expectedValue float64
			for _, h := range holdings {
				entries := priceData[h.AssetSymbol]
				expectedValue += h.Quantity * entries[pi].close
			}
			if math.Abs(pt.Value-expectedValue) > 1e-6 {
				t.Fatalf("point %d (ts=%d): value mismatch: got %f, want %f",
					pi, pt.Timestamp, pt.Value, expectedValue)
			}
		}
	})
}

// Feature: portfolio-tracker, Property 16: Price carry-forward for historical gaps
//
// Validates: Requirements 5.3
//
// For any historical price series with gaps, the ValuationEngine uses the last
// known close price to fill gap points.
func TestPropertyPriceCarryForwardForHistoricalGaps(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := uuid.New()
		quoteCurrency := "USD"

		// Single holding to isolate carry-forward behavior.
		sym := "BTC"
		qty := genQuantity(t, "gapQty")

		// Generate a series of timestamps with some having prices and some being gaps.
		totalPoints := rapid.IntRange(4, 20).Draw(t, "totalPoints")
		baseTS := time.Now().Add(-7 * 24 * time.Hour).Unix()

		// Decide which points have actual candle data (at least the first one).
		hasPrice := make([]bool, totalPoints)
		hasPrice[0] = true // first point always has a price
		for i := 1; i < totalPoints; i++ {
			hasPrice[i] = rapid.Bool().Draw(t, fmt.Sprintf("hasPrice_%d", i))
		}
		// Ensure at least one gap exists.
		hasGap := false
		for i := 1; i < totalPoints; i++ {
			if !hasPrice[i] {
				hasGap = true
				break
			}
		}
		if !hasGap && totalPoints > 1 {
			// Force a gap at a random position after the first.
			gapIdx := rapid.IntRange(1, totalPoints-1).Draw(t, "forcedGapIdx")
			hasPrice[gapIdx] = false
		}

		pair := sym + "/" + quoteCurrency
		var candles []candle.Candle
		priceAtPoint := make(map[int64]float64)

		for i := 0; i < totalPoints; i++ {
			ts := baseTS + int64(i)*3600
			if hasPrice[i] {
				closePrice := genPrice(t, fmt.Sprintf("gapPrice_%d", i))
				candles = append(candles, candle.Candle{
					Pair:      pair,
					Interval:  "1h",
					Timestamp: ts,
					Close:     closePrice,
				})
				priceAtPoint[ts] = closePrice
			}
		}

		// Use a "clock" asset that has candles at every timestamp to force all
		// timestamps into the series, while the test asset has gaps.
		clockSym := "ETH"
		clockPair := clockSym + "/" + quoteCurrency
		clockQty := 0.0 // zero-effect on value, just provides timestamps

		holdingsWithClock := []domain.Holding{
			{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   sym,
				Quantity:      qty,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			},
			{
				ID:            uuid.New(),
				UserID:        userID,
				AssetSymbol:   clockSym,
				Quantity:      clockQty,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			},
		}

		var clockCandles []candle.Candle
		for i := 0; i < totalPoints; i++ {
			ts := baseTS + int64(i)*3600
			clockCandles = append(clockCandles, candle.Candle{
				Pair:      clockPair,
				Interval:  "1h",
				Timestamp: ts,
				Close:     1.0, // constant price, zero qty means no value contribution
			})
		}

		candleMap2 := map[string][]candle.Candle{
			pair:      candles,
			clockPair: clockCandles,
		}
		repo2 := &valuationHoldingRepo{holdings: holdingsWithClock}
		market2 := &mockMarketDataProvider{candles: candleMap2}
		engine2 := NewValuationEngine(market2, repo2)

		points2, err := engine2.ComputeHistorical(context.Background(), userID, quoteCurrency, "7d")
		if err != nil {
			t.Fatalf("ComputeHistorical (with clock) failed: %v", err)
		}

		if len(points2) != totalPoints {
			t.Fatalf("expected %d points, got %d", totalPoints, len(points2))
		}

		// Sort points by timestamp.
		sort.Slice(points2, func(i, j int) bool { return points2[i].Timestamp < points2[j].Timestamp })

		// Verify carry-forward: at each point, the value should use the last known
		// close price for the test asset.
		var lastKnownPrice float64
		for i := 0; i < totalPoints; i++ {
			ts := baseTS + int64(i)*3600
			if p, ok := priceAtPoint[ts]; ok {
				lastKnownPrice = p
			}

			expectedValue := qty * lastKnownPrice // clock asset contributes 0
			pt := points2[i]

			if math.Abs(pt.Value-expectedValue) > 1e-6 {
				t.Fatalf("point %d (ts=%d): value mismatch: got %f, want %f (lastKnown=%f, hasPrice=%v)",
					i, pt.Timestamp, pt.Value, expectedValue, lastKnownPrice, hasPrice[i])
			}
		}
	})
}
