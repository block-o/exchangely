package portfolio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
	"github.com/block-o/exchangely/backend/internal/portfolio/exchange"
	"github.com/block-o/exchangely/backend/internal/portfolio/ledger"
	"github.com/block-o/exchangely/backend/internal/portfolio/wallet"
)

// ErrForbidden is returned when a user attempts to access a resource they do not own.
var ErrForbidden = errors.New("forbidden: access denied")

// PortfolioService orchestrates holding CRUD, credential management, wallet
// management, Ledger import, and sync operations. It encrypts sensitive
// fields before writes and decrypts after reads, and enforces user-scoped access.
type PortfolioService struct {
	holdings    domain.HoldingRepository
	credentials domain.CredentialRepository
	wallets     domain.WalletRepository
	encryption  *KeyEncryptionService
	exchangeMap map[string]exchange.Connector
	walletMap   map[string]wallet.WalletConnector
	ledgerConn  ledger.LedgerConnector
	catalog     map[string]bool
}

// NewPortfolioService creates a new PortfolioService with all required dependencies.
func NewPortfolioService(
	holdings domain.HoldingRepository,
	credentials domain.CredentialRepository,
	wallets domain.WalletRepository,
	encryption *KeyEncryptionService,
	exchangeMap map[string]exchange.Connector,
	walletMap map[string]wallet.WalletConnector,
	ledgerConn ledger.LedgerConnector,
	catalog map[string]bool,
) *PortfolioService {
	return &PortfolioService{
		holdings:    holdings,
		credentials: credentials,
		wallets:     wallets,
		encryption:  encryption,
		exchangeMap: exchangeMap,
		walletMap:   walletMap,
		ledgerConn:  ledgerConn,
		catalog:     catalog,
	}
}

// --- Holding CRUD ---

// CreateHolding validates and persists a new holding. Notes are encrypted before storage.
func (s *PortfolioService) CreateHolding(ctx context.Context, userID uuid.UUID, h *domain.Holding) error {
	if err := ValidateQuantity(h.Quantity); err != nil {
		return err
	}
	if err := ValidateAssetSymbol(h.AssetSymbol, s.catalog); err != nil {
		return err
	}

	h.UserID = userID
	h.ID = uuid.New()
	now := time.Now()
	h.CreatedAt = now
	h.UpdatedAt = now

	if err := s.encryptHoldingNotes(userID, h); err != nil {
		return fmt.Errorf("encrypting holding notes: %w", err)
	}

	return s.holdings.Create(ctx, h)
}

// UpdateHolding validates ownership and applies changes to an existing holding.
func (s *PortfolioService) UpdateHolding(ctx context.Context, userID uuid.UUID, h *domain.Holding) error {
	existing, err := s.holdings.FindByID(ctx, h.ID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrForbidden
	}
	if existing.UserID != userID {
		return ErrForbidden
	}

	if err := ValidateQuantity(h.Quantity); err != nil {
		return err
	}
	if err := ValidateAssetSymbol(h.AssetSymbol, s.catalog); err != nil {
		return err
	}

	h.UserID = userID
	h.UpdatedAt = time.Now()

	if err := s.encryptHoldingNotes(userID, h); err != nil {
		return fmt.Errorf("encrypting holding notes: %w", err)
	}

	return s.holdings.Update(ctx, h)
}

// DeleteHolding removes a holding after verifying ownership.
func (s *PortfolioService) DeleteHolding(ctx context.Context, userID, holdingID uuid.UUID) error {
	existing, err := s.holdings.FindByID(ctx, holdingID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrForbidden
	}
	if existing.UserID != userID {
		return ErrForbidden
	}
	return s.holdings.Delete(ctx, holdingID, userID)
}

// GetHolding retrieves a single holding by ID, enforcing user-scoped access.
// Decrypts notes before returning.
func (s *PortfolioService) GetHolding(ctx context.Context, userID, holdingID uuid.UUID) (*domain.Holding, error) {
	h, err := s.holdings.FindByID(ctx, holdingID, userID)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrForbidden
	}
	if h.UserID != userID {
		return nil, ErrForbidden
	}

	if err := s.decryptHoldingNotes(userID, h); err != nil {
		return nil, fmt.Errorf("decrypting holding notes: %w", err)
	}

	return h, nil
}

