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
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BinanceConnector fetches account balances from the Binance REST API.
type BinanceConnector struct {
	baseURL    string
	httpClient *http.Client
}

// NewBinanceConnector creates a connector for the Binance exchange.
func NewBinanceConnector(baseURL string, httpClient *http.Client) *BinanceConnector {
	if baseURL == "" {
		baseURL = "https://api.binance.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &BinanceConnector{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *BinanceConnector) Name() string { return "binance" }

func (c *BinanceConnector) FetchBalances(ctx context.Context, apiKey, apiSecret string) ([]Balance, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("binance connector retrying", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balances, retryable, err := c.fetchBalancesOnce(ctx, apiKey, apiSecret)
		if err == nil {
			// Merge Simple Earn positions (flexible + locked) into spot balances.
			earnBalances := c.fetchEarnPositions(ctx, apiKey, apiSecret)
			return mergeBalances(balances, earnBalances), nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("binance: max retries exceeded: %w", lastErr)
}

// fetchEarnPositions fetches Simple Earn flexible and locked positions and
// returns them as a combined slice. Errors are logged but not propagated
// so that spot balances are still returned if earn endpoints fail.
func (c *BinanceConnector) fetchEarnPositions(ctx context.Context, apiKey, apiSecret string) []Balance {
	var all []Balance

	flexible := c.fetchEarnEndpoint(ctx, apiKey, apiSecret, "/sapi/v1/simple-earn/flexible/position")
	all = append(all, flexible...)

	locked := c.fetchEarnEndpoint(ctx, apiKey, apiSecret, "/sapi/v1/simple-earn/locked/position")
	all = append(all, locked...)

	return all
}

// fetchEarnEndpoint fetches paginated earn positions from a single endpoint.
func (c *BinanceConnector) fetchEarnEndpoint(ctx context.Context, apiKey, apiSecret, path string) []Balance {
	var all []Balance
	current := 1
	size := 100

	for {
		params := url.Values{}
		params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
		params.Set("recvWindow", "5000")
		params.Set("current", strconv.Itoa(current))
		params.Set("size", strconv.Itoa(size))

		mac := hmac.New(sha256.New, []byte(apiSecret))
		mac.Write([]byte(params.Encode()))
		params.Set("signature", hex.EncodeToString(mac.Sum(nil)))

		reqURL := c.baseURL + path + "?" + params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			slog.Warn("binance earn request build failed", "path", path, "error", err)
			return all
		}
		req.Header.Set("X-MBX-APIKEY", apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			slog.Warn("binance earn request failed", "path", path, "error", err)
			return all
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			slog.Warn("binance earn non-200 response", "path", path, "status", resp.StatusCode)
			return all
		}

		var payload struct {
			Rows []struct {
				Asset       string `json:"asset"`
				TotalAmount string `json:"totalAmount"`
			} `json:"rows"`
			Total int `json:"total"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			_ = resp.Body.Close()
			slog.Warn("binance earn decode failed", "path", path, "error", err)
			return all
		}
		_ = resp.Body.Close()

		for _, row := range payload.Rows {
			amount, err := strconv.ParseFloat(row.TotalAmount, 64)
			if err != nil || amount <= 0 {
				continue
			}
			all = append(all, Balance{
				Asset:    strings.ToUpper(row.Asset),
				Quantity: amount,
			})
		}

		// No more pages.
		if len(payload.Rows) < size {
			break
		}
		current++
	}

	return all
}

// mergeBalances combines spot and earn balances, summing quantities for the same asset.
func mergeBalances(spot, earn []Balance) []Balance {
	if len(earn) == 0 {
		return spot
	}

	byAsset := make(map[string]float64, len(spot))
	for _, b := range spot {
		byAsset[b.Asset] += b.Quantity
	}
	for _, b := range earn {
		byAsset[b.Asset] += b.Quantity
	}

	merged := make([]Balance, 0, len(byAsset))
	for asset, qty := range byAsset {
		merged = append(merged, Balance{Asset: asset, Quantity: qty})
	}
	return merged
}

func (c *BinanceConnector) fetchBalancesOnce(ctx context.Context, apiKey, apiSecret string) ([]Balance, bool, error) {
	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("recvWindow", "5000")

	// HMAC-SHA256 signature over the query string
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(params.Encode()))
	params.Set("signature", hex.EncodeToString(mac.Sum(nil)))

	reqURL := c.baseURL + "/api/v3/account?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("binance: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("binance connector server error", "status", resp.StatusCode)
		return nil, true, fmt.Errorf("binance: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("binance: client error %d", resp.StatusCode)
	}

	var payload struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false, fmt.Errorf("binance: decode error: %w", err)
	}

	var balances []Balance
	for _, b := range payload.Balances {
		free, err := strconv.ParseFloat(b.Free, 64)
		if err != nil {
			continue
		}
		locked, err := strconv.ParseFloat(b.Locked, 64)
		if err != nil {
			continue
		}
		total := free + locked
		if total <= 0 {
			continue
		}
		balances = append(balances, Balance{
			Asset:    strings.ToUpper(b.Asset),
			Quantity: total,
		})
	}
	return balances, false, nil
}
