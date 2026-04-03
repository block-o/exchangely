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

// MarketService provides market data access and a lightweight in-memory pub/sub EventBus.
// The EventBus allows the worker layer to signal the HTTP layer when new data is persisted,
// enabling Server-Sent Events (SSE) to push updates to browser clients without polling.
type MarketService struct {
	repo      MarketRepository
	updateChs []chan struct{} // one buffered channel per active SSE subscriber
	mu        sync.RWMutex    // guards updateChs slice
}

// NewMarketService returns a MarketService backed by the given repository.
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

// Tickers delegates to the repository to fetch the latest global ticker state.
func (s *MarketService) Tickers(ctx context.Context) ([]ticker.Ticker, error) {
	return s.repo.Tickers(ctx)
}

// NotifyUpdate fans out a non-blocking signal to every active SSE subscriber channel.
// Called by the worker layer (BackfillExecutor, RealtimeIngestService) after successful
// database writes. The non-blocking send ensures a slow consumer never blocks the worker.
func (s *MarketService) NotifyUpdate() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.updateChs {
		select {
		case ch <- struct{}{}:
		default: // channel already has a pending signal; skip to avoid blocking
		}
	}
}

// Subscribe creates and returns a new buffered channel that receives signals whenever
// market data is updated. The caller must call Unsubscribe when finished (e.g. on SSE disconnect).
func (s *MarketService) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateChs = append(s.updateChs, ch)
	return ch
}

// Unsubscribe removes the given channel from the active subscriber set.
// Safe to call even if the channel was already removed.
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
