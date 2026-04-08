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
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

// rateLimitCooldown is the default wait time after a 429 response or
// Kraken "Too many requests" error payload.
const rateLimitCooldown = 30 * time.Second

// Client implements the provider.Source interface for the Kraken API.
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

func (c *Client) Capabilities() provider.Capability {
	return provider.CapHistorical | provider.CapRealtime
}

// Supports returns true if the request quote is EUR or USD and the interval
// is 1h, 1d, or ticker.
func (c *Client) Supports(request provider.Request) bool {
	if request.Quote != "EUR" && request.Quote != "USD" {
		return false
	}
	switch request.Interval {
	case "1h", "1d":
		// Kraken OHLC returns up to 720 points. For 1h, that's 30 days.
		cutoff := c.now().UTC().AddDate(0, 0, -31)
		return !request.EndTime.Before(cutoff)
	case "ticker":
		return true
	default:
		return false
	}
}

// FetchCandles retrieves OHLC data from the Kraken public API and maps it
// into the canonical exchangely domain model. For "ticker" interval requests
// it uses the Ticker endpoint instead.
func (c *Client) FetchCandles(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if request.Interval == "ticker" {
		return c.fetchTicker(ctx, request)
	}
	return c.fetchOHLC(ctx, request)
}

// fetchTicker uses the Kraken Ticker endpoint to build a single-candle snapshot
// representing the current market state for a pair.
func (c *Client) fetchTicker(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
	if err := c.cooldownError(); err != nil {
		return nil, err
	}

	pairCode, err := c.resolvePairCode(ctx, request.Pair)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/0/public/Ticker?pair="+pairCode, nil)
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
		return nil, fmt.Errorf("kraken ticker rate limited")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kraken ticker status %d", resp.StatusCode)
	}

	var payload struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			A []string `json:"a"` // ask [price, whole lot volume, lot volume]
			B []string `json:"b"` // bid [price, whole lot volume, lot volume]
			C []string `json:"c"` // last trade [price, lot volume]
			V []string `json:"v"` // volume [today, last 24 hours]
			P []string `json:"p"` // vwap [today, last 24 hours]
			L []string `json:"l"` // low [today, last 24 hours]
			H []string `json:"h"` // high [today, last 24 hours]
			O string   `json:"o"` // today's opening price
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Error) > 0 {
		msg := strings.Join(payload.Error, ", ")
		if strings.Contains(msg, "Too many requests") {
			c.setCooldown()
		}
		return nil, fmt.Errorf("kraken ticker error: %s", msg)
	}

	data, ok := payload.Result[pairCode]
	if !ok || len(data.C) < 1 || len(data.H) < 2 || len(data.L) < 2 || len(data.V) < 2 || len(data.P) < 2 {
		return nil, fmt.Errorf("kraken ticker result missing or invalid for %s", pairCode)
	}

	last, err := strconv.ParseFloat(data.C[0], 64)
	if err != nil {
		return nil, fmt.Errorf("kraken ticker parse last: %w", err)
	}
	open, err := strconv.ParseFloat(data.O, 64)
	if err != nil {
		return nil, fmt.Errorf("kraken ticker parse open: %w", err)
	}
	high, err := strconv.ParseFloat(data.H[1], 64)
	if err != nil {
		return nil, fmt.Errorf("kraken ticker parse high: %w", err)
	}
	low, err := strconv.ParseFloat(data.L[1], 64)
	if err != nil {
		return nil, fmt.Errorf("kraken ticker parse low: %w", err)
	}
	vol, err := strconv.ParseFloat(data.V[1], 64)
	if err != nil {
		return nil, fmt.Errorf("kraken ticker parse volume: %w", err)
	}
	vwap, err := strconv.ParseFloat(data.P[1], 64)
	if err != nil {
		return nil, fmt.Errorf("kraken ticker parse vwap: %w", err)
	}

	now := c.now().UTC()
	return []candle.Candle{{
		Pair:      request.Pair,
		Interval:  "ticker",
		Timestamp: now.Truncate(time.Minute).Unix(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     last,
		Volume:    vol,
		Volume24H: vol * vwap,
		Source:    c.Name(),
		Finalized: false,
	}}, nil
}

// fetchOHLC retrieves OHLC candle data from the Kraken public API.
func (c *Client) fetchOHLC(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
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

	// If the request is for the latest data (within 2 hours of now),
	// also fetch the native 24h volume from the Ticker API.
	var nativeV24H float64
	if c.now().Sub(request.EndTime) < 2*time.Hour {
		v, err := c.fetchNativeV24H(ctx, pairCode)
		if err == nil {
			nativeV24H = v
		} else {
			slog.Debug("kraken native v24h fetch failed", "pair", request.Pair, "error", err)
		}
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
	for i, row := range rows {
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

		cndl := candle.Candle{
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
		}

		// Only attach the native v24h to the latest candle in the payload
		if nativeV24H > 0 && i == len(rows)-1 {
			cndl.Volume24H = nativeV24H
		}

		items = append(items, cndl)
	}

	return items, nil
}

func (c *Client) fetchNativeV24H(ctx context.Context, pairCode string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/0/public/Ticker?pair="+pairCode, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusTooManyRequests {
		c.setCooldown()
		return 0, fmt.Errorf("rate limited")
	}

	var payload struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			V []string `json:"v"` // volume [today, last 24 hours]
			P []string `json:"p"` // vwap [today, last 24 hours]
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}
	if len(payload.Error) > 0 {
		return 0, fmt.Errorf("kraken ticker error: %s", strings.Join(payload.Error, ", "))
	}

	data, ok := payload.Result[pairCode]
	if !ok || len(data.V) < 2 || len(data.P) < 2 {
		return 0, fmt.Errorf("kraken ticker result missing or invalid for %s", pairCode)
	}

	vol, err := strconv.ParseFloat(data.V[1], 64)
	if err != nil {
		return 0, err
	}
	vwap, err := strconv.ParseFloat(data.P[1], 64)
	if err != nil {
		return 0, err
	}

	return vol * vwap, nil
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
