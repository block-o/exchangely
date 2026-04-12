package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// BitcoinConnector fetches BTC balance via the Blockstream/Mempool.space API.
type BitcoinConnector struct {
	baseURL    string
	httpClient *http.Client
}

// NewBitcoinConnector creates a connector for Bitcoin balance lookups.
func NewBitcoinConnector(baseURL string, httpClient *http.Client) *BitcoinConnector {
	if baseURL == "" {
		baseURL = "https://blockstream.info/api"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &BitcoinConnector{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *BitcoinConnector) Chain() string { return "bitcoin" }

func (c *BitcoinConnector) FetchBalances(ctx context.Context, address string) ([]Balance, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("bitcoin connector retrying", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balances, retryable, err := c.fetchBalancesOnce(ctx, address)
		if err == nil {
			return balances, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("bitcoin: max retries exceeded: %w", lastErr)
}

func (c *BitcoinConnector) fetchBalancesOnce(ctx context.Context, address string) ([]Balance, bool, error) {
	reqURL := fmt.Sprintf("%s/address/%s", c.baseURL, address)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("bitcoin: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("bitcoin connector server error", "status", resp.StatusCode)
		return nil, true, fmt.Errorf("bitcoin: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("bitcoin: client error %d", resp.StatusCode)
	}

	var payload struct {
		ChainStats struct {
			FundedTxoSum uint64 `json:"funded_txo_sum"`
			SpentTxoSum  uint64 `json:"spent_txo_sum"`
		} `json:"chain_stats"`
		MempoolStats struct {
			FundedTxoSum uint64 `json:"funded_txo_sum"`
			SpentTxoSum  uint64 `json:"spent_txo_sum"`
		} `json:"mempool_stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false, fmt.Errorf("bitcoin: decode error: %w", err)
	}

	// Balance in satoshis: confirmed + unconfirmed (1 BTC = 1e8 satoshis)
	confirmedSats := payload.ChainStats.FundedTxoSum - payload.ChainStats.SpentTxoSum
	mempoolSats := payload.MempoolStats.FundedTxoSum - payload.MempoolStats.SpentTxoSum
	totalSats := confirmedSats + mempoolSats
	btc := float64(totalSats) / 1e8

	if btc <= 0 {
		return nil, false, nil
	}

	return []Balance{{Asset: "BTC", Quantity: btc}}, false, nil
}
