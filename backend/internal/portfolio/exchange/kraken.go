package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// KrakenConnector fetches account balances from the Kraken REST API.
type KrakenConnector struct {
	baseURL    string
	httpClient *http.Client
}

// NewKrakenConnector creates a connector for the Kraken exchange.
func NewKrakenConnector(baseURL string, httpClient *http.Client) *KrakenConnector {
	if baseURL == "" {
		baseURL = "https://api.kraken.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &KrakenConnector{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *KrakenConnector) Name() string { return "kraken" }

func (c *KrakenConnector) FetchBalances(ctx context.Context, apiKey, apiSecret string) ([]Balance, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("kraken connector retrying", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		balances, retryable, err := c.fetchBalancesOnce(ctx, apiKey, apiSecret)
		if err == nil {
			return balances, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("kraken: max retries exceeded: %w", lastErr)
}

func (c *KrakenConnector) fetchBalancesOnce(ctx context.Context, apiKey, apiSecret string) ([]Balance, bool, error) {
	urlPath := "/0/private/Balance"
	nonce := strconv.FormatInt(time.Now().UnixMilli(), 10)

	data := url.Values{}
	data.Set("nonce", nonce)

	// Kraken signature: HMAC-SHA512(urlPath, SHA256(nonce + postData), base64Decode(apiSecret))
	secretBytes, err := base64.StdEncoding.DecodeString(apiSecret)
	if err != nil {
		return nil, false, fmt.Errorf("kraken: invalid api secret encoding: %w", err)
	}

	sha := sha256.Sum256([]byte(nonce + data.Encode()))
	mac := hmac.New(sha512.New, secretBytes)
	mac.Write([]byte(urlPath))
	mac.Write(sha[:])
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+urlPath, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("API-Key", apiKey)
	req.Header.Set("API-Sign", signature)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("kraken: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		slog.Error("kraken connector server error", "status", resp.StatusCode)
		return nil, true, fmt.Errorf("kraken: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("kraken: client error %d", resp.StatusCode)
	}

	var payload struct {
		Error  []string          `json:"error"`
		Result map[string]string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false, fmt.Errorf("kraken: decode error: %w", err)
	}
	if len(payload.Error) > 0 {
		msg := strings.Join(payload.Error, ", ")
		return nil, false, fmt.Errorf("kraken: api error: %s", msg)
	}

	var balances []Balance
	for asset, qtyStr := range payload.Result {
		qty, err := strconv.ParseFloat(qtyStr, 64)
		if err != nil {
			continue
		}
		if qty <= 0 {
			continue
		}
		balances = append(balances, Balance{
			Asset:    normalizeKrakenAsset(asset),
			Quantity: qty,
		})
	}
	return balances, false, nil
}

// normalizeKrakenAsset maps Kraken's internal asset codes to standard symbols.
func normalizeKrakenAsset(asset string) string {
	asset = strings.ToUpper(asset)
	switch asset {
	case "XXBT", "XBT":
		return "BTC"
	case "XETH":
		return "ETH"
	case "XXRP":
		return "XRP"
	case "XXLM":
		return "XLM"
	case "XXDG", "XDG":
		return "DOGE"
	case "XLTC":
		return "LTC"
	case "ZUSD":
		return "USD"
	case "ZEUR":
		return "EUR"
	case "ZGBP":
		return "GBP"
	case "ZJPY":
		return "JPY"
	}
	// Strip leading X or Z prefix for 4-char codes (Kraken convention)
	if len(asset) == 4 && (asset[0] == 'X' || asset[0] == 'Z') {
		return asset[1:]
	}
	return asset
}
