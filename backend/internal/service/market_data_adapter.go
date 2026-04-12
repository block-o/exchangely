package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

// MarketDataAdapter adapts the MarketRepository to the portfolio
// ValuationEngine's MarketDataProvider interface.
type MarketDataAdapter struct {
	repo MarketRepository
}

// NewMarketDataAdapter creates a MarketDataAdapter.
func NewMarketDataAdapter(repo MarketRepository) *MarketDataAdapter {
	return &MarketDataAdapter{repo: repo}
}

// GetTickerPrice returns the current price for a pair (e.g. "BTC/USD").
// The pair format uses "/" as separator; the repository expects concatenated
// symbols (e.g. "BTCUSD"), so we strip the separator.
func (a *MarketDataAdapter) GetTickerPrice(ctx context.Context, pair string) (float64, error) {
	pairSymbol := strings.ReplaceAll(pair, "/", "")
	t, err := a.repo.Ticker(ctx, pairSymbol)
	if err != nil {
		return 0, fmt.Errorf("ticker price for %s: %w", pair, err)
	}
	if t.Price == 0 {
		return 0, fmt.Errorf("no price available for %s", pair)
	}
	return t.Price, nil
}

// GetCandles returns historical candle data for a pair and interval.
func (a *MarketDataAdapter) GetCandles(ctx context.Context, pair string, interval string, start, end time.Time) ([]candle.Candle, error) {
	pairSymbol := strings.ReplaceAll(pair, "/", "")
	return a.repo.Historical(ctx, pairSymbol, interval, start, end)
}
