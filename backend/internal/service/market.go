package service

import (
	"context"
	"sort"
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
	updateChs []*MarketSubscription // one buffered delta queue per active SSE subscriber
	mu        sync.RWMutex          // guards updateChs slice

	// Caching layer
	tickerCache        map[string]ticker.Ticker
	tickersCache       []ticker.Ticker
	tickersCacheExpiry time.Time
	cacheMu            sync.RWMutex // guards tickerCache and tickersCache
	cacheSize          int
	tickersTTL         time.Duration
}

// MarketSubscription tracks pending changed pairs for one SSE client.
// A single buffered signal channel wakes the subscriber, while the pending set
// retains all changed pairs until the HTTP layer drains them.
type MarketSubscription struct {
	ch      chan struct{}
	pending map[string]struct{}
	mu      sync.Mutex
}

// NewMarketService returns a MarketService backed by the given repository and configured cache settings.
func NewMarketService(repo MarketRepository, cacheSize int, tickersTTL time.Duration) *MarketService {
	return &MarketService{
		repo:        repo,
		updateChs:   make([]*MarketSubscription, 0),
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
func (s *MarketService) NotifyUpdate(pairSymbol string) {
	s.cacheMu.Lock()
	delete(s.tickerCache, pairSymbol)
	s.cacheMu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.updateChs {
		sub.queuePair(pairSymbol)
	}
}

// Subscribe creates and returns a new buffered channel that receives signals whenever
// market data is updated. The caller must call Unsubscribe when finished (e.g. on SSE disconnect).
func (s *MarketService) Subscribe() *MarketSubscription {
	sub := &MarketSubscription{
		ch:      make(chan struct{}, 1),
		pending: make(map[string]struct{}),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateChs = append(s.updateChs, sub)
	return sub
}

// Unsubscribe removes the given channel from the active subscriber set.
// Safe to call even if the channel was already removed.
func (s *MarketService) Unsubscribe(sub *MarketSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, candidate := range s.updateChs {
		if candidate == sub {
			s.updateChs = append(s.updateChs[:i], s.updateChs[i+1:]...)
			return
		}
	}
}

// Updates returns the wake-up channel for this subscription.
func (s *MarketSubscription) Updates() <-chan struct{} {
	return s.ch
}

// DrainPendingPairs returns the distinct set of changed pairs since the last drain.
func (s *MarketSubscription) DrainPendingPairs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) == 0 {
		return nil
	}

	pairs := make([]string, 0, len(s.pending))
	for pair := range s.pending {
		pairs = append(pairs, pair)
	}
	clear(s.pending)
	sort.Strings(pairs)
	return pairs
}

func (s *MarketSubscription) queuePair(pairSymbol string) {
	s.mu.Lock()
	s.pending[pairSymbol] = struct{}{}
	s.mu.Unlock()

	select {
	case s.ch <- struct{}{}:
	default:
	}
}
