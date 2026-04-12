package portfolio

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/portfolio/exchange"
	"github.com/block-o/exchangely/backend/internal/portfolio/ledger"
	"github.com/block-o/exchangely/backend/internal/portfolio/wallet"
)

// --- Test catalog and master key ---

var testCatalog = map[string]bool{
	"BTC": true, "ETH": true, "SOL": true, "USDT": true, "USDC": true,
}

var testCatalogSymbols = []string{"BTC", "ETH", "SOL", "USDT", "USDC"}

// --- Mock: HoldingRepository ---

type mockHoldingRepo struct {
	mu       sync.Mutex
	holdings map[uuid.UUID]*domain.Holding // keyed by holding ID
}

func newMockHoldingRepo() *mockHoldingRepo {
	return &mockHoldingRepo{holdings: make(map[uuid.UUID]*domain.Holding)}
}

func (r *mockHoldingRepo) Create(_ context.Context, h *domain.Holding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *h
	r.holdings[h.ID] = &cp
	return nil
}

func (r *mockHoldingRepo) Update(_ context.Context, h *domain.Holding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.holdings[h.ID]; !ok {
		return fmt.Errorf("holding not found")
	}
	cp := *h
	r.holdings[h.ID] = &cp
	return nil
}

func (r *mockHoldingRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.holdings[id]
	if !ok || h.UserID != userID {
		return fmt.Errorf("holding not found")
	}
	delete(r.holdings, id)
	return nil
}

func (r *mockHoldingRepo) FindByID(_ context.Context, id, userID uuid.UUID) (*domain.Holding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.holdings[id]
	if !ok {
		return nil, nil
	}
	if h.UserID != userID {
		return nil, nil
	}
	cp := *h
	return &cp, nil
}

func (r *mockHoldingRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]domain.Holding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Holding
	for _, h := range r.holdings {
		if h.UserID == userID {
			result = append(result, *h)
		}
	}
	return result, nil
}

func (r *mockHoldingRepo) UpsertBySource(_ context.Context, userID uuid.UUID, source, sourceRef string, holdings []domain.Holding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Remove existing holdings for this source+sourceRef.
	for id, h := range r.holdings {
		if h.UserID == userID && h.Source == source && h.SourceRef != nil && *h.SourceRef == sourceRef {
			delete(r.holdings, id)
		}
	}
	// Insert new holdings.
	for i := range holdings {
		h := holdings[i]
		h.ID = uuid.New()
		h.CreatedAt = time.Now()
		h.UpdatedAt = time.Now()
		cp := h
		r.holdings[h.ID] = &cp
	}
	return nil
}

func (r *mockHoldingRepo) DeleteBySourceRef(_ context.Context, userID uuid.UUID, sourceRef string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, h := range r.holdings {
		if h.UserID == userID && h.SourceRef != nil && *h.SourceRef == sourceRef {
			delete(r.holdings, id)
		}
	}
	return nil
}

func (r *mockHoldingRepo) DeleteBySource(_ context.Context, userID uuid.UUID, source string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, h := range r.holdings {
		if h.UserID == userID && h.Source == source {
			delete(r.holdings, id)
		}
	}
	return nil
}

// --- Mock: CredentialRepository ---

type mockCredentialRepo struct {
	mu    sync.Mutex
	creds map[uuid.UUID]*domain.ExchangeCredential
}

func newMockCredentialRepo() *mockCredentialRepo {
	return &mockCredentialRepo{creds: make(map[uuid.UUID]*domain.ExchangeCredential)}
}

func (r *mockCredentialRepo) Create(_ context.Context, c *domain.ExchangeCredential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *c
	r.creds[c.ID] = &cp
	return nil
}