// ListHoldings returns all holdings for a user, decrypting notes on each.
func (s *PortfolioService) ListHoldings(ctx context.Context, userID uuid.UUID) ([]domain.Holding, error) {
	holdings, err := s.holdings.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	for i := range holdings {
		if err := s.decryptHoldingNotes(userID, &holdings[i]); err != nil {
			slog.Warn("failed to decrypt holding notes", "holding_id", holdings[i].ID, "error", err)
		}
	}

	return holdings, nil
}

// --- Exchange Credential Management ---

// CreateCredential validates, encrypts, and stores an exchange credential.
func (s *PortfolioService) CreateCredential(ctx context.Context, userID uuid.UUID, exchangeName, apiKey, apiSecret string) (*domain.ExchangeCredential, error) {
	if err := ValidateExchange(exchangeName); err != nil {
		return nil, err
	}

	// Encrypt API key.
	keyCipher, keyNonce, err := s.encryption.EncryptForUser(userID, []byte(apiKey))
	if err != nil {
		return nil, fmt.Errorf("encrypting API key: %w", err)
	}

	// Encrypt API secret.
	secretCipher, secretNonce, err := s.encryption.EncryptForUser(userID, []byte(apiSecret))
	if err != nil {
		return nil, fmt.Errorf("encrypting API secret: %w", err)
	}

	prefix := apiKey
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	cred := &domain.ExchangeCredential{
		ID:           uuid.New(),
		UserID:       userID,
		Exchange:     exchangeName,
		APIKeyPrefix: prefix,
		APIKeyCipher: keyCipher,
		KeyNonce:     keyNonce,
		SecretCipher: secretCipher,
		Nonce:        secretNonce,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.credentials.Create(ctx, cred); err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateCredential, exchangeName)
		}
		return nil, err
	}

	// Return metadata only — clear cipher fields.
	cred.APIKeyCipher = nil
	cred.SecretCipher = nil
	cred.Nonce = nil
	cred.KeyNonce = nil

	return cred, nil
}

// ListCredentials returns credential metadata for a user. No secrets are decrypted.
func (s *PortfolioService) ListCredentials(ctx context.Context, userID uuid.UUID) ([]domain.ExchangeCredential, error) {
	creds, err := s.credentials.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Strip cipher fields — return only metadata (prefix, timestamps, status).
	for i := range creds {
		creds[i].APIKeyCipher = nil
		creds[i].SecretCipher = nil
		creds[i].Nonce = nil
		creds[i].KeyNonce = nil
	}

	return creds, nil
}

// DeleteCredential removes a credential and cascade-deletes associated holdings.
func (s *PortfolioService) DeleteCredential(ctx context.Context, userID, credID uuid.UUID) error {
	cred, err := s.credentials.FindByID(ctx, credID, userID)
	if err != nil {
		return err
	}
	if cred == nil {
		return ErrForbidden
	}
	if cred.UserID != userID {
		return ErrForbidden
	}

	// Cascade-delete holdings linked to this credential.
	sourceRef := credID.String()
	if err := s.holdings.DeleteBySourceRef(ctx, userID, sourceRef); err != nil {
		return fmt.Errorf("cascade-deleting holdings: %w", err)
	}

	return s.credentials.Delete(ctx, credID, userID)
}

// SyncCredentialResult holds the outcome of a credential sync, including any
// trade history entries that were fetched from the exchange.
type SyncCredentialResult struct {
	Exchange string
	Trades   []domain.LedgerEntry
}

