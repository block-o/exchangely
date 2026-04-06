// Package kraken provides an OHLCV market data provider for the Kraken exchange.
// It handles pair normalization (XBT -> BTC), secondary pair resolution via AssetPairs API,
// and rate-limit cooldown tracking.
package kraken

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest/backfill"
)

// rateLimitCooldown is the default wait time after a 429 response or
// Kraken "Too many requests" error payload.
const rateLimitCooldown = 30 * time.Second

// Client implements the backfill.Source interface for the Kraken API.
// It uses a local cache for pair mapping to minimize redundant AssetPairs calls.
type Client struct {
	baseURL    string
	httpClient *http.Client
	now        func() time.Time // now allows mocking time in tests
	mu         sync.RWMutex
	pairMap    map[string]string // normalized pair -> internal kraken code
	cachedAt   time.Time
	cooldown   time.Time
}

// NewClient returns a default Kraken client pointed at api.kraken.com.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://api.kraken.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		now:        time.Now,
	}
}

// Name returns the canonical source identifier "kraken".
func (c *Client) Name() string {
	return "kraken"
}

// Supports returns true if the request quote is EUR or USD and the interval
// is 1h or 1d. It only supports requests within the current day for live OHLC.
func (c *Client) Supports(request backfill.Request) bool {
	if (request.Quote != "EUR" && request.Quote != "USD") || (request.Interval != "1h" && request.Interval != "1d") {
		return false
	}

	currentDayStart := c.now().UTC().Truncate(24 * time.Hour)
	return request.EndTime.UTC().After(currentDayStart)
}

// FetchCandles retrieves OHLC data from the Kraken public API and maps it
// into the canonical exchangely domain model.
func (c *Client) FetchCandles(ctx context.Context, request backfill.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if err := c.cooldownError(); err != nil {
		return nil, err
	}

	pairCode, err := c.resolvePairCode(ctx, request.Pair)
	if err != nil {
		return nil, err
	}

	interval, err := krakenInterval(request.Interval)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("pair", pairCode)
	params.Set("interval", strconv.Itoa(interval))
	params.Set("since", strconv.FormatInt(request.StartTime.UTC().Unix(), 10))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/0/public/OHLC?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusTooManyRequests {
		c.setCooldown()
		slog.Warn("kraken source rate limited", "pair", request.Pair, "interval", request.Interval, "status", resp.StatusCode)
		return nil, fmt.Errorf("kraken status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kraken status %d", resp.StatusCode)
	}

	var payload struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Error) > 0 {
		message := strings.Join(payload.Error, ", ")
		if strings.Contains(message, "Too many requests") {
			c.setCooldown()
		}
		return nil, fmt.Errorf("kraken error: %s", message)
	}

	rawRows, ok := payload.Result[pairCode]
	if !ok {
		return nil, fmt.Errorf("kraken result missing pair %s", pairCode)
	}

	var rows [][]any
	if err := json.Unmarshal(rawRows, &rows); err != nil {
		return nil, err
	}

	items := make([]candle.Candle, 0, len(rows))
	for _, row := range rows {
		if len(row) < 7 {
			continue
		}

		ts, err := krakenInt(row[0])
		if err != nil {
			return nil, err
		}
		if ts >= request.EndTime.UTC().Unix() {
			continue
		}

		open, err := krakenFloat(row[1])
		if err != nil {
			return nil, err
		}
		high, err := krakenFloat(row[2])
		if err != nil {
			return nil, err
		}
		low, err := krakenFloat(row[3])
		if err != nil {
			return nil, err
		}
		closePrice, err := krakenFloat(row[4])
		if err != nil {
			return nil, err
		}
		volume, err := krakenFloat(row[6])
		if err != nil {
			return nil, err
		}

		items = append(items, candle.Candle{
			Pair:      request.Pair,
			Interval:  request.Interval,
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			Source:    c.Name(),
			Finalized: true,
		})
	}

	return items, nil
}

