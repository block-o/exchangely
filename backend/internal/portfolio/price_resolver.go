package portfolio

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

// CandleFinder is the focused interface that PriceResolver needs from the
// market data layer. It avoids importing the full MarketRepository.
type CandleFinder interface {
	HourlyCandles(ctx context.Context, pairSymbol string, start, end time.Time) ([]candle.Candle, error)
	Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
}

// Resolution holds the resolved price and the method used to obtain it.
type Resolution struct {
	Price  float64
	Method string // "hourly", "daily", "unresolvable"
}

// PriceResolver resolves a transaction's value in a quote currency using the
// candle fallback chain: hourly → daily → unresolvable.
type PriceResolver struct {
	candles CandleFinder
}

// NewPriceResolver creates a PriceResolver backed by the given CandleFinder.
func NewPriceResolver(candles CandleFinder) *PriceResolver {
	return &PriceResolver{candles: candles}
}

// Resolve looks up the price of asset in quoteCurrency at the given timestamp.
// It tries the hourly candle first, then falls back to daily, then returns
// unresolvable with a zero price.
func (r *PriceResolver) Resolve(ctx context.Context, asset, quoteCurrency string, timestamp time.Time) (Resolution, error) {
	pair := asset + quoteCurrency

	// Try hourly candle for the hour containing the timestamp.
	hourStart := timestamp.UTC().Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	hourly, err := r.candles.HourlyCandles(ctx, pair, hourStart, hourEnd)
	if err != nil {
		return Resolution{}, err
	}
	if len(hourly) > 0 {
		return Resolution{Price: hourly[0].Close, Method: "hourly"}, nil
	}

	// Fall back to daily candle for the day containing the timestamp.
	dayStart := time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.AddDate(0, 0, 1)

	daily, err := r.candles.Historical(ctx, pair, "1d", dayStart, dayEnd)
	if err != nil {
		return Resolution{}, err
	}
	if len(daily) > 0 {
		return Resolution{Price: daily[0].Close, Method: "daily"}, nil
	}

	return Resolution{Price: 0, Method: "unresolvable"}, nil
}
