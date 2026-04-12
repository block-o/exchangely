package portfolio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// MarketDataProvider abstracts the existing MarketService for ticker prices
// and historical candle data. This allows the ValuationEngine to be tested
// without depending on the concrete MarketService implementation.
type MarketDataProvider interface {
	GetTickerPrice(ctx context.Context, pair string) (float64, error)
	GetCandles(ctx context.Context, pair string, interval string, start, end time.Time) ([]candle.Candle, error)
}

// ValuationEngine computes portfolio valuations and historical value series.
type ValuationEngine struct {
	marketSvc   MarketDataProvider
	holdingRepo domain.HoldingRepository
}

// NewValuationEngine creates a ValuationEngine with the given dependencies.
func NewValuationEngine(marketSvc MarketDataProvider, holdingRepo domain.HoldingRepository) *ValuationEngine {
	return &ValuationEngine{
		marketSvc:   marketSvc,
		holdingRepo: holdingRepo,
	}
}

// ComputeValuation computes the current portfolio snapshot for a user.
// For each holding it fetches the current ticker price, computes current value,
// allocation percentages (summing to 100% for priced holdings), and unrealized
// P&L when an average buy price is available.
func (e *ValuationEngine) ComputeValuation(ctx context.Context, userID uuid.UUID, quoteCurrency string) (*domain.Valuation, error) {
	holdings, err := e.holdingRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing holdings: %w", err)
	}

	var totalValue float64
	assets := make([]domain.AssetValuation, 0, len(holdings))

	for _, h := range holdings {
		pair := h.AssetSymbol + "/" + quoteCurrency
		price, err := e.marketSvc.GetTickerPrice(ctx, pair)

		av := domain.AssetValuation{
			AssetSymbol: h.AssetSymbol,
			Quantity:    h.Quantity,
			AvgBuyPrice: h.AvgBuyPrice,
			Source:      h.Source,
		}

		if err != nil {
			// Price unavailable: flag as unpriced, exclude from total.
			av.Priced = false
			assets = append(assets, av)
			continue
		}

		currentValue := h.Quantity * price
		av.Priced = true
		av.CurrentPrice = price
		av.CurrentValue = currentValue
		totalValue += currentValue

		// Compute unrealized P&L when avg buy price is available.
		if h.AvgBuyPrice != nil {
			pnl := (price - *h.AvgBuyPrice) * h.Quantity
			av.UnrealizedPnL = &pnl
			if *h.AvgBuyPrice != 0 {
				pct := ((price - *h.AvgBuyPrice) / *h.AvgBuyPrice) * 100
				av.UnrealizedPct = &pct
			}
		}
		// When no avg buy price, P&L fields remain nil (unknown).

		assets = append(assets, av)
	}

	// Compute allocation percentages for priced holdings.
	if totalValue > 0 {
		for i := range assets {
			if assets[i].Priced {
				assets[i].AllocationPct = roundTo2(assets[i].CurrentValue / totalValue * 100)
			}
		}
		// Adjust for rounding so allocations sum to exactly 100%.
		adjustAllocationRounding(assets)
	}

	return &domain.Valuation{
		TotalValue:    totalValue,
		QuoteCurrency: quoteCurrency,
		Assets:        assets,
		UpdatedAt:     time.Now(),
	}, nil
}

// ComputeHistorical computes the historical portfolio value series for a user
// over the given time range. It uses hourly candles for 1d/7d ranges and daily
// candles for 30d/1y ranges. Missing price data is filled by carrying forward
// the last known close price.
func (e *ValuationEngine) ComputeHistorical(ctx context.Context, userID uuid.UUID, quoteCurrency string, timeRange string) ([]domain.HistoricalPoint, error) {
	holdings, err := e.holdingRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing holdings: %w", err)
	}

	if len(holdings) == 0 {
		return []domain.HistoricalPoint{}, nil
	}

	interval, start, end := resolveTimeRange(timeRange)

	// Fetch candles for each holding's asset.
	// Key: asset symbol, Value: map of timestamp -> close price.
	pricesByAsset := make(map[string]map[int64]float64)
	allTimestamps := make(map[int64]struct{})

	for _, h := range holdings {
		if _, exists := pricesByAsset[h.AssetSymbol]; exists {
			continue // already fetched for this asset
		}

		pair := h.AssetSymbol + "/" + quoteCurrency
		candles, err := e.marketSvc.GetCandles(ctx, pair, interval, start, end)
		if err != nil {
			// No candle data for this asset; skip it.
			continue
		}

		prices := make(map[int64]float64, len(candles))
		for _, c := range candles {
			prices[c.Timestamp] = c.Close
			allTimestamps[c.Timestamp] = struct{}{}
		}
		pricesByAsset[h.AssetSymbol] = prices
	}

	if len(allTimestamps) == 0 {
		return []domain.HistoricalPoint{}, nil
	}

	// Sort timestamps chronologically.
	timestamps := make([]int64, 0, len(allTimestamps))
	for ts := range allTimestamps {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	// Compute portfolio value at each timestamp, carrying forward last known prices.
	lastKnown := make(map[string]float64) // asset -> last known close price
	points := make([]domain.HistoricalPoint, 0, len(timestamps))

	for _, ts := range timestamps {
		var value float64
		for _, h := range holdings {
			prices, ok := pricesByAsset[h.AssetSymbol]
			if !ok {
				continue
			}
			if closePrice, found := prices[ts]; found {
				lastKnown[h.AssetSymbol] = closePrice
				value += h.Quantity * closePrice
			} else if lk, found := lastKnown[h.AssetSymbol]; found {
				// Carry forward last known price for gaps.
				value += h.Quantity * lk
			}
		}
		points = append(points, domain.HistoricalPoint{
			Timestamp: ts,
			Value:     value,
		})
	}

	return points, nil
}

// --- Helpers ---

// resolveTimeRange maps a time range string to the candle interval, start time,
// and end time. Hourly candles are used for short ranges (1d, 7d), daily candles
// for longer ranges (30d, 1y).
func resolveTimeRange(timeRange string) (interval string, start, end time.Time) {
	end = time.Now().UTC()
	switch timeRange {
	case "1d":
		return "1h", end.Add(-24 * time.Hour), end
	case "7d":
		return "1h", end.Add(-7 * 24 * time.Hour), end
	case "30d":
		return "1d", end.Add(-30 * 24 * time.Hour), end
	case "1y":
		return "1d", end.Add(-365 * 24 * time.Hour), end
	default:
		// Default to 7d with hourly candles.
		return "1h", end.Add(-7 * 24 * time.Hour), end
	}
}

// roundTo2 rounds a float64 to 2 decimal places.
func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}

// adjustAllocationRounding ensures priced allocation percentages sum to exactly
// 100% by distributing the rounding residual to the largest allocation.
func adjustAllocationRounding(assets []domain.AssetValuation) {
	var sum float64
	largestIdx := -1
	var largestPct float64

	for i, a := range assets {
		if !a.Priced {
			continue
		}
		sum += a.AllocationPct
		if a.AllocationPct > largestPct {
			largestPct = a.AllocationPct
			largestIdx = i
		}
	}

	if largestIdx >= 0 {
		diff := 100.0 - sum
		assets[largestIdx].AllocationPct = roundTo2(assets[largestIdx].AllocationPct + diff)
	}
}