func (r *mockCredentialRepo) FindByID(_ context.Context, id, userID uuid.UUID) (*domain.ExchangeCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.creds[id]
	if !ok {
		return nil, nil
	}
	if c.UserID != userID {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (r *mockCredentialRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]domain.ExchangeCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.ExchangeCredential
	for _, c := range r.creds {
		if c.UserID == userID {
			result = append(result, *c)
		}
	}
	return result, nil
}

func (r *mockCredentialRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.creds[id]
	if !ok || c.UserID != userID {
		return fmt.Errorf("credential not found")
	}
	delete(r.creds, id)
	return nil
}

func (r *mockCredentialRepo) UpdateSyncStatus(_ context.Context, id uuid.UUID, status string, errorReason *string, syncTime *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.creds[id]
	if !ok {
		return fmt.Errorf("credential not found")
	}
	c.Status = status
	c.ErrorReason = errorReason
	c.LastSyncAt = syncTime
	return nil
}

// --- Mock: WalletRepository ---

type mockWalletRepo struct {
	mu      sync.Mutex
	wallets map[uuid.UUID]*domain.WalletAddress
}

func newMockWalletRepo() *mockWalletRepo {
	return &mockWalletRepo{wallets: make(map[uuid.UUID]*domain.WalletAddress)}
}

func (r *mockWalletRepo) Create(_ context.Context, w *domain.WalletAddress) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *w
	r.wallets[w.ID] = &cp
	return nil
}

func (r *mockWalletRepo) FindByID(_ context.Context, id, userID uuid.UUID) (*domain.WalletAddress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.wallets[id]
	if !ok {
		return nil, nil
	}
	if w.UserID != userID {
		return nil, nil
	}
	cp := *w
	return &cp, nil
}

func (r *mockWalletRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]domain.WalletAddress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.WalletAddress
	for _, w := range r.wallets {
		if w.UserID == userID {
			result = append(result, *w)
		}
	}
	return result, nil
}

func (r *mockWalletRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.wallets[id]
	if !ok || w.UserID != userID {
		return fmt.Errorf("wallet not found")
	}
	delete(r.wallets, id)
	return nil
}

func (r *mockWalletRepo) UpdateSyncTime(_ context.Context, id uuid.UUID, syncTime time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.wallets[id]
	if !ok {
		return fmt.Errorf("wallet not found")
	}
	w.LastSyncAt = &syncTime
	return nil
}

// --- Mock: ExchangeConnector ---

type mockExchangeConnector struct {
	name     string
	balances []exchange.Balance
}

func (c *mockExchangeConnector) FetchBalances(_ context.Context, _, _ string) ([]exchange.Balance, error) {
	return c.balances, nil
}

func (c *mockExchangeConnector) Name() string { return c.name }

// --- Mock: WalletConnector ---

type mockWalletConnector struct {
	chain    string
	balances []wallet.Balance
}

func (c *mockWalletConnector) FetchBalances(_ context.Context, _ string) ([]wallet.Balance, error) {
	return c.balances, nil
}

func (c *mockWalletConnector) Chain() string { return c.chain }

// --- Mock: LedgerConnector ---

type mockLedgerConnector struct {
	balances []ledger.Balance
}

func (c *mockLedgerConnector) ParseExport(_ []byte) ([]ledger.Balance, error) {
	return c.balances, nil
}

// --- Test service factory ---

func newTestService(
	holdingRepo *mockHoldingRepo,
	credRepo *mockCredentialRepo,
	walletRepo *mockWalletRepo,
	exchangeMap map[string]exchange.Connector,
	walletMap map[string]wallet.WalletConnector,
	ledgerConn ledger.LedgerConnector,
) *PortfolioService {
	enc := newTestEncryptionService(&fatalHelper{})
	return NewPortfolioService(
		holdingRepo,
		credRepo,
		walletRepo,
		enc,
		exchangeMap,
		walletMap,
		ledgerConn,
		testCatalog,
	)
}

// fatalHelper satisfies the interface needed by newTestEncryptionService.
type fatalHelper struct{}

func (f *fatalHelper) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

// --- Rapid generators ---

func genCatalogSymbol(t *rapid.T, label string) string {
	idx := rapid.IntRange(0, len(testCatalogSymbols)-1).Draw(t, label)
	return testCatalogSymbols[idx]
}

func genPositiveFloat(t *rapid.T, label string) float64 {
	// Generate a positive float in a reasonable range.
	return rapid.Float64Range(0.0001, 1_000_000.0).Draw(t, label)
}

