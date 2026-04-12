package portfolio

import (
	"errors"
	"math"
	"testing"

	"pgregory.net/rapid"
)

// --- Generators ---

const (
	hexChars    = "0123456789abcdef"
	base58Chars = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	bech32Chars = "ac-hj-np-z02-9" // for reference; we use the expanded set below
)

// validBech32Char draws a single valid bech32 character (lowercase alphanumeric minus 1, b, i, o).
var bech32ValidChars = []byte("acdefghjklmnpqrstuvwxyz023456789")

func genHexString(t *rapid.T, name string, length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = hexChars[rapid.IntRange(0, len(hexChars)-1).Draw(t, name)]
	}
	return string(b)
}

func genBase58String(t *rapid.T, name string, length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = base58Chars[rapid.IntRange(0, len(base58Chars)-1).Draw(t, name)]
	}
	return string(b)
}

func genBech32Body(t *rapid.T, name string, length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = bech32ValidChars[rapid.IntRange(0, len(bech32ValidChars)-1).Draw(t, name)]
	}
	return string(b)
}

// --- Property 18: Wallet address format validation ---

// Feature: portfolio-tracker, Property 18: Wallet address format validation
//
// For any valid Ethereum address (0x + 40 hex chars), valid Solana address
// (32-44 base58 chars), or valid Bitcoin address (bech32/base58check),
// the validator accepts the address. For any string not matching the expected
// format, the validator rejects it.
func TestPropertyWalletAddressFormatValidation(t *testing.T) {
	t.Run("valid_ethereum_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			hex40 := genHexString(t, "hexChar", 40)
			addr := "0x" + hex40
			if err := ValidateWalletAddress("ethereum", addr); err != nil {
				t.Fatalf("valid Ethereum address %q rejected: %v", addr, err)
			}
		})
	})

	t.Run("invalid_ethereum_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kind := rapid.IntRange(0, 3).Draw(t, "invalidKind")
			var addr string
			switch kind {
			case 0:
				// Wrong length: too short (1-39 hex chars)
				n := rapid.IntRange(1, 39).Draw(t, "shortLen")
				addr = "0x" + genHexString(t, "hexChar", n)
			case 1:
				// Wrong length: too long (41-80 hex chars)
				n := rapid.IntRange(41, 80).Draw(t, "longLen")
				addr = "0x" + genHexString(t, "hexChar", n)
			case 2:
				// Missing 0x prefix
				addr = genHexString(t, "hexChar", 40)
			case 3:
				// Invalid character in hex portion
				hex40 := genHexString(t, "hexChar", 40)
				pos := rapid.IntRange(0, 39).Draw(t, "pos")
				invalidChar := "gGzZ!@"
				c := invalidChar[rapid.IntRange(0, len(invalidChar)-1).Draw(t, "badChar")]
				addr = "0x" + hex40[:pos] + string(c) + hex40[pos+1:]
			}
			if err := ValidateWalletAddress("ethereum", addr); err == nil {
				t.Fatalf("invalid Ethereum address %q was accepted", addr)
			}
		})
	})

	t.Run("valid_solana_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			length := rapid.IntRange(32, 44).Draw(t, "addrLen")
			addr := genBase58String(t, "b58Char", length)
			if err := ValidateWalletAddress("solana", addr); err != nil {
				t.Fatalf("valid Solana address %q (len=%d) rejected: %v", addr, len(addr), err)
			}
		})
	})

	t.Run("invalid_solana_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kind := rapid.IntRange(0, 2).Draw(t, "invalidKind")
			var addr string
			switch kind {
			case 0:
				// Too short (1-31 chars)
				n := rapid.IntRange(1, 31).Draw(t, "shortLen")
				addr = genBase58String(t, "b58Char", n)
			case 1:
				// Too long (45-80 chars)
				n := rapid.IntRange(45, 80).Draw(t, "longLen")
				addr = genBase58String(t, "b58Char", n)
			case 2:
				// Invalid characters (0, O, I, l are not in base58)
				length := rapid.IntRange(32, 44).Draw(t, "addrLen")
				addr = genBase58String(t, "b58Char", length)
				pos := rapid.IntRange(0, length-1).Draw(t, "pos")
				invalidChars := "0OIl"
				c := invalidChars[rapid.IntRange(0, len(invalidChars)-1).Draw(t, "badChar")]
				addr = addr[:pos] + string(c) + addr[pos+1:]
			}
			if err := ValidateWalletAddress("solana", addr); err == nil {
				t.Fatalf("invalid Solana address %q was accepted", addr)
			}
		})
	})

	t.Run("valid_bitcoin_bech32_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// bech32 body length: 6-87 chars after "bc1"
			bodyLen := rapid.IntRange(6, 87).Draw(t, "bodyLen")
			body := genBech32Body(t, "bech32Char", bodyLen)
			addr := "bc1" + body
			if err := ValidateWalletAddress("bitcoin", addr); err != nil {
				t.Fatalf("valid Bitcoin bech32 address %q rejected: %v", addr, err)
			}
		})
	})

	t.Run("valid_bitcoin_base58check_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// base58check: prefix 1 or 3, total length 25-34
			prefix := rapid.SampledFrom([]string{"1", "3"}).Draw(t, "prefix")
			totalLen := rapid.IntRange(25, 34).Draw(t, "totalLen")
			body := genBase58String(t, "b58Char", totalLen-1)
			addr := prefix + body
			if err := ValidateWalletAddress("bitcoin", addr); err != nil {
				t.Fatalf("valid Bitcoin base58check address %q rejected: %v", addr, err)
			}
		})
	})

	t.Run("invalid_bitcoin_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kind := rapid.IntRange(0, 3).Draw(t, "invalidKind")
			var addr string
			switch kind {
			case 0:
				// Wrong prefix (not bc1, 1, or 3)
				badPrefixes := []string{"2", "4", "5", "bc2", "ab1", "x"}
				addr = rapid.SampledFrom(badPrefixes).Draw(t, "badPrefix") + genBase58String(t, "b58Char", 30)
			case 1:
				// bech32 too short (bc1 + fewer than 6 chars)
				n := rapid.IntRange(1, 5).Draw(t, "shortLen")
				addr = "bc1" + genBech32Body(t, "bech32Char", n)
			case 2:
				// base58check too short (prefix 1/3 + total < 25)
				prefix := rapid.SampledFrom([]string{"1", "3"}).Draw(t, "prefix")
				totalLen := rapid.IntRange(2, 24).Draw(t, "shortTotal")
				addr = prefix + genBase58String(t, "b58Char", totalLen-1)
			case 3:
				// base58check too long (prefix 1/3 + total > 34)
				prefix := rapid.SampledFrom([]string{"1", "3"}).Draw(t, "prefix")
				totalLen := rapid.IntRange(35, 60).Draw(t, "longTotal")
				addr = prefix + genBase58String(t, "b58Char", totalLen-1)
			}
			if err := ValidateWalletAddress("bitcoin", addr); err == nil {
				t.Fatalf("invalid Bitcoin address %q was accepted", addr)
			}
		})
	})

	t.Run("unsupported_chain_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			chain := rapid.StringMatching(`[a-z]{3,20}`).Draw(t, "chain")
			if AllowedChains[chain] {
				t.Skip("generated a valid chain, skipping")
			}
			addr := "0x" + genHexString(t, "hexChar", 40)
			err := ValidateWalletAddress(chain, addr)
			if err == nil {
				t.Fatalf("unsupported chain %q was accepted", chain)
			}
			if !errors.Is(err, ErrUnsupportedChain) {
				t.Fatalf("expected ErrUnsupportedChain, got: %v", err)
			}
		})
	})
}

