package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// LedgerConnector is the interface for parsing Ledger Live export data.
type LedgerConnector interface {
	ParseExport(data []byte) ([]Balance, error)
}

// Balance represents a single asset balance from a Ledger account.
type Balance struct {
	Asset    string
	Quantity float64
}

// JSONParser implements LedgerConnector by parsing Ledger Live desktop export JSON.
type JSONParser struct{}

// NewJSONParser creates a new Ledger Live JSON export parser.
func NewJSONParser() *JSONParser {
	return &JSONParser{}
}

// ledgerExport is the top-level structure of a Ledger Live export file.
type ledgerExport struct {
	Accounts []ledgerAccount `json:"accounts"`
}

// ledgerAccount represents a single account in the Ledger Live export.
type ledgerAccount struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Currency string     `json:"currency"`
	Balance  string     `json:"balance"`
	Unit     ledgerUnit `json:"unit"`
}

// ledgerUnit describes the denomination of an account's balance.
type ledgerUnit struct {
	Name      string `json:"name"`
	Code      string `json:"code"`
	Magnitude int    `json:"magnitude"`
}

// ParseExport parses a Ledger Live desktop export JSON and returns balances.
// It uses the unit code (uppercased) as the asset symbol and applies magnitude
// division (balance / 10^magnitude). Zero and negative balances are skipped.
func (p *JSONParser) ParseExport(data []byte) ([]Balance, error) {
	var export ledgerExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("invalid ledger export JSON: %w", err)
	}

	var balances []Balance
	for _, acct := range export.Accounts {
		code := strings.TrimSpace(acct.Unit.Code)
		if code == "" {
			continue
		}
		symbol := strings.ToUpper(code)

		// Parse the string balance as a float.
		var rawBalance float64
		if _, err := fmt.Sscanf(acct.Balance, "%f", &rawBalance); err != nil {
			continue
		}

		qty := rawBalance
		if acct.Unit.Magnitude > 0 {
			qty = rawBalance / math.Pow(10, float64(acct.Unit.Magnitude))
		}

		if qty <= 0 {
			continue
		}

		balances = append(balances, Balance{
			Asset:    symbol,
			Quantity: qty,
		})
	}

	return balances, nil
}
