package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// TaskPublisher publishes tasks to the messaging layer (e.g. Kafka).
type TaskPublisher interface {
	Publish(ctx context.Context, tasks []task.Task) error
}

// TransactionService orchestrates transaction normalization from ledger entries,
// manual edits, listing, and debounced recompute task queuing.
type TransactionService struct {
	txRepo          domain.TransactionRepository
	priceResolver   *PriceResolver
	taskPublisher   TaskPublisher
	debounceWindow  time.Duration
	debounceTracker map[uuid.UUID]time.Time
	mu              sync.Mutex
}

// NewTransactionService creates a TransactionService with the required dependencies.
func NewTransactionService(
	txRepo domain.TransactionRepository,
	priceResolver *PriceResolver,
	taskPublisher TaskPublisher,
	debounceWindow time.Duration,
) *TransactionService {
	return &TransactionService{
		txRepo:          txRepo,
		priceResolver:   priceResolver,
		taskPublisher:   taskPublisher,
		debounceWindow:  debounceWindow,
		debounceTracker: make(map[uuid.UUID]time.Time),
	}
}

// NormalizeForSource creates or updates transactions from ledger entries for a
// given source. Each entry is resolved via PriceResolver and upserted. Rows
// that have been manually edited are silently skipped by the repository's
// upsert logic (returns sql.ErrNoRows).
func (s *TransactionService) NormalizeForSource(
	ctx context.Context,
	userID uuid.UUID,
	source, sourceRef string,
	referenceCurrency string,
	entries []domain.LedgerEntry,
) error {
	for _, entry := range entries {
		res, err := s.priceResolver.Resolve(ctx, entry.Asset, referenceCurrency, entry.Timestamp)
		if err != nil {
			slog.WarnContext(ctx, "price resolution failed",
				"asset", entry.Asset,
				"timestamp", entry.Timestamp,
				"error", err,
			)
			res = Resolution{Price: 0, Method: "unresolvable"}
		}

		var refValue *float64
		if res.Method != "unresolvable" {
			v := res.Price * entry.Quantity
			refValue = &v
		}

		tx := &domain.Transaction{
			ID:                uuid.New(),
			UserID:            userID,
			AssetSymbol:       entry.Asset,
			Quantity:          entry.Quantity,
			Type:              entry.Type,
			Timestamp:         entry.Timestamp,
			Source:            source,
			SourceRef:         sourceRef,
			ReferenceValue:    refValue,
			ReferenceCurrency: referenceCurrency,
			Resolution:        res.Method,
			Fee:               entry.Fee,
			FeeCurrency:       entry.FeeCurrency,
		}

		err = s.txRepo.Upsert(ctx, tx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Row exists with manually_edited = true; skip silently.
				slog.DebugContext(ctx, "skipping manually edited transaction",
					"asset", entry.Asset,
					"source", source,
					"source_ref", sourceRef,
				)
				continue
			}
			return fmt.Errorf("upsert transaction for %s: %w", entry.Asset, err)
		}
	}
	return nil
}

// UpdateTransaction allows editing a transaction's reference value and notes.
// It sets the manually_edited flag to true. Only value and notes are editable;
// asset, quantity, type, and timestamp remain immutable.
func (s *TransactionService) UpdateTransaction(
	ctx context.Context,
	userID, txID uuid.UUID,
	value *float64,
	notes *string,
) error {
	tx, err := s.txRepo.FindByID(ctx, userID, txID)
	if err != nil {
		return fmt.Errorf("find transaction: %w", err)
	}
	if tx == nil {
		return sql.ErrNoRows
	}

	if value != nil {
		tx.ReferenceValue = value
	}
	if notes != nil {
		tx.Notes = *notes
	}
	tx.ManuallyEdited = true

	if err := s.txRepo.Update(ctx, tx); err != nil {
		return fmt.Errorf("update transaction: %w", err)
	}
	return nil
}

// ListTransactions returns paginated transactions for a user with optional filters.
func (s *TransactionService) ListTransactions(
	ctx context.Context,
	userID uuid.UUID,
	opts domain.ListOptions,
) ([]domain.Transaction, int, error) {
	return s.txRepo.ListByUser(ctx, userID, opts)
}

// ReNormalizeAll re-resolves prices for all non-manually-edited transactions
// belonging to the user. This is called during a full portfolio recompute to
// update reference values when the currency changes or new candle data arrives.
func (s *TransactionService) ReNormalizeAll(ctx context.Context, userID uuid.UUID, referenceCurrency string) error {
	const batchSize = 500
	page := 1

	for {
		batch, total, err := s.txRepo.ListByUser(ctx, userID, domain.ListOptions{
			Page:     page,
			PageSize: batchSize,
		})
		if err != nil {
			return fmt.Errorf("listing transactions page %d: %w", page, err)
		}

		for i := range batch {
			tx := &batch[i]
			if tx.ManuallyEdited {
				continue
			}

			res, err := s.priceResolver.Resolve(ctx, tx.AssetSymbol, referenceCurrency, tx.Timestamp)
			if err != nil {
				slog.WarnContext(ctx, "re-normalization price resolution failed",
					"tx_id", tx.ID,
					"asset", tx.AssetSymbol,
					"error", err,
				)
				res = Resolution{Price: 0, Method: "unresolvable"}
			}

			var refValue *float64
			if res.Method != "unresolvable" {
				v := res.Price * tx.Quantity
				refValue = &v
			}

			tx.ReferenceValue = refValue
			tx.ReferenceCurrency = referenceCurrency
			tx.Resolution = res.Method

			if err := s.txRepo.Update(ctx, tx); err != nil {
				return fmt.Errorf("updating transaction %s: %w", tx.ID, err)
			}
		}

		processed := page * batchSize
		if processed >= total || len(batch) == 0 {
			break
		}
		page++
	}

	slog.InfoContext(ctx, "re-normalization completed", "user_id", userID, "currency", referenceCurrency)
	return nil
}

// QueueRecompute enqueues a debounced recompute task for the user. If a
// recompute was already queued within the debounce window, the call is a
// no-op. The task uses a stable ID so at most one pending task exists per user.
func (s *TransactionService) QueueRecompute(ctx context.Context, userID uuid.UUID) error {
	return s.QueueRecomputeWithCurrency(ctx, userID, "")
}

// QueueRecomputeWithCurrency enqueues a debounced recompute task for the user
// with an explicit reference currency hint. When currency is empty, the executor
// will query distinct currencies from the user's transactions.
func (s *TransactionService) QueueRecomputeWithCurrency(ctx context.Context, userID uuid.UUID, currency string) error {
	if s.taskPublisher == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if last, ok := s.debounceTracker[userID]; ok && s.debounceWindow > 0 {
		if now.Sub(last) < s.debounceWindow {
			slog.DebugContext(ctx, "recompute debounced",
				"user_id", userID,
				"last_queued", last,
				"window", s.debounceWindow,
			)
			return nil
		}
	}

	t := task.Task{
		ID:       fmt.Sprintf("portfolio_recompute:%s:pending", userID),
		Type:     task.TypePortfolioRecompute,
		Interval: currency, // Carries the requested currency to the executor.
	}
	task.Enrich(&t)

	if err := s.taskPublisher.Publish(ctx, []task.Task{t}); err != nil {
		return fmt.Errorf("publish recompute task: %w", err)
	}

	s.debounceTracker[userID] = now
	slog.InfoContext(ctx, "recompute task queued", "user_id", userID, "task_id", t.ID, "currency", currency)
	return nil
}
