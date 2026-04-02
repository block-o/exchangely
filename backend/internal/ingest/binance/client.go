package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
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
	}
}

func (c *Client) Name() string {
	return "binance"
}

func (c *Client) Supports(request ingest.Request) bool {
	return request.Quote == "USDT" && (request.Interval == "1h" || request.Interval == "1d")
}

func (c *Client) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	params := url.Values{}
	params.Set("symbol", request.Base+request.Quote)
	params.Set("interval", request.Interval)
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
	defer resp.Body.Close()

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
