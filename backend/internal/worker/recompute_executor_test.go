package worker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/worker"
)

// --- Mock implementations ---

type mockRecomputer struct {
	called        bool
	lastUser      uuid.UUID
	lastCur       string
	allCurrencies []string
	err           error
}

func (m *mockRecomputer) ReNormalizeAll(ctx context.Context, userID uuid.UUID, referenceCurrency string) error {
	m.called = true
	m.lastUser = userID
	m.lastCur = referenceCurrency
	m.allCurrencies = append(m.allCurrencies, referenceCurrency)
	return m.err
}

type mockPnLComputer struct {
	called        bool
	lastUser      uuid.UUID
	lastCur       string
	allCurrencies []string
	err           error
}

func (m *mockPnLComputer) ComputeAndStore(ctx context.Context, userID uuid.UUID, quoteCurrency string) (*domain.PnLSnapshot, error) {
	m.called = true
	m.lastUser = userID
	m.lastCur = quoteCurrency
	m.allCurrencies = append(m.allCurrencies, quoteCurrency)
	if m.err != nil {
		return nil, m.err
	}
	return &domain.PnLSnapshot{UserID: userID, ReferenceCurrency: quoteCurrency}, nil
}

type mockNotifier struct {
	called   bool
	lastUser uuid.UUID
}

func (m *mockNotifier) NotifyPortfolioUpdate(userID uuid.UUID) {
	m.called = true
	m.lastUser = userID
}

type mockCurrencyLookup struct {
	currencies []string
	err        error
}

func (m *mockCurrencyLookup) DistinctCurrencies(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return m.currencies, m.err
}

// --- RecomputeExecutor tests ---

