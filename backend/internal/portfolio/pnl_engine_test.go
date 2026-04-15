package portfolio

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// --- Mock: PnLRepository ---

type mockPnLRepo struct {
	upserted []*domain.PnLSnapshot
}

func (m *mockPnLRepo) Upsert(_ context.Context, snapshot *domain.PnLSnapshot) error {
	m.upserted = append(m.upserted, snapshot)
	return nil
}

func (m *mockPnLRepo) FindByUser(_ context.Context, _ uuid.UUID, _ string) (*domain.PnLSnapshot, error) {
	if len(m.upserted) == 0 {
		return nil, nil
	}
	return m.upserted[len(m.upserted)-1], nil
}

// --- Mock: TickerProvider ---

type mockTickerProvider struct {
	prices map[string]float64 // "ASSET:QUOTE" -> price
}

func (m *mockTickerProvider) GetPrice(_ context.Context, asset, quoteCurrency string) (float64, error) {
	key := asset + ":" + quoteCurrency
	p, ok := m.prices[key]
	if !ok {
		return 0, fmt.Errorf("no price for %s", key)
	}
	return p, nil
}

// --- Mock: TransactionRepository for PnL tests ---

type mockPnLTxRepo struct {
	txs []domain.Transaction
}

func newMockPnLTxRepo(txs []domain.Transaction) *mockPnLTxRepo {
	return &mockPnLTxRepo{txs: txs}
}

