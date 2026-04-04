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
	"github.com/block-o/exchangely/backend/internal/ingest"
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

func (c *Client) Supports(request ingest.Request) bool {
	if request.Quote != "USDT" || (request.Interval != "1h" && request.Interval != "1d") {
		return false
	}

	currentDayStart := c.now().UTC().Truncate(24 * time.Hour)
	return request.EndTime.UTC().After(currentDayStart)
}

func (c *Client) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if err := c.cooldownError(); err != nil {
		return nil, err
	}

	intervalStr := request.Interval
	if intervalStr == "1h" {
		intervalStr = "1m"
	}

	params := url.Values{}
	params.Set("symbol", request.Base+request.Quote)
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
	for _, row := range payload {
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

		items = append(items, candle.Candle{
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
		})
	}

	return items, nil
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
