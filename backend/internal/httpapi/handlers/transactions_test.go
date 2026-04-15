package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	portfolioSvc "github.com/block-o/exchangely/backend/internal/portfolio"
	"github.com/google/uuid"
)

// --- Mock repositories for transaction handler tests ---

type mockTransactionRepo struct {
	transactions []portfolio.Transaction
	findByIDFn   func(ctx context.Context, userID, txID uuid.UUID) (*portfolio.Transaction, error)
	updateFn     func(ctx context.Context, tx *portfolio.Transaction) error
}

func (m *mockTransactionRepo) Create(_ context.Context, _ *portfolio.Transaction) error { return nil }
func (m *mockTransactionRepo) Upsert(_ context.Context, _ *portfolio.Transaction) error { return nil }
func (m *mockTransactionRepo) Update(ctx context.Context, tx *portfolio.Transaction) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, tx)
	}
	return nil
}
func (m *mockTransactionRepo) FindByID(ctx context.Context, userID, txID uuid.UUID) (*portfolio.Transaction, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, userID, txID)
	}
	return nil, nil
}
func (m *mockTransactionRepo) ListByUser(_ context.Context, _ uuid.UUID, _ portfolio.ListOptions) ([]portfolio.Transaction, int, error) {
	return m.transactions, len(m.transactions), nil
}
func (m *mockTransactionRepo) DeleteBySourceRef(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (m *mockTransactionRepo) CountByUser(_ context.Context, _ uuid.UUID) (int, error) {
	return len(m.transactions), nil
}
func (m *mockTransactionRepo) DistinctCurrencies(_ context.Context, _ uuid.UUID) ([]string, error) {
	return []string{"USD"}, nil
}

type mockPnLRepo struct {
	snapshot *portfolio.PnLSnapshot
}

func (m *mockPnLRepo) Upsert(_ context.Context, _ *portfolio.PnLSnapshot) error { return nil }
func (m *mockPnLRepo) FindByUser(_ context.Context, _ uuid.UUID, _ string) (*portfolio.PnLSnapshot, error) {
	return m.snapshot, nil
}

type mockTickerProvider struct {
	prices map[string]float64
}

func (m *mockTickerProvider) GetPrice(_ context.Context, asset, quote string) (float64, error) {
	key := asset + "/" + quote
	if p, ok := m.prices[key]; ok {
		return p, nil
	}
	return 0, sql.ErrNoRows
}

// --- Helper to build a TransactionHandler with mocks ---

func newTestTransactionHandler(txRepo *mockTransactionRepo, pnlRepo *mockPnLRepo) *TransactionHandler {
	txService := portfolioSvc.NewTransactionService(txRepo, nil, nil, 30*time.Second)
	pnlEngine := portfolioSvc.NewPnLEngine(txRepo, pnlRepo, &mockTickerProvider{})
	return NewTransactionHandler(txService, pnlEngine)
}

// txJWTRequest creates an HTTP request with JWT claims in context.
func txJWTRequest(method, path string, body *bytes.Buffer, userID uuid.UUID) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	claims := &auth.Claims{
		Sub:   userID.String(),
		Email: "test@exchangely.io",
		Role:  "user",
	}
	ctx := middleware.ContextWithClaims(req.Context(), claims)
	return req.WithContext(ctx)
}

// txAPITokenRequest creates an HTTP request with API token auth method.
func txAPITokenRequest(method, path string, body *bytes.Buffer, userID uuid.UUID) *http.Request {
	req := txJWTRequest(method, path, body, userID)
	ctx := middleware.ContextWithAuthMethod(req.Context(), "api_token")
	return req.WithContext(ctx)
}

// --- Tests ---

func TestListTransactions_200(t *testing.T) {
	userID := uuid.New()
	now := time.Now().UTC()
	refVal := 1000.0

	txRepo := &mockTransactionRepo{
		transactions: []portfolio.Transaction{
			{
				ID:                uuid.New(),
				UserID:            userID,
				AssetSymbol:       "BTC",
				Quantity:          0.5,
				Type:              "buy",
				Timestamp:         now,
				Source:            "binance",
				ReferenceValue:    &refVal,
				ReferenceCurrency: "USD",
				Resolution:        "hourly",
			},
		},
	}
	handler := newTestTransactionHandler(txRepo, &mockPnLRepo{})

	req := txJWTRequest(http.MethodGet, "/api/v1/portfolio/transactions?page=1&page_size=10", nil, userID)
	rr := httptest.NewRecorder()
	handler.ListTransactions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected 'data' array, got %T", resp["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(data))
	}

	total, ok := resp["total"].(float64)
	if !ok || int(total) != 1 {
		t.Errorf("expected total=1, got %v", resp["total"])
	}
}