// Feature: portfolio-tracker, Property 6: Holding CRUD round-trip
//
// For any valid holding (positive quantity, asset in catalog), creating it via
// the service and then fetching it by ID returns a holding with all fields
// matching the original input.
func TestPropertyHoldingCRUDRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		holdingRepo := newMockHoldingRepo()
		svc := newTestService(holdingRepo, newMockCredentialRepo(), newMockWalletRepo(), nil, nil, nil)
		ctx := context.Background()

		userID := uuid.New()
		symbol := genCatalogSymbol(t, "symbol")
		qty := genPositiveFloat(t, "quantity")

		h := &domain.Holding{
			AssetSymbol:   symbol,
			Quantity:      qty,
			QuoteCurrency: "USD",
			Source:        "manual",
		}

		if err := svc.CreateHolding(ctx, userID, h); err != nil {
			t.Fatalf("CreateHolding failed: %v", err)
		}

		fetched, err := svc.GetHolding(ctx, userID, h.ID)
		if err != nil {
			t.Fatalf("GetHolding failed: %v", err)
		}
		if fetched == nil {
			t.Fatal("GetHolding returned nil")
			return // unreachable but satisfies staticcheck
		}

		if fetched.ID != h.ID {
			t.Fatalf("ID mismatch: got %s, want %s", fetched.ID, h.ID)
		}
		if fetched.UserID != userID {
			t.Fatalf("UserID mismatch: got %s, want %s", fetched.UserID, userID)
		}
		if fetched.AssetSymbol != symbol {
			t.Fatalf("AssetSymbol mismatch: got %q, want %q", fetched.AssetSymbol, symbol)
		}
		if fetched.Quantity != qty {
			t.Fatalf("Quantity mismatch: got %f, want %f", fetched.Quantity, qty)
		}
		if fetched.QuoteCurrency != "USD" {
			t.Fatalf("QuoteCurrency mismatch: got %q, want %q", fetched.QuoteCurrency, "USD")
		}
		if fetched.Source != "manual" {
			t.Fatalf("Source mismatch: got %q, want %q", fetched.Source, "manual")
		}
	})
}

// Feature: portfolio-tracker, Property 9: Cross-user data isolation
//
// For any two distinct user IDs and a holding owned by user A, user B's
// read/update/delete attempts return ErrForbidden.
func TestPropertyCrossUserDataIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		holdingRepo := newMockHoldingRepo()
		svc := newTestService(holdingRepo, newMockCredentialRepo(), newMockWalletRepo(), nil, nil, nil)
		ctx := context.Background()

		userA := uuid.New()
		userB := uuid.New()
		for userA == userB {
			userB = uuid.New()
		}

		symbol := genCatalogSymbol(t, "symbol")
		qty := genPositiveFloat(t, "quantity")

		h := &domain.Holding{
			AssetSymbol:   symbol,
			Quantity:      qty,
			QuoteCurrency: "USD",
			Source:        "manual",
		}

		if err := svc.CreateHolding(ctx, userA, h); err != nil {
			t.Fatalf("CreateHolding for userA failed: %v", err)
		}

		// User B read attempt.
		got, err := svc.GetHolding(ctx, userB, h.ID)
		if err == nil && got != nil {
			t.Fatal("expected forbidden error for user B read, got success")
		}
		if err != nil && err != ErrForbidden {
			t.Fatalf("expected ErrForbidden for user B read, got: %v", err)
		}

		// User B update attempt.
		updateH := &domain.Holding{
			ID:            h.ID,
			AssetSymbol:   symbol,
			Quantity:      qty + 1,
			QuoteCurrency: "USD",
			Source:        "manual",
		}
		err = svc.UpdateHolding(ctx, userB, updateH)
		if err == nil {
			t.Fatal("expected forbidden error for user B update, got success")
		}
		if err != ErrForbidden {
			t.Fatalf("expected ErrForbidden for user B update, got: %v", err)
		}

		// User B delete attempt.
		err = svc.DeleteHolding(ctx, userB, h.ID)
		if err == nil {
			t.Fatal("expected forbidden error for user B delete, got success")
		}
		if err != ErrForbidden {
			t.Fatalf("expected ErrForbidden for user B delete, got: %v", err)
		}

		// Verify holding is unchanged for user A.
		fetched, err := svc.GetHolding(ctx, userA, h.ID)
		if err != nil {
			t.Fatalf("GetHolding for userA after B's attempts failed: %v", err)
		}
		if fetched.Quantity != qty {
			t.Fatalf("holding was modified by user B: got qty %f, want %f", fetched.Quantity, qty)
		}
	})
}

