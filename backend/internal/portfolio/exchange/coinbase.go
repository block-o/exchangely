package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CoinbaseConnector fetches account balances from the Coinbase Advanced Trade API.
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

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	message := timestamp + "GET" + path
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("CB-ACCESS-KEY", apiKey)
	req.Header.Set("CB-ACCESS-SIGN", signature)
	req.Header.Set("CB-ACCESS-TIMESTAMP", timestamp)

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
