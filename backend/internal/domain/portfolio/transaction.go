package portfolio

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Transaction represents a normalized, user-facing record derived from a raw ledger entry.
type Transaction struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"user_id"`
	AssetSymbol       string    `json:"asset_symbol"`
	Quantity          float64   `json:"quantity"`
	Type              string    `json:"type"` // "buy", "sell", "transfer", "fee"
	Timestamp         time.Time `json:"timestamp"`
	Source            string    `json:"source"`          // "binance", "kraken", "coinbase", "ethereum", "solana", "bitcoin", "ledger"
	SourceRef         string    `json:"source_ref"`      // credential/wallet UUID
	ReferenceValue    *float64  `json:"reference_value"` // value in reference currency, nil if unresolvable
	ReferenceCurrency string    `json:"reference_currency"`
	Resolution        string    `json:"resolution"`   // "exact", "hourly", "daily", "unresolvable"
	Fee               *float64  `json:"fee"`          // trade fee amount, nil when unknown
	FeeCurrency       string    `json:"fee_currency"` // currency the fee is denominated in
	ManuallyEdited    bool      `json:"manually_edited"`
	Notes             string    `json:"notes,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PnLSnapshot is the computed FIFO profit & loss snapshot for a user.
type PnLSnapshot struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"user_id"`
	ReferenceCurrency string     `json:"reference_currency"`
	TotalRealized     float64    `json:"total_realized"`
	TotalUnrealized   float64    `json:"total_unrealized"`
	TotalPnL          float64    `json:"total_pnl"`
	HasApproximate    bool       `json:"has_approximate"`
	ExcludedCount     int        `json:"excluded_count"`
	Assets            []AssetPnL `json:"assets"`
	ComputedAt        time.Time  `json:"computed_at"`
}

// AssetPnL is the per-asset P&L breakdown within a snapshot.
type AssetPnL struct {
	AssetSymbol      string  `json:"asset_symbol"`
	RealizedPnL      float64 `json:"realized_pnl"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	TotalPnL         float64 `json:"total_pnl"`
	TransactionCount int     `json:"transaction_count"`
}

// LedgerEntry is the raw input type for normalization, produced by exchange
// connectors, wallet connectors, and the ledger parser.
type LedgerEntry struct {
	Asset       string
	Quantity    float64
	Type        string // "buy", "sell", "transfer", "fee"
	Timestamp   time.Time
	SourceID    string   // exchange-specific trade ID or tx hash
	Fee         *float64 // trade fee amount, nil when unknown
	FeeCurrency string   // currency the fee is denominated in
}

// ListOptions holds pagination and filtering parameters for transaction queries.
type ListOptions struct {
	Asset     string
	Type      string
	StartDate *time.Time
	EndDate   *time.Time
	Page      int
	PageSize  int
}

// --- Repository interfaces ---

// TransactionRepository defines persistence operations for portfolio transactions.
type TransactionRepository interface {
	Create(ctx context.Context, tx *Transaction) error
	Upsert(ctx context.Context, tx *Transaction) error // skip if manually_edited
	Update(ctx context.Context, tx *Transaction) error
	FindByID(ctx context.Context, userID, txID uuid.UUID) (*Transaction, error)
	ListByUser(ctx context.Context, userID uuid.UUID, opts ListOptions) ([]Transaction, int, error)
	DeleteBySourceRef(ctx context.Context, userID uuid.UUID, source, sourceRef string) error
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
	// DistinctCurrencies returns the unique reference currencies across a user's transactions.
	DistinctCurrencies(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// PnLRepository defines persistence operations for P&L snapshots.
type PnLRepository interface {
	Upsert(ctx context.Context, snapshot *PnLSnapshot) error
	FindByUser(ctx context.Context, userID uuid.UUID, referenceCurrency string) (*PnLSnapshot, error)
}
