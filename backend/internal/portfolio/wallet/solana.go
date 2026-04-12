package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SolanaConnector fetches native SOL and SPL token balances via Solana JSON-RPC.
type SolanaConnector struct {
	rpcURL     string
	httpClient *http.Client
}

// NewSolanaConnector creates a connector for Solana balance lookups.
func NewSolanaConnector(rpcURL string, httpClient *http.Client) *SolanaConnector {
	if rpcURL == "" {
		rpcURL = "https://api.mainnet-beta.solana.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &SolanaConnector{
		rpcURL:     strings.TrimRight(rpcURL, "/"),
		httpClient: httpClient,
	}
}

func (c *SolanaConnector) Chain() string { return "solana" }

func (c *SolanaConnector) FetchBalances(ctx context.Context, address string) ([]Balance, error) {
	var balances []Balance

	solBalance, err := c.fetchSOLBalance(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("solana: sol balance: %w", err)
	}
	if solBalance > 0 {
		balances = append(balances, Balance{Asset: "SOL", Quantity: solBalance})
	}

	splBalances, err := c.fetchSPLTokenBalances(ctx, address)
	if err != nil {
		slog.Warn("solana connector: failed to fetch SPL token balances, returning SOL only", "error", err)
		return balances, nil
	}
	balances = append(balances, splBalances...)

	return balances, nil
}

// --- Native SOL balance ---

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

func (c *SolanaConnector) fetchSOLBalance(ctx context.Context, address string) (float64, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("solana connector retrying SOL balance", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balance, retryable, err := c.fetchSOLBalanceOnce(ctx, address)
		if err == nil {
			return balance, nil
		}
		lastErr = err
		if !retryable {
			return 0, err
		}
	}
	return 0, fmt.Errorf("solana: max retries exceeded for SOL balance: %w", lastErr)
}

func (c *SolanaConnector) fetchSOLBalanceOnce(ctx context.Context, address string) (float64, bool, error) {
	body := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "getBalance",
		Params:  []interface{}{address},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return 0, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(data))
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, true, fmt.Errorf("solana: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("solana connector server error", "status", resp.StatusCode)
		return 0, true, fmt.Errorf("solana: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return 0, false, fmt.Errorf("solana: client error %d", resp.StatusCode)
	}

	var result struct {
		Error  *json.RawMessage `json:"error"`
		Result struct {
			Value uint64 `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, false, fmt.Errorf("solana: decode error: %w", err)
	}
	if result.Error != nil {
		return 0, false, fmt.Errorf("solana: rpc error: %s", string(*result.Error))
	}

	// Value is in lamports (1 SOL = 1e9 lamports)
	return float64(result.Result.Value) / 1e9, false, nil
}

// --- SPL token balances ---

func (c *SolanaConnector) fetchSPLTokenBalances(ctx context.Context, address string) ([]Balance, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("solana connector retrying SPL token balances", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balances, retryable, err := c.fetchSPLTokenBalancesOnce(ctx, address)
		if err == nil {
			return balances, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("solana: max retries exceeded for SPL token balances: %w", lastErr)
}

func (c *SolanaConnector) fetchSPLTokenBalancesOnce(ctx context.Context, address string) ([]Balance, bool, error) {
	body := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "getTokenAccountsByOwner",
		Params: []interface{}{
			address,
			map[string]string{"programId": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"},
			map[string]string{"encoding": "jsonParsed"},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("solana: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("solana connector server error for SPL tokens", "status", resp.StatusCode)
		return nil, true, fmt.Errorf("solana: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("solana: client error %d", resp.StatusCode)
	}

	var result struct {
		Error  *json.RawMessage `json:"error"`
		Result struct {
			Value []struct {
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								Mint        string `json:"mint"`
								TokenAmount struct {
									UIAmount *float64 `json:"uiAmount"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, false, fmt.Errorf("solana: decode error: %w", err)
	}
	if result.Error != nil {
		return nil, false, fmt.Errorf("solana: rpc error: %s", string(*result.Error))
	}

	var balances []Balance
	for _, account := range result.Result.Value {
		info := account.Account.Data.Parsed.Info
		if info.TokenAmount.UIAmount == nil || *info.TokenAmount.UIAmount <= 0 {
			continue
		}
		// Use the mint address as the asset identifier for SPL tokens
		mint := info.Mint
		if mint == "" {
			continue
		}
		balances = append(balances, Balance{
			Asset:    mint,
			Quantity: *info.TokenAmount.UIAmount,
		})
	}
	return balances, false, nil
}