// --- Property 7: Non-positive quantity rejection ---

// Feature: portfolio-tracker, Property 7: Non-positive quantity rejection
//
// For any float64 value <= 0 (including zero, negative, and negative infinity),
// ValidateQuantity rejects it. For any positive finite float64, it accepts.
func TestPropertyNonPositiveQuantityRejection(t *testing.T) {
	t.Run("non_positive_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kind := rapid.IntRange(0, 3).Draw(t, "kind")
			var qty float64
			switch kind {
			case 0:
				// Zero
				qty = 0
			case 1:
				// Negative value
				qty = -rapid.Float64Range(math.SmallestNonzeroFloat64, math.MaxFloat64).Draw(t, "posVal")
			case 2:
				// Negative infinity
				qty = math.Inf(-1)
			case 3:
				// NaN (also non-positive in spirit)
				qty = math.NaN()
			}
			err := ValidateQuantity(qty)
			if err == nil {
				t.Fatalf("non-positive quantity %v was accepted", qty)
			}
			if !errors.Is(err, ErrNonPositiveQuantity) {
				t.Fatalf("expected ErrNonPositiveQuantity, got: %v", err)
			}
		})
	})

	t.Run("positive_infinity_rejected", func(t *testing.T) {
		err := ValidateQuantity(math.Inf(1))
		if err == nil {
			t.Fatalf("positive infinity was accepted")
		}
	})

	t.Run("positive_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			qty := rapid.Float64Range(math.SmallestNonzeroFloat64, 1e18).Draw(t, "qty")
			if err := ValidateQuantity(qty); err != nil {
				t.Fatalf("positive quantity %v rejected: %v", qty, err)
			}
		})
	})
}

