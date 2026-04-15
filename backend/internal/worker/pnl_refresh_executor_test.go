package worker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/worker"
)

func TestPnLRefreshExecutor(t *testing.T) {
	userID := uuid.New()
	validTaskID := "pnl_refresh:" + userID.String() + ":periodic"

	t.Run("successful execution calls pnl and notifier", func(t *testing.T) {
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewPnLRefreshExecutor(pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePnLRefresh,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !pnl.called {
			t.Error("expected ComputeAndStore to be called")
		}
		if pnl.lastUser != userID {
			t.Errorf("expected user %s, got %s", userID, pnl.lastUser)
		}
		// Common currencies (EUR, USD) are always included.
		if len(pnl.allCurrencies) != 2 {
			t.Fatalf("expected 2 currencies (EUR+USD), got %d: %v", len(pnl.allCurrencies), pnl.allCurrencies)
		}

		if !notif.called {
			t.Error("expected NotifyPortfolioUpdate to be called")
		}
		if notif.lastUser != userID {
			t.Errorf("expected notified user %s, got %s", userID, notif.lastUser)
		}
	})

	t.Run("wrong task type returns error", func(t *testing.T) {
		exec := worker.NewPnLRefreshExecutor(&mockPnLComputer{}, &mockNotifier{}, nil)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypeBackfill,
		})
		if err == nil {
			t.Fatal("expected error for wrong task type, got nil")
		}
	})

	t.Run("invalid task ID returns error", func(t *testing.T) {
		exec := worker.NewPnLRefreshExecutor(&mockPnLComputer{}, &mockNotifier{}, nil)

		err := exec.Execute(context.Background(), task.Task{
			ID:   "not-valid",
			Type: task.TypePnLRefresh,
		})
		if err == nil {
			t.Fatal("expected error for malformed task ID, got nil")
		}
	})

	t.Run("pnl error propagates", func(t *testing.T) {
		pnlErr := errors.New("pnl refresh failed")
		pnl := &mockPnLComputer{err: pnlErr}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewPnLRefreshExecutor(pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePnLRefresh,
		})
		if !errors.Is(err, pnlErr) {
			t.Fatalf("expected error %v, got %v", pnlErr, err)
		}
		if notif.called {
			t.Error("notifier should not be called when pnl fails")
		}
	})

	t.Run("multi-currency lookup refreshes all currencies", func(t *testing.T) {
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD", "EUR"}}
		exec := worker.NewPnLRefreshExecutor(pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePnLRefresh,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Both USD and EUR should be processed (sorted alphabetically: EUR, USD).
		if len(pnl.allCurrencies) != 2 {
			t.Fatalf("expected ComputeAndStore called for 2 currencies, got %d", len(pnl.allCurrencies))
		}
		if pnl.allCurrencies[0] != "EUR" || pnl.allCurrencies[1] != "USD" {
			t.Errorf("expected currencies [EUR USD], got %v", pnl.allCurrencies)
		}
	})

	t.Run("nil currency lookup includes common currencies", func(t *testing.T) {
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		exec := worker.NewPnLRefreshExecutor(pnl, notif, nil)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePnLRefresh,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With nil lookup, common currencies (EUR, USD) are still included.
		if len(pnl.allCurrencies) != 2 {
			t.Fatalf("expected 2 currencies (EUR+USD), got %d: %v", len(pnl.allCurrencies), pnl.allCurrencies)
		}
	})

	t.Run("currency lookup error still includes common currencies", func(t *testing.T) {
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{err: errors.New("db error")}
		exec := worker.NewPnLRefreshExecutor(pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePnLRefresh,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Even on lookup error, common currencies are included.
		if len(pnl.allCurrencies) != 2 {
			t.Fatalf("expected 2 currencies (EUR+USD) after lookup error, got %d: %v", len(pnl.allCurrencies), pnl.allCurrencies)
		}
	})

	t.Run("nil notifier does not panic", func(t *testing.T) {
		pnl := &mockPnLComputer{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewPnLRefreshExecutor(pnl, nil, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePnLRefresh,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !pnl.called {
			t.Error("expected ComputeAndStore to be called")
		}
	})
}
