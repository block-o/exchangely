package exchange

import "context"

// Connector is the interface each exchange adapter implements.
type Connector interface {
	FetchBalances(ctx context.Context, apiKey, apiSecret string) ([]Balance, error)
	Name() string
}

// Balance represents a single asset balance returned by an exchange.
type Balance struct {
	Asset    string
	Quantity float64
}
