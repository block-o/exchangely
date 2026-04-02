package service

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
)

type MarketRepository interface {
	Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
	Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error)
}

type MarketService struct {
	repo MarketRepository
}

func NewMarketService(repo MarketRepository) *MarketService {
	return &MarketService{repo: repo}
}

func (s *MarketService) Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error) {
	return s.repo.Historical(ctx, pairSymbol, interval, start, end)
}

func (s *MarketService) Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error) {
	return s.repo.Ticker(ctx, pairSymbol)
}
