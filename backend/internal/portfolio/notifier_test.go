package portfolio

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPortfolioUpdateBroadcaster_NotifyAndSubscribe(t *testing.T) {
	b := NewPortfolioUpdateBroadcaster()
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)

	userID := uuid.New()
	b.NotifyPortfolioUpdate(userID)

	select {
	case got := <-sub.Updates():
		if got != userID {
			t.Fatalf("expected user ID %s, got %s", userID, got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for notification")
	}
}

func TestPortfolioUpdateBroadcaster_MultipleSubscribers(t *testing.T) {
	b := NewPortfolioUpdateBroadcaster()
	sub1 := b.Subscribe()
	sub2 := b.Subscribe()
	defer b.Unsubscribe(sub1)
	defer b.Unsubscribe(sub2)

	userID := uuid.New()
	b.NotifyPortfolioUpdate(userID)

	for i, sub := range []*PortfolioUpdateSubscription{sub1, sub2} {
		select {
		case got := <-sub.Updates():
			if got != userID {
				t.Fatalf("subscriber %d: expected user ID %s, got %s", i, userID, got)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timed out waiting for notification", i)
		}
	}
}

func TestPortfolioUpdateBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewPortfolioUpdateBroadcaster()
	sub := b.Subscribe()
	b.Unsubscribe(sub)

	b.NotifyPortfolioUpdate(uuid.New())

	select {
	case <-sub.Updates():
		t.Fatal("should not receive notification after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestPortfolioUpdateBroadcaster_NonBlockingOnSlowSubscriber(t *testing.T) {
	b := NewPortfolioUpdateBroadcaster()
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)

	// Fill the channel buffer (capacity 4).
	for i := 0; i < 10; i++ {
		b.NotifyPortfolioUpdate(uuid.New())
	}

	// Should not panic or block. Drain what we can.
	drained := 0
	for {
		select {
		case <-sub.Updates():
			drained++
		default:
			goto done
		}
	}
done:
	if drained == 0 {
		t.Fatal("expected at least one notification to be delivered")
	}
}
