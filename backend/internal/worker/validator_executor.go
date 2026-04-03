package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest"
	"github.com/block-o/exchangely/backend/internal/ingest/registry"
)

type ValidatorExecutor struct {
	sources []ingest.Source
}

func NewValidatorExecutor(sources []ingest.Source) *ValidatorExecutor {
	// Filter out nils
	filtered := make([]ingest.Source, 0, len(sources))
	for _, s := range sources {
		if s != nil {
			filtered = append(filtered, s)
		}
	}
	return &ValidatorExecutor{sources: filtered}
}

func (v *ValidatorExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeDataSanity {
		return fmt.Errorf("validator received non-sanity task %q", item.Type)
	}

	base, quote, err := registry.ParsePairSymbol(item.Pair)
	if err != nil {
		return err
	}

	slog.Info("data integrity validator started", "pair", item.Pair, "window", item.WindowStart)

	results := make(map[string][]candle.Candle)
	var expectedCandleCount int

	for _, source := range v.sources {
		req := ingest.Request{
			Pair:      item.Pair,
			Base:      base,
			Quote:     quote,
			Interval:  item.Interval,
			StartTime: item.WindowStart.UTC(),
			EndTime:   item.WindowEnd.UTC(),
		}
		
		if !source.Supports(req) {
			continue
		}

		candles, err := source.FetchCandles(ctx, req)
		if err != nil {
			slog.Warn("data integrity source ping failed", "source", source.Name(), "error", err)
			continue
		}
		
		results[source.Name()] = candles
		if len(candles) > expectedCandleCount {
			expectedCandleCount = len(candles)
		}
	}

	if len(results) < 2 {
		slog.Debug("data integrity validator aborted (insufficient peer overlap)", "pair", item.Pair, "available_sources", len(results))
		return nil
	}

	// 1. Check for Continuity Gaps (e.g., Binance has 60 candles but Kraken has 58)
	for name, set := range results {
		if len(set) < expectedCandleCount {
			slog.Warn("data integrity GAP detected", 
				"pair", item.Pair, 
				"source", name, 
				"missing", expectedCandleCount - len(set),
			)
		}
	}

	// 2. Check for Price Divergence > 0.5%
	// Map all candles by timestamp:
	byTime := make(map[int64]map[string]float64)
	for sourceName, set := range results {
		for _, c := range set {
			if byTime[c.Timestamp] == nil {
				byTime[c.Timestamp] = make(map[string]float64)
			}
			byTime[c.Timestamp][sourceName] = c.Close
		}
	}

	breachCount := 0
	for ts, sourceMap := range byTime {
		if len(sourceMap) < 2 {
			continue
		}
		
		var maxPrice, minPrice float64 = 0, math.MaxFloat64
		for _, price := range sourceMap {
			if price > maxPrice {
				maxPrice = price
			}
			if price < minPrice {
				minPrice = price
			}
		}

		if minPrice == 0 {
			continue
		}

		variance := ((maxPrice - minPrice) / minPrice) * 100.0
		if variance > 0.5 {
			breachCount++
			slog.Warn("data integrity DIVERGENCE detected",
				"pair", item.Pair,
				"timestamp", ts,
				"variance_pct", fmt.Sprintf("%.2f%%", variance),
				"max_price", maxPrice,
				"min_price", minPrice,
				"details", fmt.Sprintf("%v", sourceMap),
			)
		}
	}

	if breachCount == 0 {
		slog.Info("data integrity successfully validated", "pair", item.Pair, "window", item.WindowStart, "sources", len(results))
	}

	return nil
}
