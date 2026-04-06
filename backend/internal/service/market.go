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

	// Caching layer
	tickerCache        map[string]ticker.Ticker
	tickersCache       []ticker.Ticker
	tickersCacheExpiry time.Time
	cacheMu            sync.RWMutex // guards tickerCache and tickersCache
	cacheSize          int
	tickersTTL         time.Duration
}

// NewMarketService returns a MarketService backed by the given repository and configured cache settings.
func NewMarketService(repo MarketRepository, cacheSize int, tickersTTL time.Duration) *MarketService {
	return &MarketService{
		repo:        repo,
		updateChs:   make([]chan struct{}, 0),
		tickerCache: make(map[string]ticker.Ticker, cacheSize),
		cacheSize:   cacheSize,
		tickersTTL:  tickersTTL,
	}
}

func (s *MarketService) Historical(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error) {
	return s.repo.Historical(ctx, pairSymbol, interval, start, end)
}

func (s *MarketService) Ticker(ctx context.Context, pairSymbol string) (ticker.Ticker, error) {
	s.cacheMu.RLock()
	item, ok := s.tickerCache[pairSymbol]
	s.cacheMu.RUnlock()
	if ok {
		return item, nil
	}

	item, err := s.repo.Ticker(ctx, pairSymbol)
	if err != nil {
		return ticker.Ticker{}, err
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	// Basic LRU/capacity management: if the map grows too large, clear it.
	// Since we only have ~30 pairs, we rarely hit the default 100 capacity.
	if len(s.tickerCache) >= s.cacheSize {
		s.tickerCache = make(map[string]ticker.Ticker, s.cacheSize)
	}
	s.tickerCache[pairSymbol] = item

	return item, nil
}

// Tickers delegates to the repository to fetch the latest global ticker state.
// Results are cached for a configurable duration (tickersTTL) to prevent DB pressure.
func (s *MarketService) Tickers(ctx context.Context) ([]ticker.Ticker, error) {
	s.cacheMu.RLock()
	if s.tickersCache != nil && time.Now().Before(s.tickersCacheExpiry) {
		res := s.tickersCache
		s.cacheMu.RUnlock()
		return res, nil
	}
	s.cacheMu.RUnlock()

	items, err := s.repo.Tickers(ctx)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.tickersCache = items
	s.tickersCacheExpiry = time.Now().Add(s.tickersTTL)

	return items, nil
}

// NotifyUpdate fans out a non-blocking signal to every active SSE subscriber channel.
// It also invalidates the cached ticker entry for the specific pair that was updated.
// The global "all tickers" cache is NOT invalidated here; it expires based on its TTL.
func (s *MarketService) NotifyUpdate(pairSymbol string) {
	s.cacheMu.Lock()
	delete(s.tickerCache, pairSymbol)
	s.cacheMu.Unlock()

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