func TestListTransactions_401_Unauthenticated(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/transactions", nil)
	rr := httptest.NewRecorder()
	handler.ListTransactions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestListTransactions_403_APIToken(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})
	userID := uuid.New()

	req := txAPITokenRequest(http.MethodGet, "/api/v1/portfolio/transactions", nil, userID)
	rr := httptest.NewRecorder()
	handler.ListTransactions(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestListTransactions_InvalidStartParam(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})
	userID := uuid.New()

	req := txJWTRequest(http.MethodGet, "/api/v1/portfolio/transactions?start=not-a-date", nil, userID)
	rr := httptest.NewRecorder()
	handler.ListTransactions(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestUpdateTransaction_200(t *testing.T) {
	userID := uuid.New()
	txID := uuid.New()
	refVal := 500.0

	txRepo := &mockTransactionRepo{
		findByIDFn: func(_ context.Context, uid, tid uuid.UUID) (*portfolio.Transaction, error) {
			if uid == userID && tid == txID {
				return &portfolio.Transaction{
					ID:             txID,
					UserID:         userID,
					AssetSymbol:    "ETH",
					Quantity:       2.0,
					Type:           "buy",
					Timestamp:      time.Now().UTC(),
					ReferenceValue: &refVal,
				}, nil
			}
			return nil, nil
		},
		updateFn: func(_ context.Context, _ *portfolio.Transaction) error {
			return nil
		},
	}
	handler := newTestTransactionHandler(txRepo, &mockPnLRepo{})

	body := bytes.NewBufferString(`{"reference_value": 600.0, "notes": "corrected"}`)
	req := txJWTRequest(http.MethodPut, "/api/v1/portfolio/transactions/"+txID.String(), body, userID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.UpdateTransaction(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateTransaction_404_NotFound(t *testing.T) {
	userID := uuid.New()
	txID := uuid.New()

	txRepo := &mockTransactionRepo{
		findByIDFn: func(_ context.Context, _, _ uuid.UUID) (*portfolio.Transaction, error) {
			return nil, nil
		},
	}
	handler := newTestTransactionHandler(txRepo, &mockPnLRepo{})

	body := bytes.NewBufferString(`{"reference_value": 100.0}`)
	req := txJWTRequest(http.MethodPut, "/api/v1/portfolio/transactions/"+txID.String(), body, userID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.UpdateTransaction(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateTransaction_400_NoFields(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})
	userID := uuid.New()

	body := bytes.NewBufferString(`{}`)
	req := txJWTRequest(http.MethodPut, "/api/v1/portfolio/transactions/"+uuid.New().String(), body, userID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.UpdateTransaction(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateTransaction_401_Unauthenticated(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})

	body := bytes.NewBufferString(`{"reference_value": 100.0}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/portfolio/transactions/"+uuid.New().String(), body)
	rr := httptest.NewRecorder()
	handler.UpdateTransaction(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestUpdateTransaction_403_APIToken(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})
	userID := uuid.New()

	body := bytes.NewBufferString(`{"reference_value": 100.0}`)
	req := txAPITokenRequest(http.MethodPut, "/api/v1/portfolio/transactions/"+uuid.New().String(), body, userID)
	rr := httptest.NewRecorder()
	handler.UpdateTransaction(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestGetPnL_200_WithSnapshot(t *testing.T) {
	userID := uuid.New()
	now := time.Now().UTC()

	pnlRepo := &mockPnLRepo{
		snapshot: &portfolio.PnLSnapshot{
			ID:                uuid.New(),
			UserID:            userID,
			ReferenceCurrency: "USD",
			TotalRealized:     150.0,
			TotalUnrealized:   50.0,
			TotalPnL:          200.0,
			HasApproximate:    true,
			ExcludedCount:     2,
			Assets: []portfolio.AssetPnL{
				{
					AssetSymbol:      "BTC",
					RealizedPnL:      100.0,
					UnrealizedPnL:    30.0,
					TotalPnL:         130.0,
					TransactionCount: 5,
				},
			},
			ComputedAt: now,
		},
	}
	handler := newTestTransactionHandler(&mockTransactionRepo{}, pnlRepo)

	req := txJWTRequest(http.MethodGet, "/api/v1/portfolio/pnl?quote=USD", nil, userID)
	rr := httptest.NewRecorder()
	handler.GetPnL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["total_pnl"].(float64) != 200.0 {
		t.Errorf("expected total_pnl=200, got %v", resp["total_pnl"])
	}
	if resp["has_approximate"].(bool) != true {
		t.Error("expected has_approximate=true")
	}
}

func TestGetPnL_200_NoSnapshot(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})
	userID := uuid.New()

	req := txJWTRequest(http.MethodGet, "/api/v1/portfolio/pnl", nil, userID)
	rr := httptest.NewRecorder()
	handler.GetPnL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["total_pnl"].(float64) != 0 {
		t.Errorf("expected total_pnl=0 when no snapshot, got %v", resp["total_pnl"])
	}
}

func TestGetPnL_401_Unauthenticated(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/pnl", nil)
	rr := httptest.NewRecorder()
	handler.GetPnL(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestGetPnL_403_APIToken(t *testing.T) {
	handler := newTestTransactionHandler(&mockTransactionRepo{}, &mockPnLRepo{})
	userID := uuid.New()

	req := txAPITokenRequest(http.MethodGet, "/api/v1/portfolio/pnl", nil, userID)
	rr := httptest.NewRecorder()
	handler.GetPnL(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestGetPnL_DefaultQuoteCurrency(t *testing.T) {
	pnlRepo := &mockPnLRepo{
		snapshot: &portfolio.PnLSnapshot{
			ID:                uuid.New(),
			UserID:            uuid.New(),
			ReferenceCurrency: "USD",
			TotalPnL:          0,
			Assets:            []portfolio.AssetPnL{},
			ComputedAt:        time.Now().UTC(),
		},
	}
	handler := newTestTransactionHandler(&mockTransactionRepo{}, pnlRepo)
	userID := uuid.New()

	// No quote param — should default to USD.
	req := txJWTRequest(http.MethodGet, "/api/v1/portfolio/pnl", nil, userID)
	rr := httptest.NewRecorder()
	handler.GetPnL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