// Feature: portfolio-tracker, Property 10: Holdings list completeness
//
// For any set of N holdings created for a user, listing holdings returns
// exactly N holdings with matching IDs.
func TestPropertyHoldingsListCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		holdingRepo := newMockHoldingRepo()
		svc := newTestService(holdingRepo, newMockCredentialRepo(), newMockWalletRepo(), nil, nil, nil)
		ctx := context.Background()

		userID := uuid.New()
		n := rapid.IntRange(0, 20).Draw(t, "n")

		createdIDs := make(map[uuid.UUID]bool, n)
		for i := 0; i < n; i++ {
			symbol := genCatalogSymbol(t, fmt.Sprintf("symbol_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("qty_%d", i))

			h := &domain.Holding{
				AssetSymbol:   symbol,
				Quantity:      qty,
				QuoteCurrency: "USD",
				Source:        "manual",
			}
			if err := svc.CreateHolding(ctx, userID, h); err != nil {
				t.Fatalf("CreateHolding %d failed: %v", i, err)
			}
			createdIDs[h.ID] = true
		}

		listed, err := svc.ListHoldings(ctx, userID)
		if err != nil {
			t.Fatalf("ListHoldings failed: %v", err)
		}

		if len(listed) != n {
			t.Fatalf("expected %d holdings, got %d", n, len(listed))
		}

		for _, h := range listed {
			if !createdIDs[h.ID] {
				t.Fatalf("listed holding %s was not in the created set", h.ID)
			}
		}
	})
}

// Feature: portfolio-tracker, Property 11: Sensitive data hidden in listings
//
// For any stored credentials and wallets, listing them returns only prefixes
// and metadata — cipher fields are nil.
func TestPropertySensitiveDataHiddenInListings(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		credRepo := newMockCredentialRepo()
		walletRepo := newMockWalletRepo()
		svc := newTestService(newMockHoldingRepo(), credRepo, walletRepo, nil, nil, nil)
		ctx := context.Background()

		userID := uuid.New()

		// Generate and store a credential.
		numCreds := rapid.IntRange(1, 5).Draw(t, "numCreds")
		exchanges := []string{"binance", "kraken", "coinbase"}
		for i := 0; i < numCreds; i++ {
			exchIdx := rapid.IntRange(0, len(exchanges)-1).Draw(t, fmt.Sprintf("exchIdx_%d", i))
			apiKey := rapid.StringMatching(`[a-zA-Z0-9]{16,32}`).Draw(t, fmt.Sprintf("apiKey_%d", i))
			apiSecret := rapid.StringMatching(`[a-zA-Z0-9]{32,64}`).Draw(t, fmt.Sprintf("apiSecret_%d", i))

			_, err := svc.CreateCredential(ctx, userID, exchanges[exchIdx], apiKey, apiSecret)
			if err != nil {
				t.Fatalf("CreateCredential %d failed: %v", i, err)
			}
		}

		// List credentials and verify cipher fields are nil.
		creds, err := svc.ListCredentials(ctx, userID)
		if err != nil {
			t.Fatalf("ListCredentials failed: %v", err)
		}
		if len(creds) != numCreds {
			t.Fatalf("expected %d credentials, got %d", numCreds, len(creds))
		}
		for _, c := range creds {
			if c.APIKeyCipher != nil {
				t.Fatal("APIKeyCipher should be nil in listing")
			}
			if c.SecretCipher != nil {
				t.Fatal("SecretCipher should be nil in listing")
			}
			if c.Nonce != nil {
				t.Fatal("Nonce should be nil in listing")
			}
			if c.KeyNonce != nil {
				t.Fatal("KeyNonce should be nil in listing")
			}
			if c.APIKeyPrefix == "" {
				t.Fatal("APIKeyPrefix should not be empty")
			}
			if len(c.APIKeyPrefix) > 8 {
				t.Fatalf("APIKeyPrefix should be at most 8 chars, got %d", len(c.APIKeyPrefix))
			}
		}

		// Generate and store wallets.
		numWallets := rapid.IntRange(1, 5).Draw(t, "numWallets")
		for i := 0; i < numWallets; i++ {
			// Use a valid Ethereum address for simplicity.
			addrHex := rapid.StringMatching(`[0-9a-f]{40}`).Draw(t, fmt.Sprintf("addrHex_%d", i))
			address := "0x" + addrHex
			label := rapid.StringMatching(`[a-zA-Z ]{0,20}`).Draw(t, fmt.Sprintf("label_%d", i))

			_, err := svc.CreateWallet(ctx, userID, "ethereum", address, label)
			if err != nil {
				t.Fatalf("CreateWallet %d failed: %v", i, err)
			}
		}

		// List wallets and verify cipher fields are nil.
		wallets, err := svc.ListWallets(ctx, userID)
		if err != nil {
			t.Fatalf("ListWallets failed: %v", err)
		}
		if len(wallets) != numWallets {
			t.Fatalf("expected %d wallets, got %d", numWallets, len(wallets))
		}
		for _, w := range wallets {
			if w.AddressCipher != nil {
				t.Fatal("AddressCipher should be nil in listing")
			}
			if w.AddressNonce != nil {
				t.Fatal("AddressNonce should be nil in listing")
			}
			if w.LabelCipher != nil {
				t.Fatal("LabelCipher should be nil in listing")
			}
			if w.LabelNonce != nil {
				t.Fatal("LabelNonce should be nil in listing")
			}
			if w.AddressPrefix == "" {
				t.Fatal("AddressPrefix should not be empty")
			}
			if len(w.AddressPrefix) > 8 {
				t.Fatalf("AddressPrefix should be at most 8 chars, got %d", len(w.AddressPrefix))
			}
		}
	})
}

