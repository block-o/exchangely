package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// EthereumConnector fetches native ETH and ERC-20 token balances via the Etherscan API.
type EthereumConnector struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewEthereumConnector creates a connector for Ethereum balance lookups.
func NewEthereumConnector(baseURL, apiKey string, httpClient *http.Client) *EthereumConnector {
	if baseURL == "" {
		baseURL = "https://api.etherscan.io"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &EthereumConnector{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

func (c *EthereumConnector) Chain() string { return "ethereum" }

func (c *EthereumConnector) FetchBalances(ctx context.Context, address string) ([]Balance, error) {
	var balances []Balance

	ethBalance, err := c.fetchETHBalance(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("ethereum: eth balance: %w", err)
	}
	if ethBalance > 0 {
		balances = append(balances, Balance{Asset: "ETH", Quantity: ethBalance})
	}

	tokenBalances, err := c.fetchERC20Balances(ctx, address)
	if err != nil {
		slog.Warn("ethereum connector: failed to fetch ERC-20 balances, returning ETH only", "error", err)
		return balances, nil
	}
	balances = append(balances, tokenBalances...)

	return balances, nil
}

// --- ETH native balance ---

func (c *EthereumConnector) fetchETHBalance(ctx context.Context, address string) (float64, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("ethereum connector retrying ETH balance", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balance, retryable, err := c.fetchETHBalanceOnce(ctx, address)
		if err == nil {
			return balance, nil
		}
		lastErr = err
		if !retryable {
			return 0, err
		}
	}
	return 0, fmt.Errorf("ethereum: max retries exceeded for ETH balance: %w", lastErr)
}

func (c *EthereumConnector) fetchETHBalanceOnce(ctx context.Context, address string) (float64, bool, error) {
	reqURL := fmt.Sprintf("%s/api?module=account&action=balance&address=%s&tag=latest&apikey=%s",
		c.baseURL, address, c.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, true, fmt.Errorf("ethereum: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("ethereum connector server error", "status", resp.StatusCode)
		return 0, true, fmt.Errorf("ethereum: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return 0, false, fmt.Errorf("ethereum: client error %d", resp.StatusCode)
	}

	var payload struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Result  string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, false, fmt.Errorf("ethereum: decode error: %w", err)
	}
	if payload.Status != "1" {
		return 0, false, fmt.Errorf("ethereum: api error: %s", payload.Message)
	}

	wei, err := strconv.ParseFloat(payload.Result, 64)
	if err != nil {
		return 0, false, fmt.Errorf("ethereum: parse wei: %w", err)
	}
	return wei / 1e18, false, nil
}

// --- ERC-20 token balances ---

func (c *EthereumConnector) fetchERC20Balances(ctx context.Context, address string) ([]Balance, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("ethereum connector retrying ERC-20 balances", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balances, retryable, err := c.fetchERC20BalancesOnce(ctx, address)
		if err == nil {
			return balances, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("ethereum: max retries exceeded for ERC-20 balances: %w", lastErr)
}

func (c *EthereumConnector) fetchERC20BalancesOnce(ctx context.Context, address string) ([]Balance, bool, error) {
	reqURL := fmt.Sprintf("%s/api?module=account&action=tokentx&address=%s&sort=asc&apikey=%s",
		c.baseURL, address, c.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("ethereum: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("ethereum connector server error for ERC-20", "status", resp.StatusCode)
		return nil, true, fmt.Errorf("ethereum: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("ethereum: client error %d", resp.StatusCode)
	}

	var payload struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Result  []struct {
			TokenSymbol  string `json:"tokenSymbol"`
			TokenDecimal string `json:"tokenDecimal"`
			To           string `json:"to"`
			From         string `json:"from"`
			Value        string `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false, fmt.Errorf("ethereum: decode error: %w", err)
	}
	if payload.Status != "1" && payload.Message != "No transactions found" {
		return nil, false, fmt.Errorf("ethereum: api error: %s", payload.Message)
	}

	// Aggregate net token balances from transfer history
	type tokenInfo struct {
		symbol   string
		decimals int
		net      float64
	}
	tokens := make(map[string]*tokenInfo)
	addrLower := strings.ToLower(address)

	for _, tx := range payload.Result {
		symbol := strings.ToUpper(tx.TokenSymbol)
		if symbol == "" {
			continue
		}
		decimals, err := strconv.Atoi(tx.TokenDecimal)
		if err != nil {
			decimals = 18
		}

		value, err := strconv.ParseFloat(tx.Value, 64)
		if err != nil {
			continue
		}
		amount := value / math.Pow(10, float64(decimals))

		info, ok := tokens[symbol]
		if !ok {
			info = &tokenInfo{symbol: symbol, decimals: decimals}
			tokens[symbol] = info
		}

		if strings.ToLower(tx.To) == addrLower {
			info.net += amount
		}
		if strings.ToLower(tx.From) == addrLower {
			info.net -= amount
		}
	}

	var balances []Balance
	for _, info := range tokens {
		if info.net <= 0 {
			continue
		}
		balances = append(balances, Balance{
			Asset:    info.symbol,
			Quantity: info.net,
		})
	}
	return balances, false, nil
}