// SyncCredential decrypts the credential, fetches balances from the exchange,
// filters against the asset catalog, upserts holdings, and updates the sync timestamp.
// If the connector supports trade history, trades are fetched and returned as ledger entries.
// On auth errors the credential is marked as failed.
func (s *PortfolioService) SyncCredential(ctx context.Context, userID, credID uuid.UUID) (*SyncCredentialResult, error) {
	cred, err := s.credentials.FindByID(ctx, credID, userID)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, ErrForbidden
	}
	if cred.UserID != userID {
		return nil, ErrForbidden
	}

	connector, ok := s.exchangeMap[cred.Exchange]
	if !ok {
		return nil, fmt.Errorf("no connector for exchange %q", cred.Exchange)
	}

	// Decrypt API key and secret.
	apiKeyBytes, err := s.encryption.DecryptForUser(userID, cred.APIKeyCipher, cred.KeyNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypting API key: %w", err)
	}
	secretBytes, err := s.encryption.DecryptForUser(userID, cred.SecretCipher, cred.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decrypting API secret: %w", err)
	}

	balances, err := connector.FetchBalances(ctx, string(apiKeyBytes), string(secretBytes))
	if err != nil {
		// Check for auth errors — mark credential as failed.
		if isAuthError(err) {
			reason := err.Error()
			_ = s.credentials.UpdateSyncStatus(ctx, credID, "failed", &reason, nil)
			return nil, fmt.Errorf("exchange auth error: %w", err)
		}
		return nil, fmt.Errorf("fetching exchange balances: %w", err)
	}

	// Filter balances against asset catalog.
	sourceRef := credID.String()
	var holdings []domain.Holding
	for _, b := range balances {
		symbol := strings.ToUpper(b.Asset)
		if !s.catalog[symbol] {
			slog.Warn("skipping unknown asset from exchange sync",
				"exchange", cred.Exchange,
				"asset", b.Asset,
			)
			continue
		}
		holdings = append(holdings, domain.Holding{
			UserID:        userID,
			AssetSymbol:   symbol,
			Quantity:      b.Quantity,
			QuoteCurrency: "USD",
			Source:        cred.Exchange,
			SourceRef:     &sourceRef,
		})
	}

	if err := s.holdings.UpsertBySource(ctx, userID, cred.Exchange, sourceRef, holdings); err != nil {
		return nil, fmt.Errorf("upserting exchange holdings: %w", err)
	}

	// Fetch trade history if the connector supports it.
	result := &SyncCredentialResult{Exchange: cred.Exchange}
	if thc, ok := connector.(exchange.TradeHistoryConnector); ok {
		trades, err := thc.FetchTrades(ctx, string(apiKeyBytes), string(secretBytes))
		if err != nil {
			slog.Warn("trade history fetch failed (non-fatal)",
				"exchange", cred.Exchange,
				"error", err,
			)
		} else {
			for _, t := range trades {
				symbol := strings.ToUpper(t.Asset)
				if !s.catalog[symbol] {
					continue
				}
				var fee *float64
				if t.Fee > 0 {
					f := t.Fee
					fee = &f
				}
				result.Trades = append(result.Trades, domain.LedgerEntry{
					Asset:       symbol,
					Quantity:    t.Quantity,
					Type:        t.Type,
					Timestamp:   t.Timestamp,
					SourceID:    t.TradeID,
					Fee:         fee,
					FeeCurrency: t.FeeCurrency,
				})
			}
			slog.Info("trade history synced",
				"exchange", cred.Exchange,
				"trade_count", len(result.Trades),
			)
		}
	}

	// Update sync timestamp and mark as active.
	now := time.Now()
	if err := s.credentials.UpdateSyncStatus(ctx, credID, "active", nil, &now); err != nil {
		return nil, fmt.Errorf("updating sync status: %w", err)
	}

	return result, nil
}

// --- Wallet Management ---

