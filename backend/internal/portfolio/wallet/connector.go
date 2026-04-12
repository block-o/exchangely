package wallet

import "context"

// WalletConnector is the interface each blockchain wallet adapter implements.
type WalletConnector interface {
	FetchBalances(ctx context.Context, address string) ([]Balance, error)
	Chain() string
}

// Balance represents a single asset balance from an on-chain wallet.
type Balance struct {
	Asset    string
	Quantity float64
}