func TestRecomputeExecutor(t *testing.T) {
	userID := uuid.New()
	validTaskID := "portfolio_recompute:" + userID.String() + ":pending"

	t.Run("successful execution calls recomputer, pnl, and notifier", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !recomp.called {
			t.Error("expected ReNormalizeAll to be called")
		}
		if recomp.lastUser != userID {
			t.Errorf("expected user %s, got %s", userID, recomp.lastUser)
		}
		// Common currencies (EUR, USD) are always included.
		if len(recomp.allCurrencies) != 2 {
			t.Fatalf("expected 2 currencies (EUR+USD), got %d: %v", len(recomp.allCurrencies), recomp.allCurrencies)
		}

		if !pnl.called {
			t.Error("expected ComputeAndStore to be called")
		}
		if pnl.lastUser != userID {
			t.Errorf("expected user %s, got %s", userID, pnl.lastUser)
		}

		if !notif.called {
			t.Error("expected NotifyPortfolioUpdate to be called")
		}
		if notif.lastUser != userID {
			t.Errorf("expected notified user %s, got %s", userID, notif.lastUser)
		}
	})

	t.Run("uses currency hint from task interval", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:       validTaskID,
			Type:     task.TypePortfolioRecompute,
			Interval: "EUR",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if recomp.lastCur != "EUR" {
			t.Errorf("expected currency EUR from hint, got %s", recomp.lastCur)
		}
		if pnl.lastCur != "EUR" {
			t.Errorf("expected pnl currency EUR from hint, got %s", pnl.lastCur)
		}
	})

	t.Run("wrong task type returns error", func(t *testing.T) {
		exec := worker.NewRecomputeExecutor(&mockRecomputer{}, &mockPnLComputer{}, &mockNotifier{}, nil)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypeBackfill,
		})
		if err == nil {
			t.Fatal("expected error for wrong task type, got nil")
		}
	})

	t.Run("invalid task ID returns error", func(t *testing.T) {
		exec := worker.NewRecomputeExecutor(&mockRecomputer{}, &mockPnLComputer{}, &mockNotifier{}, nil)

		err := exec.Execute(context.Background(), task.Task{
			ID:   "bad-id",
			Type: task.TypePortfolioRecompute,
		})
		if err == nil {
			t.Fatal("expected error for malformed task ID, got nil")
		}
	})

	t.Run("recomputer error propagates", func(t *testing.T) {
		recompErr := errors.New("renormalize failed")
		recomp := &mockRecomputer{err: recompErr}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if !errors.Is(err, recompErr) {
			t.Fatalf("expected error %v, got %v", recompErr, err)
		}
		if pnl.called {
			t.Error("ComputeAndStore should not be called when recomputer fails")
		}
	})

	t.Run("pnl error propagates", func(t *testing.T) {
		pnlErr := errors.New("pnl compute failed")
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{err: pnlErr}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if !errors.Is(err, pnlErr) {
			t.Fatalf("expected error %v, got %v", pnlErr, err)
		}
		if notif.called {
			t.Error("notifier should not be called when pnl fails")
		}
	})

	t.Run("multi-currency lookup computes for each currency", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD", "EUR"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Both USD and EUR should be processed (sorted alphabetically: EUR, USD).
		if len(recomp.allCurrencies) != 2 {
			t.Fatalf("expected ReNormalizeAll called for 2 currencies, got %d", len(recomp.allCurrencies))
		}
		if recomp.allCurrencies[0] != "EUR" || recomp.allCurrencies[1] != "USD" {
			t.Errorf("expected currencies [EUR USD], got %v", recomp.allCurrencies)
		}
		if len(pnl.allCurrencies) != 2 {
			t.Fatalf("expected ComputeAndStore called for 2 currencies, got %d", len(pnl.allCurrencies))
		}
		if pnl.allCurrencies[0] != "EUR" || pnl.allCurrencies[1] != "USD" {
			t.Errorf("expected currencies [EUR USD], got %v", pnl.allCurrencies)
		}
	})

	t.Run("nil currency lookup includes common currencies", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, nil)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With nil lookup, common currencies (EUR, USD) are still included.
		if len(recomp.allCurrencies) != 2 {
			t.Fatalf("expected 2 currencies (EUR+USD), got %d: %v", len(recomp.allCurrencies), recomp.allCurrencies)
		}
	})

	t.Run("currency lookup error propagates", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{err: errors.New("db error")}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if err == nil {
			t.Fatal("expected error from currency lookup, got nil")
		}
		if recomp.called {
			t.Fatal("recomputer should not be called when currency resolution fails")
		}
	})

	t.Run("currency hint takes precedence over lookup", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		notif := &mockNotifier{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD", "GBP"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, notif, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:       validTaskID,
			Type:     task.TypePortfolioRecompute,
			Interval: "EUR",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(recomp.allCurrencies) != 1 {
			t.Fatalf("expected 1 currency call, got %d", len(recomp.allCurrencies))
		}
		if recomp.allCurrencies[0] != "EUR" {
			t.Errorf("expected currency EUR from hint, got %s", recomp.allCurrencies[0])
		}
		if len(pnl.allCurrencies) != 1 {
			t.Fatalf("expected 1 pnl currency call, got %d", len(pnl.allCurrencies))
		}
		if pnl.allCurrencies[0] != "EUR" {
			t.Errorf("expected pnl currency EUR from hint, got %s", pnl.allCurrencies[0])
		}
	})

	t.Run("nil notifier does not panic", func(t *testing.T) {
		recomp := &mockRecomputer{}
		pnl := &mockPnLComputer{}
		lookup := &mockCurrencyLookup{currencies: []string{"USD"}}
		exec := worker.NewRecomputeExecutor(recomp, pnl, nil, lookup)

		err := exec.Execute(context.Background(), task.Task{
			ID:   validTaskID,
			Type: task.TypePortfolioRecompute,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !recomp.called {
			t.Error("expected ReNormalizeAll to be called")
		}
		if !pnl.called {
			t.Error("expected ComputeAndStore to be called")
		}
	})
}
