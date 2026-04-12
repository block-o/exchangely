package ledger

import (
	"math"
	"testing"
)

// --- Interface compliance ---

func TestJSONParserInterfaceCompliance(t *testing.T) {
	var _ LedgerConnector = (*JSONParser)(nil)
}

// --- ParseExport tests ---

func TestParseExportBalanceParsing(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"id": "js:2:bitcoin:xpub...:native_segwit",
				"name": "Bitcoin 1",
				"currency": "bitcoin",
				"balance": "150000000",
				"unit": {"name": "BTC", "code": "BTC", "magnitude": 8}
			},
			{
				"id": "js:2:ethereum:0x...",
				"name": "Ethereum 1",
				"currency": "ethereum",
				"balance": "2500000000000000000",
				"unit": {"name": "ETH", "code": "ETH", "magnitude": 18}
			},
			{
				"id": "js:2:solana:...",
				"name": "Solana 1",
				"currency": "solana",
				"balance": "5000000000",
				"unit": {"name": "SOL", "code": "SOL", "magnitude": 9}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 3 {
		t.Fatalf("expected 3 balances, got %d: %+v", len(balances), balances)
	}

	byAsset := make(map[string]float64)
	for _, b := range balances {
		byAsset[b.Asset] = b.Quantity
	}

	// BTC: 150000000 / 10^8 = 1.5
	if math.Abs(byAsset["BTC"]-1.5) > 1e-10 {
		t.Errorf("expected BTC=1.5, got %f", byAsset["BTC"])
	}
	// ETH: 2500000000000000000 / 10^18 = 2.5
	if math.Abs(byAsset["ETH"]-2.5) > 1e-4 {
		t.Errorf("expected ETH=2.5, got %f", byAsset["ETH"])
	}
	// SOL: 5000000000 / 10^9 = 5.0
	if math.Abs(byAsset["SOL"]-5.0) > 1e-10 {
		t.Errorf("expected SOL=5.0, got %f", byAsset["SOL"])
	}
}

func TestParseExportZeroBalanceFiltering(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"currency": "bitcoin",
				"balance": "100000000",
				"unit": {"code": "BTC", "magnitude": 8}
			},
			{
				"currency": "ethereum",
				"balance": "0",
				"unit": {"code": "ETH", "magnitude": 18}
			},
			{
				"currency": "solana",
				"balance": "0",
				"unit": {"code": "SOL", "magnitude": 9}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 non-zero balance, got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", balances[0].Asset)
	}
}

func TestParseExportMagnitudeConversion(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"currency": "bitcoin",
				"balance": "50000000",
				"unit": {"code": "BTC", "magnitude": 8}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d", len(balances))
	}
	// 50000000 / 10^8 = 0.5 BTC
	if math.Abs(balances[0].Quantity-0.5) > 1e-10 {
		t.Errorf("expected 0.5 BTC, got %f", balances[0].Quantity)
	}
}

func TestParseExportZeroMagnitude(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"currency": "tether",
				"balance": "1000",
				"unit": {"code": "USDT", "magnitude": 0}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d", len(balances))
	}
	if math.Abs(balances[0].Quantity-1000.0) > 1e-10 {
		t.Errorf("expected 1000.0, got %f", balances[0].Quantity)
	}
}

func TestParseExportEmptyAccounts(t *testing.T) {
	data := []byte(`{"accounts":[]}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances, got %d", len(balances))
	}
}

func TestParseExportSkipsEmptyUnitCode(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"currency": "unknown",
				"balance": "100000000",
				"unit": {"code": "", "magnitude": 8}
			},
			{
				"currency": "bitcoin",
				"balance": "50000000",
				"unit": {"code": "BTC", "magnitude": 8}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance (empty code skipped), got %d: %+v", len(balances), balances)
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", balances[0].Asset)
	}
}

func TestParseExportInvalidJSON(t *testing.T) {
	data := []byte(`not valid json`)

	p := NewJSONParser()
	_, err := p.ParseExport(data)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseExportLowercaseCodeUppercased(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"currency": "bitcoin",
				"balance": "100000000",
				"unit": {"code": "btc", "magnitude": 8}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d", len(balances))
	}
	if balances[0].Asset != "BTC" {
		t.Errorf("expected BTC (uppercased), got %s", balances[0].Asset)
	}
}

func TestParseExportNegativeBalanceSkipped(t *testing.T) {
	data := []byte(`{
		"accounts": [
			{
				"currency": "bitcoin",
				"balance": "-100000000",
				"unit": {"code": "BTC", "magnitude": 8}
			}
		]
	}`)

	p := NewJSONParser()
	balances, err := p.ParseExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances (negative skipped), got %d", len(balances))
	}
}
