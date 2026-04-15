package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/block-o/exchangely/backend/internal/auth"
	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/httpapi/middleware"
	"github.com/block-o/exchangely/backend/internal/portfolio"
	"github.com/block-o/exchangely/backend/internal/portfolio/exchange"
	"github.com/block-o/exchangely/backend/internal/portfolio/ledger"
	"github.com/block-o/exchangely/backend/internal/portfolio/wallet"
)

// --- Mocks for sync hook tests ---

type hookCredentialRepo struct {
	cred *domain.ExchangeCredential
}

func (r *hookCredentialRepo) Create(_ context.Context, _ *domain.ExchangeCredential) error {
	return nil
}
func (r *hookCredentialRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.ExchangeCredential, error) {
	return r.cred, nil
}
func (r *hookCredentialRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]domain.ExchangeCredential, error) {
	if r.cred != nil {
		return []domain.ExchangeCredential{*r.cred}, nil
	}
	return nil, nil
}
func (r *hookCredentialRepo) Delete(_ context.Context, _, _ uuid.UUID) error { return nil }
func (r *hookCredentialRepo) UpdateSyncStatus(_ context.Context, _ uuid.UUID, _ string, _ *string, _ *time.Time) error {
	return nil
}

type hookWalletRepo struct {
	w *domain.WalletAddress
}

func (r *hookWalletRepo) Create(_ context.Context, _ *domain.WalletAddress) error { return nil }
func (r *hookWalletRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.WalletAddress, error) {
	return r.w, nil
}
func (r *hookWalletRepo) ListByUserID(_ context.Context, _ uuid.UUID) ([]domain.WalletAddress, error) {
	if r.w != nil {
		return []domain.WalletAddress{*r.w}, nil
	}
	return nil, nil
}
func (r *hookWalletRepo) Delete(_ context.Context, _, _ uuid.UUID) error { return nil }
func (r *hookWalletRepo) UpdateSyncTime(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}

type hookExchangeConnector struct{}

func (c *hookExchangeConnector) Name() string { return "testexchange" }
func (c *hookExchangeConnector) FetchBalances(_ context.Context, _, _ string) ([]exchange.Balance, error) {
	return []exchange.Balance{{Asset: "BTC", Quantity: 1.0}}, nil
}

type hookWalletConnector struct{}

func (c *hookWalletConnector) Chain() string { return "ethereum" }
func (c *hookWalletConnector) FetchBalances(_ context.Context, _ string) ([]wallet.Balance, error) {
	return []wallet.Balance{{Asset: "ETH", Quantity: 2.0}}, nil
}

type hookLedgerConnector struct{}

func (c *hookLedgerConnector) ParseExport(_ []byte) ([]ledger.Balance, error) {
	return []ledger.Balance{{Asset: "BTC", Quantity: 0.5}}, nil
}

// countingPublisher tracks how many times Publish is called.
type countingPublisher struct {
	count atomic.Int32
}

func (p *countingPublisher) Publish(_ context.Context, _ []task.Task) error {
	p.count.Add(1)
	return nil
}

// syncHookSetup holds the test fixtures with known IDs.
type syncHookSetup struct {
	handler   *PortfolioHandler
	publisher *countingPublisher
	credID    uuid.UUID
	walletID  uuid.UUID
}