// --- Property 8: Invalid enum value rejection ---

// Feature: portfolio-tracker, Property 8: Invalid enum value rejection
//
// For any string not in the valid exchange set, ValidateExchange rejects it.
// For any string not in the valid chain set, ValidateChain rejects it.
// For any string not in the asset catalog, ValidateAssetSymbol rejects it.
// Valid values are accepted.
func TestPropertyInvalidEnumValueRejection(t *testing.T) {
	t.Run("invalid_exchange_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			exchange := rapid.StringMatching(`[a-zA-Z0-9_]{1,30}`).Draw(t, "exchange")
			if AllowedExchanges[exchange] {
				t.Skip("generated a valid exchange, skipping")
			}
			err := ValidateExchange(exchange)
			if err == nil {
				t.Fatalf("invalid exchange %q was accepted", exchange)
			}
			if !errors.Is(err, ErrUnsupportedExchange) {
				t.Fatalf("expected ErrUnsupportedExchange, got: %v", err)
			}
		})
	})

	t.Run("valid_exchange_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			exchange := rapid.SampledFrom([]string{"binance", "kraken", "coinbase"}).Draw(t, "exchange")
			if err := ValidateExchange(exchange); err != nil {
				t.Fatalf("valid exchange %q rejected: %v", exchange, err)
			}
		})
	})

	t.Run("invalid_chain_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			chain := rapid.StringMatching(`[a-zA-Z0-9_]{1,30}`).Draw(t, "chain")
			if AllowedChains[chain] {
				t.Skip("generated a valid chain, skipping")
			}
			err := ValidateChain(chain)
			if err == nil {
				t.Fatalf("invalid chain %q was accepted", chain)
			}
			if !errors.Is(err, ErrUnsupportedChain) {
				t.Fatalf("expected ErrUnsupportedChain, got: %v", err)
			}
		})
	})

	t.Run("valid_chain_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			chain := rapid.SampledFrom([]string{"ethereum", "solana", "bitcoin"}).Draw(t, "chain")
			if err := ValidateChain(chain); err != nil {
				t.Fatalf("valid chain %q rejected: %v", chain, err)
			}
		})
	})

	t.Run("invalid_asset_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			catalog := map[string]bool{"BTC": true, "ETH": true, "SOL": true}
			symbol := rapid.StringMatching(`[A-Z0-9]{1,10}`).Draw(t, "symbol")
			if catalog[symbol] {
				t.Skip("generated a valid asset, skipping")
			}
			err := ValidateAssetSymbol(symbol, catalog)
			if err == nil {
				t.Fatalf("invalid asset %q was accepted", symbol)
			}
			if !errors.Is(err, ErrInvalidAssetSymbol) {
				t.Fatalf("expected ErrInvalidAssetSymbol, got: %v", err)
			}
		})
	})

	t.Run("valid_asset_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			catalog := map[string]bool{"BTC": true, "ETH": true, "SOL": true, "USDT": true}
			symbol := rapid.SampledFrom([]string{"BTC", "ETH", "SOL", "USDT"}).Draw(t, "symbol")
			if err := ValidateAssetSymbol(symbol, catalog); err != nil {
				t.Fatalf("valid asset %q rejected: %v", symbol, err)
			}
		})
	})

	t.Run("empty_string_rejected", func(t *testing.T) {
		if err := ValidateExchange(""); err == nil {
			t.Fatal("empty exchange was accepted")
		}
		if err := ValidateChain(""); err == nil {
			t.Fatal("empty chain was accepted")
		}
		if err := ValidateAssetSymbol("", map[string]bool{"BTC": true}); err == nil {
			t.Fatal("empty asset symbol was accepted")
		}
	})
}