// Feature: portfolio-tracker, Property 12: Unknown assets skipped during sync
//
// For any balance list with a mix of catalog and non-catalog assets, the sync
// operation creates holdings only for assets present in the catalog.
func TestPropertyUnknownAssetsSkippedDuringSync(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		holdingRepo := newMockHoldingRepo()
		credRepo := newMockCredentialRepo()
		walletRepo := newMockWalletRepo()

		// Generate a mix of catalog and non-catalog assets for exchange balances.
		numCatalog := rapid.IntRange(0, 5).Draw(t, "numCatalog")
		numUnknown := rapid.IntRange(0, 5).Draw(t, "numUnknown")

		var exchangeBalances []exchange.Balance
		expectedExchangeCount := 0

		for i := 0; i < numCatalog; i++ {
			sym := genCatalogSymbol(t, fmt.Sprintf("catSym_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("catQty_%d", i))
			exchangeBalances = append(exchangeBalances, exchange.Balance{Asset: sym, Quantity: qty})
			expectedExchangeCount++
		}
		for i := 0; i < numUnknown; i++ {
			// Generate a symbol guaranteed not in catalog.
			sym := rapid.StringMatching(`UNKNOWN[A-Z]{3,6}`).Draw(t, fmt.Sprintf("unkSym_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("unkQty_%d", i))
			exchangeBalances = append(exchangeBalances, exchange.Balance{Asset: sym, Quantity: qty})
		}

		mockExch := &mockExchangeConnector{name: "binance", balances: exchangeBalances}
		exchangeMap := map[string]exchange.Connector{"binance": mockExch}

		// Also test wallet sync with mixed balances.
		var walletBalances []wallet.Balance
		expectedWalletCount := 0
		numWalletCatalog := rapid.IntRange(0, 3).Draw(t, "numWalletCatalog")
		numWalletUnknown := rapid.IntRange(0, 3).Draw(t, "numWalletUnknown")

		for i := 0; i < numWalletCatalog; i++ {
			sym := genCatalogSymbol(t, fmt.Sprintf("wCatSym_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("wCatQty_%d", i))
			walletBalances = append(walletBalances, wallet.Balance{Asset: sym, Quantity: qty})
			expectedWalletCount++
		}
		for i := 0; i < numWalletUnknown; i++ {
			sym := rapid.StringMatching(`XUNK[A-Z]{3,6}`).Draw(t, fmt.Sprintf("wUnkSym_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("wUnkQty_%d", i))
			walletBalances = append(walletBalances, wallet.Balance{Asset: sym, Quantity: qty})
		}

		mockWallet := &mockWalletConnector{chain: "ethereum", balances: walletBalances}
		walletMap := map[string]wallet.WalletConnector{"ethereum": mockWallet}

		// Also test ledger import with mixed balances.
		var ledgerBalances []ledger.Balance
		expectedLedgerCount := 0
		numLedgerCatalog := rapid.IntRange(0, 3).Draw(t, "numLedgerCatalog")
		numLedgerUnknown := rapid.IntRange(0, 3).Draw(t, "numLedgerUnknown")

		for i := 0; i < numLedgerCatalog; i++ {
			sym := genCatalogSymbol(t, fmt.Sprintf("lCatSym_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("lCatQty_%d", i))
			ledgerBalances = append(ledgerBalances, ledger.Balance{Asset: sym, Quantity: qty})
			expectedLedgerCount++
		}
		for i := 0; i < numLedgerUnknown; i++ {
			sym := rapid.StringMatching(`ZUNK[A-Z]{3,6}`).Draw(t, fmt.Sprintf("lUnkSym_%d", i))
			qty := genPositiveFloat(t, fmt.Sprintf("lUnkQty_%d", i))
			ledgerBalances = append(ledgerBalances, ledger.Balance{Asset: sym, Quantity: qty})
		}

		mockLedger := &mockLedgerConnector{balances: ledgerBalances}

		svc := newTestService(holdingRepo, credRepo, walletRepo, exchangeMap, walletMap, mockLedger)
		ctx := context.Background()
		userID := uuid.New()
		enc := newTestEncryptionService(&fatalHelper{})

		// --- Exchange sync ---
		apiKey := "testkey12345678"
		apiSecret := "testsecret1234567890"
		keyCipher, keyNonce, _ := enc.EncryptForUser(userID, []byte(apiKey))
		secretCipher, secretNonce, _ := enc.EncryptForUser(userID, []byte(apiSecret))

		credID := uuid.New()
		cred := &domain.ExchangeCredential{
			ID:           credID,
			UserID:       userID,
			Exchange:     "binance",
			APIKeyPrefix: apiKey[:8],
			APIKeyCipher: keyCipher,
			KeyNonce:     keyNonce,
			SecretCipher: secretCipher,
			Nonce:        secretNonce,
			Status:       "active",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		_ = credRepo.Create(ctx, cred)

		if err := svc.SyncCredential(ctx, userID, credID); err != nil {
			t.Fatalf("SyncCredential failed: %v", err)
		}

		// --- Wallet sync ---
		address := "0x1234567890abcdef1234567890abcdef12345678"
		addrCipher, addrNonce, _ := enc.EncryptForUser(userID, []byte(address))
		walletID := uuid.New()
		w := &domain.WalletAddress{
			ID:            walletID,
			UserID:        userID,
			Chain:         "ethereum",
			AddressPrefix: address[:8],
			AddressCipher: addrCipher,
			AddressNonce:  addrNonce,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		_ = walletRepo.Create(ctx, w)

		if err := svc.SyncWallet(ctx, userID, walletID); err != nil {
			t.Fatalf("SyncWallet failed: %v", err)
		}

		// --- Ledger import ---
		// The mock connector returns the generated balances regardless of input data.
		imported, err := svc.ImportLedgerExport(ctx, userID, []byte(`{"accounts":[]}`))
		if err != nil {
			t.Fatalf("ImportLedgerExport failed: %v", err)
		}
		if imported != expectedLedgerCount {
			t.Fatalf("ImportLedgerExport: expected %d imported, got %d", expectedLedgerCount, imported)
		}

		// Verify: all holdings should only contain catalog assets.
		holdings, err := svc.ListHoldings(ctx, userID)
		if err != nil {
			t.Fatalf("ListHoldings failed: %v", err)
		}

		for _, h := range holdings {
			if !testCatalog[h.AssetSymbol] {
				t.Fatalf("holding with non-catalog asset %q found after sync", h.AssetSymbol)
			}
		}

		// Count holdings by source to verify expected counts.
		exchangeCount := 0
		walletCount := 0
		ledgerCount := 0
		for _, h := range holdings {
			switch h.Source {
			case "binance":
				exchangeCount++
			case "ethereum":
				walletCount++
			case "ledger":
				ledgerCount++
			}
		}

		// The number of catalog balances from each source.
		if exchangeCount != expectedExchangeCount {
			t.Fatalf("exchange holdings: expected %d catalog assets, got %d", expectedExchangeCount, exchangeCount)
		}
		if walletCount != expectedWalletCount {
			t.Fatalf("wallet holdings: expected %d catalog assets, got %d", expectedWalletCount, walletCount)
		}
		if ledgerCount != expectedLedgerCount {
			t.Fatalf("ledger holdings: expected %d catalog assets, got %d", expectedLedgerCount, ledgerCount)
		}
	})
}
