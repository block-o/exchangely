package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// PnLRefreshExecutor handles pnl_refresh tasks. It recomputes unrealized P&L
// using current ticker prices and pushes an SSE notification.
type PnLRefreshExecutor struct {
	pnl            PnLComputer
	notifier       PortfolioNotifier
	currencyLookup CurrencyLookup
}

// NewPnLRefreshExecutor creates a PnLRefreshExecutor with the required dependencies.
func NewPnLRefreshExecutor(pnl PnLComputer, notifier PortfolioNotifier, currencyLookup CurrencyLookup) *PnLRefreshExecutor {
	return &PnLRefreshExecutor{
		pnl:            pnl,
		notifier:       notifier,
		currencyLookup: currencyLookup,
	}
}

// Execute performs a P&L refresh for the user identified in the task ID.
func (e *PnLRefreshExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypePnLRefresh {
		return fmt.Errorf("pnl refresh executor received unexpected task type %q", item.Type)
	}

	userID, err := parseUserIDFromTaskID(item.ID)
	if err != nil {
		return fmt.Errorf("parsing user ID from task %q: %w", item.ID, err)
	}

	// Determine which currencies to refresh. Query distinct currencies from
	// the user's transactions so we refresh P&L for all of them. Common quote
	// currencies (USD, EUR) are always included to ensure snapshots exist for
	// whichever currency the frontend requests.
	seen := make(map[string]bool)
	if e.currencyLookup != nil {
		found, lookupErr := e.currencyLookup.DistinctCurrencies(ctx, userID)
		if lookupErr != nil {
			slog.WarnContext(ctx, "failed to query distinct currencies", "user_id", userID, "error", lookupErr)
		}
		for _, c := range found {
			seen[c] = true
		}
	}

	// Always include common quote currencies.
	for _, c := range []string{"USD", "EUR"} {
		seen[c] = true
	}

	currencies := make([]string, 0, len(seen))
	for c := range seen {
		currencies = append(currencies, c)
	}
	sort.Strings(currencies)

	for _, cur := range currencies {
		slog.InfoContext(ctx, "pnl refresh started", "user_id", userID, "currency", cur)

		if _, err := e.pnl.ComputeAndStore(ctx, userID, cur); err != nil {
			slog.WarnContext(ctx, "pnl refresh computation failed", "user_id", userID, "currency", cur, "error", err)
			return fmt.Errorf("computing P&L for %s: %w", cur, err)
		}
	}

	if e.notifier != nil {
		e.notifier.NotifyPortfolioUpdate(userID)
	}

	slog.InfoContext(ctx, "pnl refresh completed", "user_id", userID, "currencies", currencies)
	return nil
}
