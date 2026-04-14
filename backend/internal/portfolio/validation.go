package portfolio

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Validation errors returned by portfolio input validators.
var (
	ErrInvalidEthereumAddress = errors.New("invalid Ethereum address: must be 0x followed by 40 hex characters")
	ErrInvalidSolanaAddress   = errors.New("invalid Solana address: must be 32-44 base58 characters")
	ErrInvalidBitcoinAddress  = errors.New("invalid Bitcoin address: must be a valid bech32 or base58check address")
	ErrUnsupportedChain       = errors.New("unsupported chain: must be one of ethereum, solana, bitcoin")
	ErrUnsupportedExchange    = errors.New("unsupported exchange: must be one of binance, kraken, coinbase")
	ErrInvalidAssetSymbol     = errors.New("asset symbol not found in catalog")
	ErrNonPositiveQuantity    = errors.New("holding quantity must be greater than zero")
	ErrDuplicateCredential    = errors.New("exchange credential already exists for this user")
)

// Allowed exchanges and chains.
var (
	AllowedExchanges = map[string]bool{
		"binance":  true,
		"kraken":   true,
		"coinbase": true,
	}
	AllowedChains = map[string]bool{
		"ethereum": true,
		"solana":   true,
		"bitcoin":  true,
	}
)

// Regex patterns for address validation.
var (
	ethAddressRegex     = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	solanaBase58Charset = regexp.MustCompile(`^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$`)
	bech32AddressRegex  = regexp.MustCompile(`^bc1[ac-hj-np-z02-9]{6,87}$`)
	base58CheckCharset  = regexp.MustCompile(`^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$`)
)

// ValidateWalletAddress validates a wallet address for the given chain.
func ValidateWalletAddress(chain, address string) error {
	switch chain {
	case "ethereum":
		return validateEthereumAddress(address)
	case "solana":
		return validateSolanaAddress(address)
	case "bitcoin":
		return validateBitcoinAddress(address)
	default:
		return fmt.Errorf("%w: got %q", ErrUnsupportedChain, chain)
	}
}

// validateEthereumAddress checks for 0x prefix followed by exactly 40 hex characters.
func validateEthereumAddress(address string) error {
	if !ethAddressRegex.MatchString(address) {
		return ErrInvalidEthereumAddress
	}
	return nil
}

// validateSolanaAddress checks for 32-44 base58 characters.
func validateSolanaAddress(address string) error {
	if len(address) < 32 || len(address) > 44 {
		return ErrInvalidSolanaAddress
	}
	if !solanaBase58Charset.MatchString(address) {
		return ErrInvalidSolanaAddress
	}
	return nil
}

// validateBitcoinAddress accepts bech32 (bc1 prefix) or base58check (1 or 3 prefix) formats.
func validateBitcoinAddress(address string) error {
	if strings.HasPrefix(address, "bc1") {
		return validateBitcoinBech32(address)
	}
	if len(address) > 0 && (address[0] == '1' || address[0] == '3') {
		return validateBitcoinBase58Check(address)
	}
	return ErrInvalidBitcoinAddress
}

// validateBitcoinBech32 checks bc1 prefix with valid bech32 characters and reasonable length.
func validateBitcoinBech32(address string) error {
	if !bech32AddressRegex.MatchString(address) {
		return ErrInvalidBitcoinAddress
	}
	return nil
}

// validateBitcoinBase58Check checks 1/3 prefix with valid base58 characters and length 25-34.
func validateBitcoinBase58Check(address string) error {
	if len(address) < 25 || len(address) > 34 {
		return ErrInvalidBitcoinAddress
	}
	if !base58CheckCharset.MatchString(address) {
		return ErrInvalidBitcoinAddress
	}
	return nil
}

// ValidateExchange checks that the exchange name is in the allowed set.
func ValidateExchange(exchange string) error {
	if !AllowedExchanges[exchange] {
		return fmt.Errorf("%w: got %q", ErrUnsupportedExchange, exchange)
	}
	return nil
}

// ValidateChain checks that the chain name is in the allowed set.
func ValidateChain(chain string) error {
	if !AllowedChains[chain] {
		return fmt.Errorf("%w: got %q", ErrUnsupportedChain, chain)
	}
	return nil
}

// ValidateAssetSymbol checks that the asset symbol exists in the provided catalog.
func ValidateAssetSymbol(symbol string, catalog map[string]bool) error {
	if !catalog[symbol] {
		return fmt.Errorf("%w: %q", ErrInvalidAssetSymbol, symbol)
	}
	return nil
}

// ValidateQuantity checks that the holding quantity is positive (> 0) and finite.
func ValidateQuantity(quantity float64) error {
	if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return ErrNonPositiveQuantity
	}
	return nil
}
