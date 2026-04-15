package exchange

import (
	"context"
	"time"
)

// Connector is the interface each exchange adapter implements.
type Connector interface {
	FetchBalances(ctx context.Context, apiKey, apiSecret string) ([]Balance, error)
	Name() string
}

// TradeHistoryConnector is an optional interface for connectors that support
// fetching trade history. Not all exchanges expose this via their API.
type TradeHistoryConnector interface {
	FetchTrades(ctx context.Context, apiKey, apiSecret string) ([]Trade, error)
}

// Balance represents a single asset balance returned by an exchange.
type Balance struct {
	Asset    string
	Quantity float64
}

// Trade represents a single executed trade returned by an exchange.
type Trade struct {
	Asset       string
	Quantity    float64
	Type        string // "buy" or "sell"
	Price       float64
	Cost        float64
	Fee         float64
	FeeCurrency string
	Timestamp   time.Time
	TradeID     string
}
