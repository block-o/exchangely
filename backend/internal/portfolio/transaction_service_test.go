package portfolio

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// --- Mock: TransactionRepository ---

type mockTxRepo struct {
	upserted  []*domain.Transaction
	updated   []*domain.Transaction
	findByID  map[uuid.UUID]*domain.Transaction
	listed    []domain.Transaction
	listTotal int
	listOpts  domain.ListOptions

	// upsertErr lets tests inject errors for specific calls.
	// When set to sql.ErrNoRows, it simulates a manually_edited skip.
	upsertErr error
}

func newMockTxRepo() *mockTxRepo {
	return &mockTxRepo{
		findByID: make(map[uuid.UUID]*domain.Transaction),
	}
}

func (m *mockTxRepo) Create(_ context.Context, tx *domain.Transaction) error {
	return nil
}

func (m *mockTxRepo) Upsert(_ context.Context, tx *domain.Transaction) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserted = append(m.upserted, tx)
	return nil
}

func (m *mockTxRepo) Update(_ context.Context, tx *domain.Transaction) error {
	m.updated = append(m.updated, tx)
	return nil
}

func (m *mockTxRepo) FindByID(_ context.Context, userID, txID uuid.UUID) (*domain.Transaction, error) {
	tx, ok := m.findByID[txID]
	if !ok {
		return nil, nil
	}
	if tx.UserID != userID {
		return nil, nil
	}
	return tx, nil
}

func (m *mockTxRepo) ListByUser(_ context.Context, _ uuid.UUID, opts domain.ListOptions) ([]domain.Transaction, int, error) {
	m.listOpts = opts
	return m.listed, m.listTotal, nil
}

func (m *mockTxRepo) DeleteBySourceRef(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}

func (m *mockTxRepo) CountByUser(_ context.Context, _ uuid.UUID) (int, error) {
	return len(m.upserted), nil
}
func (m *mockTxRepo) DistinctCurrencies(_ context.Context, _ uuid.UUID) ([]string, error) {
	return []string{"USD"}, nil
}

// --- Tests ---

func TestNormalizeForSource_CreatesTransactions(t *testing.T) {
	repo := newMockTxRepo()
	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{
			"BTCUSD": {{Pair: "BTCUSD", Interval: "1h", Close: 50000}},
		},
	}
	resolver := NewPriceResolver(finder)
	svc := NewTransactionService(repo, resolver, nil, 0)

	userID := uuid.New()
	entries := []domain.LedgerEntry{
		{
			Asset:     "BTC",
			Quantity:  1.5,
			Type:      "buy",
			Timestamp: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
			SourceID:  "trade-123",
		},
	}

	err := svc.NormalizeForSource(context.Background(), userID, "binance", "cred-abc", "USD", entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.upserted) != 1 {
		t.Fatalf("expected 1 upserted transaction, got %d", len(repo.upserted))
	}

	tx := repo.upserted[0]
	if tx.UserID != userID {
		t.Errorf("expected user_id %v, got %v", userID, tx.UserID)
	}
	if tx.AssetSymbol != "BTC" {
		t.Errorf("expected asset BTC, got %s", tx.AssetSymbol)
	}
	if tx.Quantity != 1.5 {
		t.Errorf("expected quantity 1.5, got %f", tx.Quantity)
	}
	if tx.Type != "buy" {
		t.Errorf("expected type buy, got %s", tx.Type)
	}
	if tx.Source != "binance" {
		t.Errorf("expected source binance, got %s", tx.Source)
	}
	if tx.SourceRef != "cred-abc" {
		t.Errorf("expected source_ref cred-abc, got %s", tx.SourceRef)
	}
	if tx.Resolution != "hourly" {
		t.Errorf("expected resolution hourly, got %s", tx.Resolution)
	}
	if tx.ReferenceValue == nil {
		t.Fatal("expected non-nil reference_value")
	}
	expected := 50000.0 * 1.5
	if *tx.ReferenceValue != expected {
		t.Errorf("expected reference_value %f, got %f", expected, *tx.ReferenceValue)
	}
	if tx.ReferenceCurrency != "USD" {
		t.Errorf("expected reference_currency USD, got %s", tx.ReferenceCurrency)
	}
}