func (m *mockPnLTxRepo) Create(_ context.Context, _ *domain.Transaction) error { return nil }
func (m *mockPnLTxRepo) Upsert(_ context.Context, _ *domain.Transaction) error { return nil }
func (m *mockPnLTxRepo) Update(_ context.Context, _ *domain.Transaction) error { return nil }
func (m *mockPnLTxRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.Transaction, error) {
	return nil, nil
}
func (m *mockPnLTxRepo) DeleteBySourceRef(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (m *mockPnLTxRepo) CountByUser(_ context.Context, _ uuid.UUID) (int, error) {
	return len(m.txs), nil
}
func (m *mockPnLTxRepo) DistinctCurrencies(_ context.Context, _ uuid.UUID) ([]string, error) {
	return []string{"USD"}, nil
}

// ListByUser returns transactions sorted by timestamp DESC (matching real repo behavior).
// The PnLEngine's loadAllTransactions reverses this to ASC for FIFO processing.
func (m *mockPnLTxRepo) ListByUser(_ context.Context, _ uuid.UUID, opts domain.ListOptions) ([]domain.Transaction, int, error) {
	start := (opts.Page - 1) * opts.PageSize
	if start >= len(m.txs) {
		return nil, len(m.txs), nil
	}
	end := start + opts.PageSize
	if end > len(m.txs) {
		end = len(m.txs)
	}
	return m.txs[start:end], len(m.txs), nil
}

// --- Helpers ---

func refVal(v float64) *float64 { return &v }

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func makeTx(asset, txType, resolution string, qty float64, refValue *float64, ts time.Time) domain.Transaction {
	return domain.Transaction{
		ID:                uuid.New(),
		UserID:            uuid.Nil,
		AssetSymbol:       asset,
		Quantity:          qty,
		Type:              txType,
		Timestamp:         ts,
		Source:            "test",
		SourceRef:         "ref",
		ReferenceValue:    refValue,
		ReferenceCurrency: "USD",
		Resolution:        resolution,
	}
}

// --- Tests ---

func TestPnLEngine_BasicFIFO(t *testing.T) {
	// Buy 10 BTC @ $100/unit ($1000 total), sell 5 BTC @ $150/unit ($750 total)
	// Realized = (150 - 100) * 5 = $250
	userID := uuid.New()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	txs := []domain.Transaction{
		makeTx("BTC", "buy", "hourly", 10, refVal(1000), t0),
		makeTx("BTC", "sell", "hourly", 5, refVal(750), t1),
	}
	// Set user ID on all transactions.
	for i := range txs {
		txs[i].UserID = userID
	}

	txRepo := newMockPnLTxRepo(txs)
	pnlRepo := &mockPnLRepo{}
	ticker := &mockTickerProvider{prices: map[string]float64{"BTC:USD": 150}}

	engine := NewPnLEngine(txRepo, pnlRepo, ticker)
	snap, err := engine.ComputeAndStore(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Realized: (150 - 100) * 5 = 250
	if !almostEqual(snap.TotalRealized, 250, 0.01) {
		t.Errorf("expected total realized 250, got %f", snap.TotalRealized)
	}

	// Unrealized: remaining 5 BTC, cost basis $100/unit, current price $150
	// (150 - 100) * 5 = 250
	if !almostEqual(snap.TotalUnrealized, 250, 0.01) {
		t.Errorf("expected total unrealized 250, got %f", snap.TotalUnrealized)
	}

	if !almostEqual(snap.TotalPnL, 500, 0.01) {
		t.Errorf("expected total PnL 500, got %f", snap.TotalPnL)
	}

	if len(snap.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(snap.Assets))
	}
	if snap.Assets[0].AssetSymbol != "BTC" {
		t.Errorf("expected asset BTC, got %s", snap.Assets[0].AssetSymbol)
	}
	if snap.Assets[0].TransactionCount != 2 {
		t.Errorf("expected 2 transactions, got %d", snap.Assets[0].TransactionCount)
	}

	// Verify snapshot was persisted.
	if len(pnlRepo.upserted) != 1 {
		t.Fatalf("expected 1 upserted snapshot, got %d", len(pnlRepo.upserted))
	}
}

func TestPnLEngine_MultipleLots(t *testing.T) {
	// Buy 5 BTC @ $100 ($500), buy 5 BTC @ $200 ($1000), sell 7 BTC @ $300 ($2100)
	// FIFO: consumes all 5 from first lot + 2 from second lot
	// Realized = (300-100)*5 + (300-200)*2 = 1000 + 200 = 1200
	userID := uuid.New()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	txs := []domain.Transaction{
		makeTx("BTC", "buy", "hourly", 5, refVal(500), t0),
		makeTx("BTC", "buy", "hourly", 5, refVal(1000), t1),
		makeTx("BTC", "sell", "hourly", 7, refVal(2100), t2),
	}
	for i := range txs {
		txs[i].UserID = userID
	}

	txRepo := newMockPnLTxRepo(txs)
	pnlRepo := &mockPnLRepo{}
	ticker := &mockTickerProvider{prices: map[string]float64{"BTC:USD": 300}}

	engine := NewPnLEngine(txRepo, pnlRepo, ticker)
	snap, err := engine.ComputeAndStore(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Realized: (300-100)*5 + (300-200)*2 = 1000 + 200 = 1200
	if !almostEqual(snap.TotalRealized, 1200, 0.01) {
		t.Errorf("expected total realized 1200, got %f", snap.TotalRealized)
	}

	// Remaining: 3 BTC from second lot at $200/unit, current price $300
	// Unrealized: (300-200)*3 = 300
	if !almostEqual(snap.TotalUnrealized, 300, 0.01) {
		t.Errorf("expected total unrealized 300, got %f", snap.TotalUnrealized)
	}

	if !almostEqual(snap.TotalPnL, 1500, 0.01) {
		t.Errorf("expected total PnL 1500, got %f", snap.TotalPnL)
	}

	if snap.Assets[0].TransactionCount != 3 {
		t.Errorf("expected 3 transactions, got %d", snap.Assets[0].TransactionCount)
	}
}

func TestPnLEngine_UnrealizedOnly(t *testing.T) {
	// Buy 10 BTC @ $100 ($1000), no sells, current price $150
	// Unrealized = (150-100)*10 = 500
	userID := uuid.New()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	txs := []domain.Transaction{
		makeTx("BTC", "buy", "exact", 10, refVal(1000), t0),
	}
	for i := range txs {
		txs[i].UserID = userID
	}

	txRepo := newMockPnLTxRepo(txs)
	pnlRepo := &mockPnLRepo{}
	ticker := &mockTickerProvider{prices: map[string]float64{"BTC:USD": 150}}

	engine := NewPnLEngine(txRepo, pnlRepo, ticker)
	snap, err := engine.ComputeAndStore(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !almostEqual(snap.TotalRealized, 0, 0.01) {
		t.Errorf("expected total realized 0, got %f", snap.TotalRealized)
	}
	if !almostEqual(snap.TotalUnrealized, 500, 0.01) {
		t.Errorf("expected total unrealized 500, got %f", snap.TotalUnrealized)
	}
	if !almostEqual(snap.TotalPnL, 500, 0.01) {
		t.Errorf("expected total PnL 500, got %f", snap.TotalPnL)
	}
}

func TestPnLEngine_UnresolvableExcluded(t *testing.T) {
	// 2 resolvable buys + 1 unresolvable buy → excluded_count = 1
	userID := uuid.New()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	txs := []domain.Transaction{
		makeTx("BTC", "buy", "hourly", 5, refVal(500), t0),
		makeTx("DOGE", "buy", "unresolvable", 1000, nil, t1),
		makeTx("BTC", "sell", "hourly", 3, refVal(450), t2),
	}
	for i := range txs {
		txs[i].UserID = userID
	}

	txRepo := newMockPnLTxRepo(txs)
	pnlRepo := &mockPnLRepo{}
	ticker := &mockTickerProvider{prices: map[string]float64{"BTC:USD": 150}}

	engine := NewPnLEngine(txRepo, pnlRepo, ticker)
	snap, err := engine.ComputeAndStore(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.ExcludedCount != 1 {
		t.Errorf("expected excluded_count 1, got %d", snap.ExcludedCount)
	}

	// Only BTC transactions should be in the computation.
	// Realized: sell 3 BTC @ $150/unit (450/3=150), cost $100/unit (500/5=100)
	// (150-100)*3 = 150
	if !almostEqual(snap.TotalRealized, 150, 0.01) {
		t.Errorf("expected total realized 150, got %f", snap.TotalRealized)
	}

	// Remaining: 2 BTC at $100/unit, current $150 → (150-100)*2 = 100
	if !almostEqual(snap.TotalUnrealized, 100, 0.01) {
		t.Errorf("expected total unrealized 100, got %f", snap.TotalUnrealized)
	}

	// DOGE should not appear in assets (it was excluded).
	for _, a := range snap.Assets {
		if a.AssetSymbol == "DOGE" {
			t.Error("DOGE should be excluded from assets")
		}
	}
}

func TestPnLEngine_HasApproximate(t *testing.T) {
	// Transactions with hourly/daily resolution should set has_approximate = true.
	userID := uuid.New()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	tests := []struct {
		name       string
		resolution string
		wantApprox bool
	}{
		{"hourly resolution", "hourly", true},
		{"daily resolution", "daily", true},
		{"exact resolution", "exact", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs := []domain.Transaction{
				makeTx("ETH", "buy", tt.resolution, 10, refVal(20000), t0),
				makeTx("ETH", "sell", tt.resolution, 5, refVal(12500), t1),
			}
			for i := range txs {
				txs[i].UserID = userID
			}

			txRepo := newMockPnLTxRepo(txs)
			pnlRepo := &mockPnLRepo{}
			ticker := &mockTickerProvider{prices: map[string]float64{"ETH:USD": 2500}}

			engine := NewPnLEngine(txRepo, pnlRepo, ticker)
			snap, err := engine.ComputeAndStore(context.Background(), userID, "USD")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if snap.HasApproximate != tt.wantApprox {
				t.Errorf("expected has_approximate %v, got %v", tt.wantApprox, snap.HasApproximate)
			}
		})
	}
}

func TestPnLEngine_GetSnapshot(t *testing.T) {
	userID := uuid.New()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	txs := []domain.Transaction{
		makeTx("BTC", "buy", "hourly", 1, refVal(100), t0),
	}
	for i := range txs {
		txs[i].UserID = userID
	}

	txRepo := newMockPnLTxRepo(txs)
	pnlRepo := &mockPnLRepo{}
	ticker := &mockTickerProvider{prices: map[string]float64{"BTC:USD": 200}}

	engine := NewPnLEngine(txRepo, pnlRepo, ticker)

	// Before compute, GetSnapshot returns nil.
	snap, err := engine.GetSnapshot(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap != nil {
		t.Error("expected nil snapshot before compute")
	}

	// After compute, GetSnapshot returns the persisted snapshot.
	_, err = engine.ComputeAndStore(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap, err = engine.GetSnapshot(context.Background(), userID, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot after compute")
	}
	if snap.UserID != userID {
		t.Errorf("expected user_id %v, got %v", userID, snap.UserID)
	}
}