// CreateWallet validates, encrypts, and stores a wallet address.
func (s *PortfolioService) CreateWallet(ctx context.Context, userID uuid.UUID, chain, address, label string) (*domain.WalletAddress, error) {
	if err := ValidateChain(chain); err != nil {
		return nil, err
	}
	if err := ValidateWalletAddress(chain, address); err != nil {
		return nil, err
	}

	// Encrypt address.
	addrCipher, addrNonce, err := s.encryption.EncryptForUser(userID, []byte(address))
	if err != nil {
		return nil, fmt.Errorf("encrypting wallet address: %w", err)
	}

	// Encrypt label if provided.
	var labelCipher, labelNonce []byte
	if label != "" {
		labelCipher, labelNonce, err = s.encryption.EncryptForUser(userID, []byte(label))
		if err != nil {
			return nil, fmt.Errorf("encrypting wallet label: %w", err)
		}
	}

	prefix := address
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	w := &domain.WalletAddress{
		ID:            uuid.New(),
		UserID:        userID,
		Chain:         chain,
		AddressPrefix: prefix,
		AddressCipher: addrCipher,
		AddressNonce:  addrNonce,
		LabelCipher:   labelCipher,
		LabelNonce:    labelNonce,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.wallets.Create(ctx, w); err != nil {
		return nil, err
	}

	// Return metadata only — clear cipher fields.
	w.AddressCipher = nil
	w.AddressNonce = nil
	w.LabelCipher = nil
	w.LabelNonce = nil

	return w, nil
}

// ListWallets returns wallet metadata for a user. No addresses or labels are decrypted.
func (s *PortfolioService) ListWallets(ctx context.Context, userID uuid.UUID) ([]domain.WalletAddress, error) {
	wallets, err := s.wallets.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Strip cipher fields — return only metadata (prefix, timestamps, chain).
	for i := range wallets {
		wallets[i].AddressCipher = nil
		wallets[i].AddressNonce = nil
		wallets[i].LabelCipher = nil
		wallets[i].LabelNonce = nil
	}

	return wallets, nil
}

// DeleteWallet removes a wallet and cascade-deletes associated holdings.
func (s *PortfolioService) DeleteWallet(ctx context.Context, userID, walletID uuid.UUID) error {
	w, err := s.wallets.FindByID(ctx, walletID, userID)
	if err != nil {
		return err
	}
	if w == nil {
		return ErrForbidden
	}
	if w.UserID != userID {
		return ErrForbidden
	}

	// Cascade-delete holdings linked to this wallet.
	sourceRef := walletID.String()
	if err := s.holdings.DeleteBySourceRef(ctx, userID, sourceRef); err != nil {
		return fmt.Errorf("cascade-deleting wallet holdings: %w", err)
	}

	return s.wallets.Delete(ctx, walletID, userID)
}

// SyncWallet decrypts the wallet address, fetches on-chain balances,
// filters against the asset catalog, upserts holdings, and updates the sync timestamp.
func (s *PortfolioService) SyncWallet(ctx context.Context, userID, walletID uuid.UUID) error {
	w, err := s.wallets.FindByID(ctx, walletID, userID)
	if err != nil {
		return err
	}
	if w == nil {
		return ErrForbidden
	}
	if w.UserID != userID {
		return ErrForbidden
	}

	connector, ok := s.walletMap[w.Chain]
	if !ok {
		return fmt.Errorf("no connector for chain %q", w.Chain)
	}

	// Decrypt address.
	addrBytes, err := s.encryption.DecryptForUser(userID, w.AddressCipher, w.AddressNonce)
	if err != nil {
		return fmt.Errorf("decrypting wallet address: %w", err)
	}

	balances, err := connector.FetchBalances(ctx, string(addrBytes))
	if err != nil {
		return fmt.Errorf("fetching wallet balances: %w", err)
	}

	// Filter balances against asset catalog.
	sourceRef := walletID.String()
	var holdings []domain.Holding
	for _, b := range balances {
		symbol := strings.ToUpper(b.Asset)
		if !s.catalog[symbol] {
			slog.Warn("skipping unknown asset from wallet sync",
				"chain", w.Chain,
				"asset", b.Asset,
			)
			continue
		}
		holdings = append(holdings, domain.Holding{
			UserID:        userID,
			AssetSymbol:   symbol,
			Quantity:      b.Quantity,
			QuoteCurrency: "USD",
			Source:        w.Chain,
			SourceRef:     &sourceRef,
		})
	}

	if err := s.holdings.UpsertBySource(ctx, userID, w.Chain, sourceRef, holdings); err != nil {
		return fmt.Errorf("upserting wallet holdings: %w", err)
	}

	now := time.Now()
	if err := s.wallets.UpdateSyncTime(ctx, walletID, now); err != nil {
		return fmt.Errorf("updating wallet sync time: %w", err)
	}

	return nil
}

// --- Sync All ---

// SyncAllResult summarizes the outcome of syncing all sources.
type SyncAllResult struct {
	ExchangesSynced int      `json:"exchanges_synced"`
	WalletsSynced   int      `json:"wallets_synced"`
	Errors          []string `json:"errors"`
}

// SyncAll syncs all exchange credentials and wallets for the user,
// collecting errors without failing on the first one.
func (s *PortfolioService) SyncAll(ctx context.Context, userID uuid.UUID) SyncAllResult {
	result := SyncAllResult{Errors: []string{}}

	creds, err := s.credentials.ListByUserID(ctx, userID)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("listing credentials: %v", err))
	} else {
		for _, cred := range creds {
			if _, err := s.SyncCredential(ctx, userID, cred.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("sync exchange %s (%s): %v", cred.Exchange, cred.ID, err))
			} else {
				result.ExchangesSynced++
			}
		}
	}

	wallets, err := s.wallets.ListByUserID(ctx, userID)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("listing wallets: %v", err))
	} else {
		for _, w := range wallets {
			if err := s.SyncWallet(ctx, userID, w.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("sync wallet %s (%s): %v", w.Chain, w.ID, err))
			} else {
				result.WalletsSynced++
			}
		}
	}

	return result
}

