package service

import (
	"context"
	"sync"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
)

type MarketRepository interface {
	Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
	Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error)
	Tickers(ctx context.Context) ([]ticker.Ticker, error)
}

type MarketService struct {
	repo      MarketRepository
	updateChs []chan struct{}
	mu        sync.RWMutex
}

func NewMarketService(repo MarketRepository) *MarketService {
	return &MarketService{
		repo:      repo,
		updateChs: make([]chan struct{}, 0),
	}
}

func (s *MarketService) Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error) {
	return s.repo.Historical(ctx, pairSymbol, interval, start, end)
}

func (s *MarketService) Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error) {
	return s.repo.Ticker(ctx, pairSymbol)
}

func (s *MarketService) Tickers(ctx context.Context) ([]ticker.Ticker, error) {
	return s.repo.Tickers(ctx)
}

func (s *MarketService) NotifyUpdate() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.updateChs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *MarketService) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateChs = append(s.updateChs, ch)
	return ch
}

func (s *MarketService) Unsubscribe(ch <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.updateChs {
		if c == ch {
			s.updateChs = append(s.updateChs[:i], s.updateChs[i+1:]...)
			return
		}
	}
}
