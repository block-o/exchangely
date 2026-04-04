package coingecko

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
	apiKey     string
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://api.coingecko.com/api/v3"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
		now:        time.Now,
	}
}

func (c *Client) Name() string {
	return "coingecko"
}

func (c *Client) Supports(request ingest.Request) bool {
	if request.Interval != "1h" {
		return false
	}
	if request.Quote != "EUR" {
		return false
	}
	if _, ok := coinIDForBase(request.Base); !ok {
		return false
	}

	currentDayStart := c.now().UTC().Truncate(24 * time.Hour)
	return request.EndTime.UTC().After(currentDayStart)
}

func (c *Client) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}

	coinID, ok := coinIDForBase(request.Base)
	if !ok {
		return nil, fmt.Errorf("unsupported CoinGecko asset %s", request.Base)
	}

	end := minTime(request.EndTime.UTC(), c.now().UTC())
	if !end.After(request.StartTime.UTC()) {
		return nil, nil
	}

	query := url.Values{}
	query.Set("vs_currency", "eur")
	query.Set("from", strconv.FormatInt(request.StartTime.UTC().Unix(), 10))
	query.Set("to", strconv.FormatInt(end.Unix(), 10))
	query.Set("precision", "full")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/coins/%s/market_chart/range?%s", c.baseURL, coinID, query.Encode()),
		nil,
	)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("coingecko status %d", resp.StatusCode)
	}

	var payload struct {
		Prices [][]float64 `json:"prices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	items := make([]candle.Candle, 0, len(payload.Prices))
	for _, sample := range payload.Prices {
		if len(sample) < 2 {
			continue
		}

		ts := normalizeTimestamp(int64(sample[0]))
		if ts < request.StartTime.UTC().Unix() || ts >= end.Unix() {
			continue
		}
		price := sample[1]

		items = append(items, candle.Candle{
			Pair:      request.Pair,
			Interval:  request.Interval,
			Timestamp: ts,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    0,
			Source:    c.Name(),
			Finalized: true,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no coingecko prices for %s %s", request.Pair, request.Interval)
	}
	return items, nil
}

func coinIDForBase(base string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "BTC":
		return "bitcoin", true
	case "ETH":
		return "ethereum", true
	case "SOL":
		return "solana", true
	case "XRP":
		return "ripple", true
	case "ADA":
		return "cardano", true
	case "LINK":
		return "chainlink", true
	case "AVAX":
		return "avalanche-2", true
	case "DOGE":
		return "dogecoin", true
	default:
		return "", false
	}
}

func normalizeTimestamp(value int64) int64 {
	switch {
	case value >= 1_000_000_000_000_000:
		return value / 1_000_000
	case value >= 1_000_000_000_000:
		return value / 1_000
	default:
		return value
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