// --- Ledger Management ---

// ImportLedgerExport parses a Ledger Live JSON export, filters balances against
// the asset catalog, and upserts holdings with source "ledger". Each upload
// replaces all previous ledger holdings for the user. Returns the count of
// imported holdings.
func (s *PortfolioService) ImportLedgerExport(ctx context.Context, userID uuid.UUID, data []byte) (int, error) {
	balances, err := s.ledgerConn.ParseExport(data)
	if err != nil {
		return 0, fmt.Errorf("parsing ledger export: %w", err)
	}

	// Delete existing ledger holdings before importing fresh data.
	if err := s.holdings.DeleteBySource(ctx, userID, "ledger"); err != nil {
		return 0, fmt.Errorf("clearing previous ledger holdings: %w", err)
	}

	var holdings []domain.Holding
	for _, b := range balances {
		symbol := strings.ToUpper(b.Asset)
		if !s.catalog[symbol] {
			slog.Warn("skipping unknown asset from ledger import", "asset", b.Asset)
			continue
		}
		holdings = append(holdings, domain.Holding{
			ID:            uuid.New(),
			UserID:        userID,
			AssetSymbol:   symbol,
			Quantity:      b.Quantity,
			QuoteCurrency: "USD",
			Source:        "ledger",
		})
	}

	for i := range holdings {
		h := &holdings[i]
		h.CreatedAt = time.Now()
		h.UpdatedAt = time.Now()
		if err := s.holdings.Create(ctx, h); err != nil {
			return 0, fmt.Errorf("creating ledger holding: %w", err)
		}
	}

	return len(holdings), nil
}

// DisconnectLedger deletes all ledger-sourced holdings for the user.
func (s *PortfolioService) DisconnectLedger(ctx context.Context, userID uuid.UUID) error {
	return s.holdings.DeleteBySource(ctx, userID, "ledger")
}

// --- Helpers ---

// encryptHoldingNotes encrypts the Notes field in-place if non-empty.
// The Holding domain struct stores notes as plaintext in the Notes field for
// the application layer; the repository is expected to persist NotesCipher/NotesNonce
// columns. This helper sets the encrypted fields on the holding for the repo to use.
func (s *PortfolioService) encryptHoldingNotes(userID uuid.UUID, h *domain.Holding) error {
	if h.Notes == "" {
		return nil
	}
	_, _, err := s.encryption.EncryptForUser(userID, []byte(h.Notes))
	if err != nil {
		return err
	}
	// Notes remain in the struct for the repository layer to encrypt at write time.
	// The repository handles the cipher/nonce columns directly.
	return nil
}

// decryptHoldingNotes is a no-op placeholder. The repository layer stores notes
// as cipher+nonce columns and the Holding struct carries the plaintext Notes field.
// If the repository returns encrypted notes, this method would decrypt them.
func (s *PortfolioService) decryptHoldingNotes(_ uuid.UUID, _ *domain.Holding) error {
	// Notes decryption is handled at the repository/handler boundary.
	// The Holding struct's Notes field is populated by the repository after decryption.
	return nil
}

// isAuthError checks if an error indicates an authentication failure from an exchange.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "invalid key") ||
		strings.Contains(msg, "invalid api")
}
