package portfolio

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// TickerProvider abstracts fetching the current price of an asset in a given
// quote currency. Used by PnLEngine to compute unrealized P&L.
type TickerProvider interface {
	GetPrice(ctx context.Context, asset, quoteCurrency string) (float64, error)
}

// costLot represents a single FIFO cost basis lot.
type costLot struct {
	quantity float64
	unitCost float64
}

// PnLEngine computes FIFO-based realized and unrealized P&L from transactions.
type PnLEngine struct {
	txRepo    domain.TransactionRepository
	pnlRepo   domain.PnLRepository
	tickerSvc TickerProvider
}

// NewPnLEngine creates a PnLEngine with the required dependencies.
func NewPnLEngine(
	txRepo domain.TransactionRepository,
	pnlRepo domain.PnLRepository,
	tickerSvc TickerProvider,
) *PnLEngine {
	return &PnLEngine{
		txRepo:    txRepo,
		pnlRepo:   pnlRepo,
		tickerSvc: tickerSvc,
	}
}

// ComputeAndStore runs the FIFO P&L computation for a user and persists the
// resulting snapshot. It loads all transactions, excludes unresolvable ones,
// processes buys/sells through per-asset FIFO queues, then computes unrealized
// P&L for remaining lots using current ticker prices.
func (e *PnLEngine) ComputeAndStore(ctx context.Context, userID uuid.UUID, quoteCurrency string) (*domain.PnLSnapshot, error) {
	// Load all transactions sorted by timestamp ASC.
	allTxs, err := e.loadAllTransactions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading transactions: %w", err)
	}

	// Separate resolvable from unresolvable; track approximate flag.
	var (
		resolved       []domain.Transaction
		excludedCount  int
		hasApproximate bool
	)

	for _, tx := range allTxs {
		if tx.Resolution == "unresolvable" {
			excludedCount++
			continue
		}
		if tx.Resolution == "hourly" || tx.Resolution == "daily" {
			hasApproximate = true
		}
		resolved = append(resolved, tx)
	}

	// Per-asset FIFO queues and realized P&L accumulators.
	fifoQueues := make(map[string][]costLot)    // asset -> FIFO queue
	realizedByAsset := make(map[string]float64) // asset -> realized P&L
	txCountByAsset := make(map[string]int)      // asset -> transaction count

	for _, tx := range resolved {
		asset := tx.AssetSymbol
		txCountByAsset[asset]++

		switch tx.Type {
		case "buy":
			unitCost := 0.0
			if tx.ReferenceValue != nil && tx.Quantity != 0 {
				unitCost = *tx.ReferenceValue / tx.Quantity
			}
			fifoQueues[asset] = append(fifoQueues[asset], costLot{
				quantity: tx.Quantity,
				unitCost: unitCost,
			})

		case "sell":
			sellUnitPrice := 0.0
			if tx.ReferenceValue != nil && tx.Quantity != 0 {
				sellUnitPrice = *tx.ReferenceValue / tx.Quantity
			}
			remaining := tx.Quantity
			queue := fifoQueues[asset]

			for remaining > 0 && len(queue) > 0 {
				lot := &queue[0]
				matched := lot.quantity
				if matched > remaining {
					matched = remaining
				}

				realizedByAsset[asset] += (sellUnitPrice - lot.unitCost) * matched

				lot.quantity -= matched
				remaining -= matched

				if lot.quantity <= 0 {
					queue = queue[1:]
				}
			}
			fifoQueues[asset] = queue

		// Transfers and fees don't affect FIFO cost basis.
		case "transfer", "fee":
			continue
		}
	}

	// Compute unrealized P&L for remaining lots using current prices.
	unrealizedByAsset := make(map[string]float64)
	for asset, queue := range fifoQueues {
		if len(queue) == 0 {
			continue
		}

		currentPrice, err := e.tickerSvc.GetPrice(ctx, asset, quoteCurrency)
		if err != nil {
			// If we can't get a current price, skip unrealized for this asset.
			continue
		}

		for _, lot := range queue {
			unrealizedByAsset[asset] += (currentPrice - lot.unitCost) * lot.quantity
		}
	}

	// Build per-asset breakdown.
	assetSet := make(map[string]struct{})
	for a := range realizedByAsset {
		assetSet[a] = struct{}{}
	}
	for a := range unrealizedByAsset {
		assetSet[a] = struct{}{}
	}
	for a := range txCountByAsset {
		assetSet[a] = struct{}{}
	}

	var (
		assets          []domain.AssetPnL
		totalRealized   float64
		totalUnrealized float64
	)

	for asset := range assetSet {
		realized := realizedByAsset[asset]
		unrealized := unrealizedByAsset[asset]
		assets = append(assets, domain.AssetPnL{
			AssetSymbol:      asset,
			RealizedPnL:      realized,
			UnrealizedPnL:    unrealized,
			TotalPnL:         realized + unrealized,
			TransactionCount: txCountByAsset[asset],
		})
		totalRealized += realized
		totalUnrealized += unrealized
	}

	// Sort assets alphabetically for deterministic output.
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].AssetSymbol < assets[j].AssetSymbol
	})

	snapshot := &domain.PnLSnapshot{
		ID:                uuid.New(),
		UserID:            userID,
		ReferenceCurrency: quoteCurrency,
		TotalRealized:     totalRealized,
		TotalUnrealized:   totalUnrealized,
		TotalPnL:          totalRealized + totalUnrealized,
		HasApproximate:    hasApproximate,
		ExcludedCount:     excludedCount,
		Assets:            assets,
		ComputedAt:        time.Now().UTC(),
	}

	if err := e.pnlRepo.Upsert(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("persisting snapshot: %w", err)
	}

	return snapshot, nil
}

// GetSnapshot returns the latest persisted P&L snapshot for the user.
func (e *PnLEngine) GetSnapshot(ctx context.Context, userID uuid.UUID, quoteCurrency string) (*domain.PnLSnapshot, error) {
	return e.pnlRepo.FindByUser(ctx, userID, quoteCurrency)
}

// loadAllTransactions fetches all transactions for a user sorted by timestamp
// ASC. It pages through the repository to collect the full set.
func (e *PnLEngine) loadAllTransactions(ctx context.Context, userID uuid.UUID) ([]domain.Transaction, error) {
	const batchSize = 500
	var all []domain.Transaction
	page := 1

	for {
		batch, total, err := e.txRepo.ListByUser(ctx, userID, domain.ListOptions{
			Page:     page,
			PageSize: batchSize,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(all) >= total || len(batch) == 0 {
			break
		}
		page++
	}

	// ListByUser returns DESC; reverse to ASC for FIFO processing.
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.Before(all[j].Timestamp)
	})

	return all, nil
}
