package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// PortfolioRecomputer re-resolves prices for all of a user's transactions.
type PortfolioRecomputer interface {
	ReNormalizeAll(ctx context.Context, userID uuid.UUID, referenceCurrency string) error
}

// PnLComputer computes and persists a FIFO P&L snapshot for a user.
type PnLComputer interface {
	ComputeAndStore(ctx context.Context, userID uuid.UUID, quoteCurrency string) (*domain.PnLSnapshot, error)
}

// PortfolioNotifier pushes portfolio update notifications (e.g. via SSE).
type PortfolioNotifier interface {
	NotifyPortfolioUpdate(userID uuid.UUID)
}

// CurrencyLookup returns the distinct reference currencies for a user's transactions.
type CurrencyLookup interface {
	DistinctCurrencies(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// RecomputeExecutor handles portfolio_recompute tasks. It re-resolves prices
// for all transactions, runs FIFO P&L computation, and pushes an SSE notification.
type RecomputeExecutor struct {
	recomputer     PortfolioRecomputer
	pnl            PnLComputer
	notifier       PortfolioNotifier
	currencyLookup CurrencyLookup
}

// NewRecomputeExecutor creates a RecomputeExecutor with the required dependencies.
func NewRecomputeExecutor(
	recomputer PortfolioRecomputer,
	pnl PnLComputer,
	notifier PortfolioNotifier,
	currencyLookup CurrencyLookup,
) *RecomputeExecutor {
	return &RecomputeExecutor{
		recomputer:     recomputer,
		pnl:            pnl,
		notifier:       notifier,
		currencyLookup: currencyLookup,
	}
}

// Execute performs a full portfolio recompute for the user identified in the task ID.
func (e *RecomputeExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypePortfolioRecompute {
		return fmt.Errorf("recompute executor received unexpected task type %q", item.Type)
	}

	userID, err := parseUserIDFromTaskID(item.ID)
	if err != nil {
		return fmt.Errorf("parsing user ID from task %q: %w", item.ID, err)
	}

	// Determine which currencies to recompute. The Interval field carries the
	// requested currency when set by the handler (e.g. on currency change).
	// Otherwise, query the distinct currencies from the user's transactions.
	currencies, err := e.resolveCurrencies(ctx, userID, item.Interval)
	if err != nil {
		return fmt.Errorf("resolving currencies: %w", err)
	}

	for _, cur := range currencies {
		slog.InfoContext(ctx, "portfolio recompute started", "user_id", userID, "currency", cur)

		if err := e.recomputer.ReNormalizeAll(ctx, userID, cur); err != nil {
			slog.WarnContext(ctx, "portfolio re-normalization failed", "user_id", userID, "currency", cur, "error", err)
			return fmt.Errorf("re-normalizing transactions for %s: %w", cur, err)
		}

		if _, err := e.pnl.ComputeAndStore(ctx, userID, cur); err != nil {
			slog.WarnContext(ctx, "portfolio P&L computation failed", "user_id", userID, "currency", cur, "error", err)
			return fmt.Errorf("computing P&L for %s: %w", cur, err)
		}
	}

	if e.notifier != nil {
		e.notifier.NotifyPortfolioUpdate(userID)
	}

	slog.InfoContext(ctx, "portfolio recompute completed", "user_id", userID, "currencies", currencies)
	return nil
}

// resolveCurrencies determines which currencies to process. If the task carries
// a currency hint (via the Interval field), that single currency is used.
// Otherwise it queries the user's transaction currencies, falling back to USD
// if none exist. Common quote currencies (USD, EUR) are always included to
// ensure P&L snapshots exist for the most likely frontend requests.
func (e *RecomputeExecutor) resolveCurrencies(ctx context.Context, userID uuid.UUID, hint string) ([]string, error) {
	// If the task carries a specific currency hint, use it.
	if hint != "" {
		return []string{hint}, nil
	}

	// Otherwise query distinct currencies from the user's transactions.
	seen := make(map[string]bool)
	if e.currencyLookup != nil {
		currencies, err := e.currencyLookup.DistinctCurrencies(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, c := range currencies {
			seen[c] = true
		}
	}

	// Always include common quote currencies so P&L snapshots exist for
	// whichever currency the frontend requests.
	for _, c := range []string{"USD", "EUR"} {
		seen[c] = true
	}

	result := make([]string, 0, len(seen))
	for c := range seen {
		result = append(result, c)
	}

	// Sort for deterministic ordering in logs.
	sort.Strings(result)
	return result, nil
}

// parseUserIDFromTaskID extracts the UUID from a task ID with the format
// "portfolio_recompute:{USER_ID}:pending" or "pnl_refresh:{USER_ID}:periodic".
func parseUserIDFromTaskID(taskID string) (uuid.UUID, error) {
	parts := strings.Split(taskID, ":")
	if len(parts) < 3 {
		return uuid.Nil, fmt.Errorf("expected at least 3 colon-separated segments, got %d", len(parts))
	}
	return uuid.Parse(parts[1])
}
