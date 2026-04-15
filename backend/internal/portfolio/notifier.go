package portfolio

import (
	"sync"

	"github.com/google/uuid"
)

// PortfolioUpdateSubscription tracks a single SSE client's pending portfolio
// update notifications. A buffered signal channel wakes the subscriber when
// a notification arrives.
type PortfolioUpdateSubscription struct {
	ch      chan uuid.UUID
	closeCh chan struct{}
}

// Updates returns the channel that receives user IDs when their portfolio is updated.
func (s *PortfolioUpdateSubscription) Updates() <-chan uuid.UUID {
	return s.ch
}

// PortfolioUpdateBroadcaster fans out portfolio update notifications to all
// active SSE subscribers. It implements the worker.PortfolioNotifier interface
// so executors can push notifications after recompute/refresh tasks complete.
type PortfolioUpdateBroadcaster struct {
	subs []*PortfolioUpdateSubscription
	mu   sync.RWMutex
}

// NewPortfolioUpdateBroadcaster creates a new broadcaster.
func NewPortfolioUpdateBroadcaster() *PortfolioUpdateBroadcaster {
	return &PortfolioUpdateBroadcaster{}
}

// NotifyPortfolioUpdate sends a non-blocking notification to all active subscribers
// for the given user. This satisfies the worker.PortfolioNotifier interface.
func (b *PortfolioUpdateBroadcaster) NotifyPortfolioUpdate(userID uuid.UUID) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		select {
		case sub.ch <- userID:
		default:
			// Drop if subscriber is slow; they'll get the next one.
		}
	}
}

// Subscribe creates a new subscription for portfolio update notifications.
func (b *PortfolioUpdateBroadcaster) Subscribe() *PortfolioUpdateSubscription {
	sub := &PortfolioUpdateSubscription{
		ch:      make(chan uuid.UUID, 4),
		closeCh: make(chan struct{}),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = append(b.subs, sub)
	return sub
}

// Unsubscribe removes a subscription from the broadcaster.
func (b *PortfolioUpdateBroadcaster) Unsubscribe(sub *PortfolioUpdateSubscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, candidate := range b.subs {
		if candidate == sub {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}
