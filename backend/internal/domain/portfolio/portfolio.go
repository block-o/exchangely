package portfolio

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Holding represents a single position in a user's portfolio.
type Holding struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	AssetSymbol   string    `json:"asset_symbol"`
	Quantity      float64   `json:"quantity"`
	AvgBuyPrice   *float64  `json:"avg_buy_price,omitempty"`
	QuoteCurrency string    `json:"quote_currency"`
	Source        string    `json:"source"`               // "manual", "binance", "kraken", "coinbase", "ethereum", "solana", "bitcoin", "ledger"
	SourceRef     *string   `json:"source_ref,omitempty"` // credential ID or wallet ID
	Notes         string    `json:"notes,omitempty"`      // encrypted at rest
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ExchangeCredential stores encrypted API keys for a centralized exchange.
type ExchangeCredential struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Exchange     string     `json:"exchange"` // "binance", "kraken", "coinbase"
	APIKeyPrefix string     `json:"api_key_prefix"`
	APIKeyCipher []byte     `json:"-"`
	SecretCipher []byte     `json:"-"`
	Nonce        []byte     `json:"-"`      // GCM nonce for secret
	KeyNonce     []byte     `json:"-"`      // GCM nonce for API key
	Status       string     `json:"status"` // "active", "failed"
	ErrorReason  *string    `json:"error_reason,omitempty"`
	LastSyncAt   *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// WalletAddress stores an encrypted blockchain wallet address.
type WalletAddress struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Chain         string     `json:"chain"` // "ethereum", "solana", "bitcoin"
	AddressPrefix string     `json:"address_prefix"`
	AddressCipher []byte     `json:"-"`
	AddressNonce  []byte     `json:"-"`
	Label         string     `json:"label,omitempty"` // encrypted at rest
	LabelCipher   []byte     `json:"-"`
	LabelNonce    []byte     `json:"-"`
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// LedgerCredential stores encrypted Ledger Live API credentials.
type LedgerCredential struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	TokenCipher []byte     `json:"-"`
	TokenNonce  []byte     `json:"-"`
	LastSyncAt  *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Valuation is the computed portfolio snapshot.
type Valuation struct {
	TotalValue    float64          `json:"total_value"`
	QuoteCurrency string           `json:"quote_currency"`
	Assets        []AssetValuation `json:"assets"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// AssetValuation is the per-asset breakdown within a portfolio valuation.
type AssetValuation struct {
	AssetSymbol   string   `json:"asset_symbol"`
	Quantity      float64  `json:"quantity"`
	CurrentPrice  float64  `json:"current_price"`
	CurrentValue  float64  `json:"current_value"`
	AllocationPct float64  `json:"allocation_pct"`
	AvgBuyPrice   *float64 `json:"avg_buy_price,omitempty"`
	UnrealizedPnL *float64 `json:"unrealized_pnl,omitempty"`
	UnrealizedPct *float64 `json:"unrealized_pnl_pct,omitempty"`
	Priced        bool     `json:"priced"`
	Source        string   `json:"source"`
}

// HistoricalPoint is a single data point in the historical portfolio value series.
type HistoricalPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// --- Repository interfaces ---

// HoldingRepository defines persistence operations for portfolio holdings.
type HoldingRepository interface {
	Create(ctx context.Context, h *Holding) error
	Update(ctx context.Context, h *Holding) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	FindByID(ctx context.Context, id, userID uuid.UUID) (*Holding, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]Holding, error)
	UpsertBySource(ctx context.Context, userID uuid.UUID, source, sourceRef string, holdings []Holding) error
	DeleteBySourceRef(ctx context.Context, userID uuid.UUID, sourceRef string) error
	DeleteBySource(ctx context.Context, userID uuid.UUID, source string) error
}

// CredentialRepository defines persistence operations for exchange credentials.
type CredentialRepository interface {
	Create(ctx context.Context, c *ExchangeCredential) error
	FindByID(ctx context.Context, id, userID uuid.UUID) (*ExchangeCredential, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]ExchangeCredential, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	UpdateSyncStatus(ctx context.Context, id uuid.UUID, status string, errorReason *string, syncTime *time.Time) error
}

// WalletRepository defines persistence operations for wallet addresses.
type WalletRepository interface {
	Create(ctx context.Context, w *WalletAddress) error
	FindByID(ctx context.Context, id, userID uuid.UUID) (*WalletAddress, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]WalletAddress, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	UpdateSyncTime(ctx context.Context, id uuid.UUID, syncTime time.Time) error
}

// LedgerCredentialRepository defines persistence operations for Ledger Live credentials.
type LedgerCredentialRepository interface {
	Create(ctx context.Context, c *LedgerCredential) error
	FindByUserID(ctx context.Context, userID uuid.UUID) (*LedgerCredential, error)
	Delete(ctx context.Context, userID uuid.UUID) error
	UpdateSyncTime(ctx context.Context, id uuid.UUID, syncTime time.Time) error
}