func newSyncHookTestSetup(userID uuid.UUID) syncHookSetup {
	enc, _ := portfolio.NewKeyEncryptionService("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	keyCipher, keyNonce, _ := enc.EncryptForUser(userID, []byte("test-api-key"))
	secretCipher, secretNonce, _ := enc.EncryptForUser(userID, []byte("test-api-secret"))

	credID := uuid.New()
	cred := &domain.ExchangeCredential{
		ID:           credID,
		UserID:       userID,
		Exchange:     "testexchange",
		APIKeyPrefix: "test-api",
		APIKeyCipher: keyCipher,
		KeyNonce:     keyNonce,
		SecretCipher: secretCipher,
		Nonce:        secretNonce,
		Status:       "active",
	}

	addrCipher, addrNonce, _ := enc.EncryptForUser(userID, []byte("0x1234567890abcdef"))

	walletID := uuid.New()
	w := &domain.WalletAddress{
		ID:            walletID,
		UserID:        userID,
		Chain:         "ethereum",
		AddressPrefix: "0x123456",
		AddressCipher: addrCipher,
		AddressNonce:  addrNonce,
	}

	holdingRepo := &sseHoldingRepo{}
	credRepo := &hookCredentialRepo{cred: cred}
	walletRepo := &hookWalletRepo{w: w}

	exchangeMap := map[string]exchange.Connector{
		"testexchange": &hookExchangeConnector{},
	}
	walletMap := map[string]wallet.WalletConnector{
		"ethereum": &hookWalletConnector{},
	}
	catalog := map[string]bool{"BTC": true, "ETH": true}

	svc := portfolio.NewPortfolioService(
		holdingRepo, credRepo, walletRepo, enc,
		exchangeMap, walletMap, &hookLedgerConnector{}, catalog,
	)

	publisher := &countingPublisher{}
	txService := portfolio.NewTransactionService(nil, nil, publisher, 0)

	ve := portfolio.NewValuationEngine(nil, holdingRepo)
	ph := NewPortfolioHandler(svc, ve).WithTransactionService(txService)

	return syncHookSetup{
		handler:   ph,
		publisher: publisher,
		credID:    credID,
		walletID:  walletID,
	}
}

func syncHookJWTRequest(method, path string, body *bytes.Buffer, userID uuid.UUID) *http.Request {
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

// --- Tests ---

func TestSyncCredential_QueuesRecompute(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)

	path := "/api/v1/portfolio/credentials/" + s.credID.String() + "/sync"
	req := syncHookJWTRequest(http.MethodPost, path, nil, userID)
	rr := httptest.NewRecorder()

	s.handler.SyncCredential(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if s.publisher.count.Load() != 1 {
		t.Fatalf("expected QueueRecompute to be called once, got %d", s.publisher.count.Load())
	}
}

func TestSyncWallet_QueuesRecompute(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)

	path := "/api/v1/portfolio/wallets/" + s.walletID.String() + "/sync"
	req := syncHookJWTRequest(http.MethodPost, path, nil, userID)
	rr := httptest.NewRecorder()

	s.handler.SyncWallet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if s.publisher.count.Load() != 1 {
		t.Fatalf("expected QueueRecompute to be called once, got %d", s.publisher.count.Load())
	}
}

func TestConnectLedger_QueuesRecompute(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "ledger.json")
	_, _ = part.Write([]byte(`{}`))
	_ = writer.Close()

	req := syncHookJWTRequest(http.MethodPost, "/api/v1/portfolio/ledger/connect", &buf, userID)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	s.handler.ConnectLedger(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if s.publisher.count.Load() != 1 {
		t.Fatalf("expected QueueRecompute to be called once, got %d", s.publisher.count.Load())
	}
}

func TestSyncAll_QueuesRecompute(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)

	req := syncHookJWTRequest(http.MethodPost, "/api/v1/portfolio/sync-all", nil, userID)
	rr := httptest.NewRecorder()

	s.handler.SyncAll(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if s.publisher.count.Load() != 1 {
		t.Fatalf("expected QueueRecompute to be called once after SyncAll, got %d", s.publisher.count.Load())
	}
}

func TestSyncCredential_NoRecompute_WhenNoTransactionService(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)
	s.handler.txService = nil

	path := "/api/v1/portfolio/credentials/" + s.credID.String() + "/sync"
	req := syncHookJWTRequest(http.MethodPost, path, nil, userID)
	rr := httptest.NewRecorder()

	s.handler.SyncCredential(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if s.publisher.count.Load() != 0 {
		t.Fatalf("expected no QueueRecompute calls when txService is nil, got %d", s.publisher.count.Load())
	}
}

func TestRecompute_QueuesRecompute(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)

	req := syncHookJWTRequest(http.MethodPost, "/api/v1/portfolio/recompute", nil, userID)
	rr := httptest.NewRecorder()

	s.handler.Recompute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "queued" {
		t.Errorf("expected status 'queued', got %q", resp["status"])
	}
	if s.publisher.count.Load() != 1 {
		t.Fatalf("expected QueueRecompute to be called once, got %d", s.publisher.count.Load())
	}
}

func TestRecompute_NoRecompute_WhenNoTransactionService(t *testing.T) {
	userID := uuid.New()
	s := newSyncHookTestSetup(userID)
	s.handler.txService = nil

	req := syncHookJWTRequest(http.MethodPost, "/api/v1/portfolio/recompute", nil, userID)
	rr := httptest.NewRecorder()

	s.handler.Recompute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if s.publisher.count.Load() != 0 {
		t.Fatalf("expected no QueueRecompute calls when txService is nil, got %d", s.publisher.count.Load())
	}
}