// resolvePairCode maps a user-facing pair (e.g., BTCEUR) to Kraken's internal
// symbol code (e.g., XXBTZEUR). It uses an internal cache that refreshes
// periodically via the AssetPairs endpoint.
func (c *Client) resolvePairCode(ctx context.Context, pair string) (string, error) {
	normalized := normalizeKrakenPair(pair)

	c.mu.RLock()
	if c.pairMap != nil {
		if code, ok := c.pairMap[normalized]; ok {
			c.mu.RUnlock()
			return code, nil
		}
	}
	c.mu.RUnlock()

	if err := c.loadPairMap(ctx); err != nil {
		return "", err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	code, ok := c.pairMap[normalized]
	if !ok {
		return "", fmt.Errorf("pair %s is not mapped for kraken", pair)
	}
	return code, nil
}

// loadPairMap fetches the complete asset pair list from Kraken to build a
// normalization map. It caches the result for 6 hours.
func (c *Client) loadPairMap(ctx context.Context) error {
	c.mu.RLock()
	if c.pairMap != nil && c.now().UTC().Sub(c.cachedAt) < 6*time.Hour {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/0/public/AssetPairs", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusTooManyRequests {
		c.setCooldown()
		slog.Warn("kraken asset-pairs rate limited", "status", resp.StatusCode)
		return fmt.Errorf("kraken asset pairs status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("kraken asset pairs status %d", resp.StatusCode)
	}

	var payload struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			AltName string `json:"altname"`
			WSName  string `json:"wsname"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if len(payload.Error) > 0 {
		message := strings.Join(payload.Error, ", ")
		if strings.Contains(message, "Too many requests") {
			c.setCooldown()
		}
		return fmt.Errorf("kraken asset pairs error: %s", message)
	}

	pairs := make(map[string]string, len(payload.Result)*3)
	for code, item := range payload.Result {
		keys := []string{
			normalizeKrakenPair(code),
			normalizeKrakenPair(item.AltName),
			normalizeKrakenPair(item.WSName),
		}
		for _, key := range keys {
			if key == "" {
				continue
			}
			pairs[key] = code
		}
	}

	c.mu.Lock()
	c.pairMap = pairs
	c.cachedAt = c.now().UTC()
	c.mu.Unlock()
	slog.Info("kraken asset-pairs cache refreshed", "pair_count", len(pairs))
	return nil
}

// normalizeKrakenPair performs string cleaning and canonical currency remapping.
// Kraken uses legacy X-based codes (XBT for BTC, XDG for DOGE) internally;
// this helper ensures lookups match exchangely standards.
func normalizeKrakenPair(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer("/", "", "-", "", "_", "", " ", "")
	value = replacer.Replace(value)
	value = strings.ReplaceAll(value, "XBT", "BTC")
	value = strings.ReplaceAll(value, "XDG", "DOGE")
	return value
}

// cooldownError returns an error if the source is currently in rate-limit backoff.
func (c *Client) cooldownError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.cooldown.After(c.now().UTC()) {
		return fmt.Errorf("kraken cooldown active until %s", c.cooldown.Format(time.RFC3339))
	}
	return nil
}

// setCooldown triggers a 30-second backoff window for the source.
func (c *Client) setCooldown() {
	c.mu.Lock()
	c.cooldown = c.now().UTC().Add(rateLimitCooldown)
	c.mu.Unlock()
}

// krakenInterval maps exchangely string intervals to Kraken's expected minute values.
func krakenInterval(interval string) (int, error) {
	switch interval {
	case "1m":
		return 1, nil
	case "1h":
		return 60, nil
	case "1d":
		return 1440, nil
	default:
		return 0, fmt.Errorf("unsupported interval %q", interval)
	}
}

// krakenFloat converts various dynamic types from the Kraken JSON response (string or float64)
// to a canonical float64.
func krakenFloat(value any) (float64, error) {
	switch typed := value.(type) {
	case string:
		return strconv.ParseFloat(typed, 64)
	case float64:
		return typed, nil
	default:
		return 0, fmt.Errorf("unsupported float value %T", value)
	}
}

// krakenInt converts various dynamic types from the Kraken JSON response (float64, int64, or string)
// to a canonical int64.
func krakenInt(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported int value %T", value)
	}
}