func TestNormalizeForSource_UnresolvableEntry(t *testing.T) {
	repo := newMockTxRepo()
	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{},
		daily:  map[string][]candle.Candle{},
	}
	resolver := NewPriceResolver(finder)
	svc := NewTransactionService(repo, resolver, nil, 0)

	entries := []domain.LedgerEntry{
		{
			Asset:     "DOGE",
			Quantity:  1000,
			Type:      "buy",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	err := svc.NormalizeForSource(context.Background(), uuid.New(), "kraken", "cred-xyz", "USD", entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.upserted) != 1 {
		t.Fatalf("expected 1 upserted transaction, got %d", len(repo.upserted))
	}

	tx := repo.upserted[0]
	if tx.Resolution != "unresolvable" {
		t.Errorf("expected resolution unresolvable, got %s", tx.Resolution)
	}
	if tx.ReferenceValue != nil {
		t.Errorf("expected nil reference_value for unresolvable, got %v", *tx.ReferenceValue)
	}
}

func TestNormalizeForSource_SkipsManuallyEdited(t *testing.T) {
	repo := newMockTxRepo()
	repo.upsertErr = sql.ErrNoRows // simulates manually_edited skip

	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{
			"BTCUSD": {{Pair: "BTCUSD", Interval: "1h", Close: 50000}},
		},
	}
	resolver := NewPriceResolver(finder)
	svc := NewTransactionService(repo, resolver, nil, 0)

	entries := []domain.LedgerEntry{
		{
			Asset:     "BTC",
			Quantity:  1.0,
			Type:      "buy",
			Timestamp: time.Date(2024, 6, 15, 14, 0, 0, 0, time.UTC),
		},
	}

	err := svc.NormalizeForSource(context.Background(), uuid.New(), "binance", "cred-1", "USD", entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No transactions should have been recorded because upsert returned ErrNoRows.
	if len(repo.upserted) != 0 {
		t.Errorf("expected 0 upserted transactions (manually edited skip), got %d", len(repo.upserted))
	}
}

func TestUpdateTransaction_SetsManuallyEdited(t *testing.T) {
	repo := newMockTxRepo()
	resolver := NewPriceResolver(&mockCandleFinder{})
	svc := NewTransactionService(repo, resolver, nil, 0)

	userID := uuid.New()
	txID := uuid.New()
	repo.findByID[txID] = &domain.Transaction{
		ID:                txID,
		UserID:            userID,
		AssetSymbol:       "ETH",
		Quantity:          10,
		Type:              "buy",
		Timestamp:         time.Now(),
		Source:            "kraken",
		SourceRef:         "cred-1",
		ReferenceValue:    nil,
		ReferenceCurrency: "USD",
		Resolution:        "hourly",
	}

	newValue := 35000.0
	newNotes := "corrected fee"
	err := svc.UpdateTransaction(context.Background(), userID, txID, &newValue, &newNotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 updated transaction, got %d", len(repo.updated))
	}

	tx := repo.updated[0]
	if !tx.ManuallyEdited {
		t.Error("expected manually_edited to be true")
	}
	if tx.ReferenceValue == nil || *tx.ReferenceValue != 35000.0 {
		t.Errorf("expected reference_value 35000, got %v", tx.ReferenceValue)
	}
	if tx.Notes != "corrected fee" {
		t.Errorf("expected notes %q, got %q", "corrected fee", tx.Notes)
	}
	// Immutable fields should remain unchanged.
	if tx.AssetSymbol != "ETH" {
		t.Errorf("asset should be immutable, got %s", tx.AssetSymbol)
	}
	if tx.Quantity != 10 {
		t.Errorf("quantity should be immutable, got %f", tx.Quantity)
	}
}

func TestUpdateTransaction_NotFound(t *testing.T) {
	repo := newMockTxRepo()
	resolver := NewPriceResolver(&mockCandleFinder{})
	svc := NewTransactionService(repo, resolver, nil, 0)

	err := svc.UpdateTransaction(context.Background(), uuid.New(), uuid.New(), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-existent transaction")
	}
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestUpdateTransaction_PartialUpdate(t *testing.T) {
	repo := newMockTxRepo()
	resolver := NewPriceResolver(&mockCandleFinder{})
	svc := NewTransactionService(repo, resolver, nil, 0)

	userID := uuid.New()
	txID := uuid.New()
	origValue := 1000.0
	repo.findByID[txID] = &domain.Transaction{
		ID:                txID,
		UserID:            userID,
		AssetSymbol:       "BTC",
		Quantity:          0.5,
		Type:              "buy",
		Timestamp:         time.Now(),
		Source:            "binance",
		SourceRef:         "cred-1",
		ReferenceValue:    &origValue,
		ReferenceCurrency: "USD",
		Resolution:        "hourly",
		Notes:             "original note",
	}

	// Update only notes, leave value unchanged.
	newNotes := "updated note"
	err := svc.UpdateTransaction(context.Background(), userID, txID, nil, &newNotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tx := repo.updated[0]
	if *tx.ReferenceValue != origValue {
		t.Errorf("expected value unchanged at %f, got %f", origValue, *tx.ReferenceValue)
	}
	if tx.Notes != "updated note" {
		t.Errorf("expected notes %q, got %q", "updated note", tx.Notes)
	}
}

func TestListTransactions_DelegatesToRepo(t *testing.T) {
	repo := newMockTxRepo()
	repo.listed = []domain.Transaction{
		{ID: uuid.New(), AssetSymbol: "BTC"},
		{ID: uuid.New(), AssetSymbol: "ETH"},
	}
	repo.listTotal = 2

	resolver := NewPriceResolver(&mockCandleFinder{})
	svc := NewTransactionService(repo, resolver, nil, 0)

	opts := domain.ListOptions{
		Asset:    "BTC",
		Page:     1,
		PageSize: 50,
	}

	txs, total, err := svc.ListTransactions(context.Background(), uuid.New(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(txs))
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if repo.listOpts.Asset != "BTC" {
		t.Errorf("expected filter asset BTC, got %s", repo.listOpts.Asset)
	}
}

// --- Mock: TaskPublisher ---

type mockTaskPublisher struct {
	published []task.Task
}

func (m *mockTaskPublisher) Publish(_ context.Context, tasks []task.Task) error {
	m.published = append(m.published, tasks...)
	return nil
}

// --- QueueRecompute tests ---

func TestQueueRecompute_PublishesTask(t *testing.T) {
	pub := &mockTaskPublisher{}
	svc := NewTransactionService(newMockTxRepo(), NewPriceResolver(&mockCandleFinder{}), pub, 30*time.Second)

	userID := uuid.New()
	err := svc.QueueRecompute(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published task, got %d", len(pub.published))
	}

	got := pub.published[0]
	expectedID := "portfolio_recompute:" + userID.String() + ":pending"
	if got.ID != expectedID {
		t.Errorf("expected task ID %q, got %q", expectedID, got.ID)
	}
	if got.Type != task.TypePortfolioRecompute {
		t.Errorf("expected task type %q, got %q", task.TypePortfolioRecompute, got.Type)
	}
	if got.Description == "" {
		t.Error("expected non-empty description after Enrich")
	}
}

func TestQueueRecompute_DebouncesSuppressesDuplicate(t *testing.T) {
	pub := &mockTaskPublisher{}
	svc := NewTransactionService(newMockTxRepo(), NewPriceResolver(&mockCandleFinder{}), pub, 30*time.Second)

	userID := uuid.New()

	if err := svc.QueueRecompute(context.Background(), userID); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := svc.QueueRecompute(context.Background(), userID); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if len(pub.published) != 1 {
		t.Errorf("expected 1 published task (second call debounced), got %d", len(pub.published))
	}
}

func TestQueueRecompute_AllowsAfterWindow(t *testing.T) {
	pub := &mockTaskPublisher{}
	window := 50 * time.Millisecond
	svc := NewTransactionService(newMockTxRepo(), NewPriceResolver(&mockCandleFinder{}), pub, window)

	userID := uuid.New()

	if err := svc.QueueRecompute(context.Background(), userID); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Wait for the debounce window to expire.
	time.Sleep(window + 10*time.Millisecond)

	if err := svc.QueueRecompute(context.Background(), userID); err != nil {
		t.Fatalf("second call after window: %v", err)
	}

	if len(pub.published) != 2 {
		t.Errorf("expected 2 published tasks after window expired, got %d", len(pub.published))
	}
}

func TestQueueRecompute_NilPublisher(t *testing.T) {
	svc := NewTransactionService(newMockTxRepo(), NewPriceResolver(&mockCandleFinder{}), nil, 30*time.Second)

	err := svc.QueueRecompute(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("expected no error with nil publisher, got %v", err)
	}
}

func TestQueueRecomputeWithCurrency_PassesCurrencyInInterval(t *testing.T) {
	pub := &mockTaskPublisher{}
	svc := NewTransactionService(newMockTxRepo(), NewPriceResolver(&mockCandleFinder{}), pub, 0)

	userID := uuid.New()
	err := svc.QueueRecomputeWithCurrency(context.Background(), userID, "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published task, got %d", len(pub.published))
	}
	if pub.published[0].Interval != "EUR" {
		t.Errorf("expected Interval EUR, got %q", pub.published[0].Interval)
	}
}

func TestQueueRecomputeWithCurrency_EmptyCurrencyLeavesIntervalEmpty(t *testing.T) {
	pub := &mockTaskPublisher{}
	svc := NewTransactionService(newMockTxRepo(), NewPriceResolver(&mockCandleFinder{}), pub, 0)

	userID := uuid.New()
	err := svc.QueueRecomputeWithCurrency(context.Background(), userID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published task, got %d", len(pub.published))
	}
	if pub.published[0].Interval != "" {
		t.Errorf("expected empty Interval, got %q", pub.published[0].Interval)
	}
}

func TestReNormalizeAll_UpdatesNonManualTransactions(t *testing.T) {
	repo := newMockTxRepo()
	txID1 := uuid.New()
	txID2 := uuid.New()
	userID := uuid.New()
	ts := time.Date(2024, 6, 15, 14, 0, 0, 0, time.UTC)

	repo.listed = []domain.Transaction{
		{
			ID:                txID1,
			UserID:            userID,
			AssetSymbol:       "BTC",
			Quantity:          1.0,
			Type:              "buy",
			Timestamp:         ts,
			Source:            "binance",
			SourceRef:         "cred-1",
			ReferenceCurrency: "USD",
			Resolution:        "hourly",
			ManuallyEdited:    false,
		},
		{
			ID:                txID2,
			UserID:            userID,
			AssetSymbol:       "ETH",
			Quantity:          5.0,
			Type:              "buy",
			Timestamp:         ts,
			Source:            "kraken",
			SourceRef:         "cred-2",
			ReferenceCurrency: "USD",
			Resolution:        "hourly",
			ManuallyEdited:    true,
		},
	}
	repo.listTotal = 2

	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{
			"BTCEUR": {{Pair: "BTCEUR", Interval: "1h", Close: 45000}},
		},
	}
	resolver := NewPriceResolver(finder)
	svc := NewTransactionService(repo, resolver, nil, 0)

	err := svc.ReNormalizeAll(context.Background(), userID, "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 updated transaction (non-manual only), got %d", len(repo.updated))
	}

	tx := repo.updated[0]
	if tx.ID != txID1 {
		t.Errorf("expected updated tx ID %v, got %v", txID1, tx.ID)
	}
	if tx.ReferenceCurrency != "EUR" {
		t.Errorf("expected ReferenceCurrency EUR, got %s", tx.ReferenceCurrency)
	}
	if tx.Resolution != "hourly" {
		t.Errorf("expected Resolution hourly, got %s", tx.Resolution)
	}
}

func TestReNormalizeAll_UnresolvableFallback(t *testing.T) {
	repo := newMockTxRepo()
	txID := uuid.New()
	userID := uuid.New()
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	repo.listed = []domain.Transaction{
		{
			ID:                txID,
			UserID:            userID,
			AssetSymbol:       "DOGE",
			Quantity:          1000,
			Type:              "buy",
			Timestamp:         ts,
			Source:            "kraken",
			SourceRef:         "cred-1",
			ReferenceCurrency: "USD",
			Resolution:        "hourly",
			ManuallyEdited:    false,
		},
	}
	repo.listTotal = 1

	finder := &mockCandleFinder{
		hourly: map[string][]candle.Candle{},
		daily:  map[string][]candle.Candle{},
	}
	resolver := NewPriceResolver(finder)
	svc := NewTransactionService(repo, resolver, nil, 0)

	err := svc.ReNormalizeAll(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 updated transaction, got %d", len(repo.updated))
	}

	tx := repo.updated[0]
	if tx.Resolution != "unresolvable" {
		t.Errorf("expected Resolution unresolvable, got %s", tx.Resolution)
	}
	if tx.ReferenceValue != nil {
		t.Errorf("expected nil ReferenceValue for unresolvable, got %v", *tx.ReferenceValue)
	}
}
