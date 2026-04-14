package exchange

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CoinbaseConnector fetches account balances from the Coinbase Advanced Trade API
// using JWT (ES256) authentication with CDP API keys.
type CoinbaseConnector struct {
	baseURL    string
	httpClient *http.Client
}

// NewCoinbaseConnector creates a connector for the Coinbase exchange.
func NewCoinbaseConnector(baseURL string, httpClient *http.Client) *CoinbaseConnector {
	if baseURL == "" {
		baseURL = "https://api.coinbase.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &CoinbaseConnector{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *CoinbaseConnector) Name() string { return "coinbase" }

func (c *CoinbaseConnector) FetchBalances(ctx context.Context, apiKey, apiSecret string) ([]Balance, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("coinbase connector retrying", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balances, retryable, err := c.fetchAllBalances(ctx, apiKey, apiSecret)
		if err == nil {
			return balances, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("coinbase: max retries exceeded: %w", lastErr)
}

func (c *CoinbaseConnector) fetchAllBalances(ctx context.Context, apiKey, apiSecret string) ([]Balance, bool, error) {
	var allBalances []Balance
	cursor := ""

	for {
		page, nextCursor, retryable, err := c.fetchPage(ctx, apiKey, apiSecret, cursor)
		if err != nil {
			return nil, retryable, err
		}
		allBalances = append(allBalances, page...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return allBalances, false, nil
}

func (c *CoinbaseConnector) fetchPage(ctx context.Context, apiKey, apiSecret, cursor string) ([]Balance, string, bool, error) {
	path := "/api/v3/brokerage/accounts"
	if cursor != "" {
		path += "?cursor=" + cursor
	}

	token, err := c.buildJWT(apiKey, apiSecret, "GET", path)
	if err != nil {
		return nil, "", false, fmt.Errorf("coinbase: jwt build failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", true, fmt.Errorf("coinbase: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("coinbase connector server error", "status", resp.StatusCode)
		return nil, "", true, fmt.Errorf("coinbase: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, "", false, fmt.Errorf("coinbase: client error %d", resp.StatusCode)
	}

	var payload struct {
		Accounts []struct {
			Currency string `json:"currency"`
			Balance  struct {
				Value string `json:"value"`
			} `json:"available_balance"`
		} `json:"accounts"`
		Cursor string `json:"cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", false, fmt.Errorf("coinbase: decode error: %w", err)
	}

	var balances []Balance
	for _, acct := range payload.Accounts {
		qty, err := strconv.ParseFloat(acct.Balance.Value, 64)
		if err != nil {
			continue
		}
		if qty <= 0 {
			continue
		}
		balances = append(balances, Balance{
			Asset:    strings.ToUpper(acct.Currency),
			Quantity: qty,
		})
	}
	return balances, payload.Cursor, false, nil
}

// buildJWT creates an ES256-signed JWT for Coinbase CDP API authentication.
// The apiKey is the key name (e.g. "organizations/.../apiKeys/...") and
// apiSecret is the PEM-encoded EC private key.
func (c *CoinbaseConnector) buildJWT(apiKey, apiSecret, method, path string) (string, error) {
	key, err := parseECPrivateKey(apiSecret)
	if err != nil {
		return "", fmt.Errorf("parsing EC private key: %w", err)
	}

	uri := fmt.Sprintf("%s %s%s", method, "api.coinbase.com", path)

	now := time.Now()
	claims := jwt.MapClaims{
		"sub": apiKey,
		"iss": "cdp",
		"aud": []string{"cdp_service"},
		"nbf": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(),
		"uri": uri,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)

	// Coinbase requires a "kid" header with the API key name and a random nonce.
	token.Header["kid"] = apiKey
	token.Header["nonce"] = randomHex(16)
	token.Header["typ"] = "JWT"

	return token.SignedString(key)
}

// parseECPrivateKey parses a PEM-encoded EC private key. It accepts both
// SEC 1 (EC PRIVATE KEY) and PKCS#8 (PRIVATE KEY) formats. Literal "\n"
// escape sequences (common when copying from JSON or .env files) are
// converted to real newlines before parsing.
func parseECPrivateKey(pemStr string) (*ecdsa.PrivateKey, error) {
	// Normalize literal \n sequences to real newlines.
	normalized := strings.ReplaceAll(pemStr, `\n`, "\n")

	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
	}

	switch block.Type {
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		ecKey, ok := parsed.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is not an EC key")
		}
		return ecKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}
}

// randomHex generates a random hex string of the given byte length.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// GenerateTestECKey creates a P-256 EC key pair for testing.
func generateTestECKey() (*ecdsa.PrivateKey, string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		panic(err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	return key, string(pemBlock)
}

// init ensures jwt.SigningMethodES256 is registered (it is by default).
func init() {
	_ = jwt.SigningMethodES256
}
