package binance

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

type Client struct {
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
	mu         sync.RWMutex
	cooldown   time.Time
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://api.binance.com"
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

func (c *Client) Name() string {
	return "binance"
}

func (c *Client) Capabilities() provider.Capability {
	return provider.CapHistorical | provider.CapRealtime
}

func (c *Client) Supports(request provider.Request) bool {
	switch request.Quote {
	case "USDT", "EUR", "USD":
		// Binance lists major crypto pairs against all three quote assets.
	default:
		return false
	}
	switch request.Interval {
	case "1h", "1d":
		// Binance klines API returns up to 1000 points. For 1h, that's ~41 days.
		cutoff := c.now().UTC().AddDate(0, 0, -31)
		return !request.EndTime.Before(cutoff)
	case "ticker":
		return true
	default:
		return false
	}
}

func (c *Client) FetchCandles(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if request.Interval == "ticker" {
		return c.fetchTicker(ctx, request)
	}
	return c.fetchOHLC(ctx, request)
}

// fetchTicker uses the Binance 24hr ticker endpoint to build a single-candle
// snapshot representing the current market state for a pair.
func (c *Client) fetchTicker(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
	if err := c.cooldownError(); err != nil {
		return nil, err
	}

	symbol := strings.ToUpper(request.Base + request.Quote)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v3/ticker/24hr?symbol="+symbol, nil)
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

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusTeapot {
		c.setCooldown()
		return nil, fmt.Errorf("binance ticker rate limited")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance ticker status %d", resp.StatusCode)
	}

	var payload struct {
		OpenPrice   string `json:"openPrice"`
		HighPrice   string `json:"highPrice"`
		LowPrice    string `json:"lowPrice"`
		LastPrice   string `json:"lastPrice"`
		Volume      string `json:"volume"`
		QuoteVolume string `json:"quoteVolume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	open, err := strconv.ParseFloat(payload.OpenPrice, 64)
	if err != nil {
		return nil, fmt.Errorf("binance ticker parse open: %w", err)
	}
	high, err := strconv.ParseFloat(payload.HighPrice, 64)
	if err != nil {
		return nil, fmt.Errorf("binance ticker parse high: %w", err)
	}
	low, err := strconv.ParseFloat(payload.LowPrice, 64)
	if err != nil {
		return nil, fmt.Errorf("binance ticker parse low: %w", err)
	}
	last, err := strconv.ParseFloat(payload.LastPrice, 64)
	if err != nil {
		return nil, fmt.Errorf("binance ticker parse last: %w", err)
	}
	vol, err := strconv.ParseFloat(payload.Volume, 64)
	if err != nil {
		return nil, fmt.Errorf("binance ticker parse volume: %w", err)
	}
	quoteVol, err := strconv.ParseFloat(payload.QuoteVolume, 64)
	if err != nil {
		return nil, fmt.Errorf("binance ticker parse quote volume: %w", err)
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
		Volume24H: quoteVol,
		Source:    c.Name(),
		Finalized: false,
	}}, nil
}

func (c *Client) fetchOHLC(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
	if err := c.cooldownError(); err != nil {
		return nil, err
	}

	symbol := strings.ToUpper(request.Base + request.Quote)
	intervalStr := request.Interval
	if intervalStr == "1h" {
		intervalStr = "1m"
	}

	// If the request is for the latest data (within 2 hours of now),
	// also fetch the native 24h volume from the 24hr ticker API.
	var nativeV24H float64
	if c.now().Sub(request.EndTime) < 2*time.Hour {
		v, err := c.fetchNativeV24H(ctx, symbol)
		if err == nil {
			nativeV24H = v
		} else {
			slog.Debug("binance native v24h fetch failed", "pair", request.Pair, "error", err)
		}
	}

	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", intervalStr)
	params.Set("startTime", strconv.FormatInt(request.StartTime.UTC().UnixMilli(), 10))
	params.Set("endTime", strconv.FormatInt(request.EndTime.UTC().UnixMilli(), 10))
	params.Set("limit", "1000")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v3/klines?"+params.Encode(), nil)
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

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusTeapot {
		c.setCooldown()
		slog.Warn("binance source rate limited",
			"pair", request.Pair,
			"interval", request.Interval,
			"status", resp.StatusCode,
		)
		return nil, fmt.Errorf("binance status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance status %d", resp.StatusCode)
	}

	var payload [][]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	items := make([]candle.Candle, 0, len(payload))
	for i, row := range payload {
		if len(row) < 6 {
			continue
		}

		openTime, err := toInt64(row[0])
		if err != nil {
			return nil, err
		}
		open, err := toFloat64(row[1])
		if err != nil {
			return nil, err
		}
		high, err := toFloat64(row[2])
		if err != nil {
			return nil, err
		}
		low, err := toFloat64(row[3])
		if err != nil {
			return nil, err
		}
		closePrice, err := toFloat64(row[4])
		if err != nil {
			return nil, err
		}
		volume, err := toFloat64(row[5])
		if err != nil {
			return nil, err
		}

		cndl := candle.Candle{
			Pair:      request.Pair,
			Interval:  request.Interval,
			Timestamp: openTime / 1000,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			Source:    c.Name(),
			Finalized: true,
		}

		// Only attach the native v24h to the latest candle in the payload
		if nativeV24H > 0 && i == len(payload)-1 {
			cndl.Volume24H = nativeV24H
		}

		items = append(items, cndl)
	}

	return items, nil
}

func (c *Client) fetchNativeV24H(ctx context.Context, symbol string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v3/ticker/24hr?symbol="+symbol, nil)
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

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusTeapot {
		c.setCooldown()
		return 0, fmt.Errorf("rate limited")
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("binance ticker status %d", resp.StatusCode)
	}

	var payload struct {
		QuoteVolume string `json:"quoteVolume"` // quote asset volume
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}

	return strconv.ParseFloat(payload.QuoteVolume, 64)
}

func (c *Client) cooldownError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cooldown.IsZero() || !c.now().UTC().Before(c.cooldown) {
		return nil
	}

	return fmt.Errorf("binance cooldown active until %s", c.cooldown.Format(time.RFC3339))
}

func (c *Client) setCooldown() {
	c.mu.Lock()
	c.cooldown = c.now().UTC().Add(30 * time.Second)
	c.mu.Unlock()
}

func toFloat64(value any) (float64, error) {
	switch typed := value.(type) {
	case string:
		return strconv.ParseFloat(typed, 64)
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	default:
		return 0, fmt.Errorf("unsupported float value %T", value)
	}
}

func toInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported int value %T", value)
	}
}
