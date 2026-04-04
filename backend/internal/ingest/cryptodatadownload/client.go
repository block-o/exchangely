package cryptodatadownload

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://www.cryptodatadownload.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		now:        time.Now,
	}
}

func (c *Client) Name() string {
	return "cryptodatadownload"
}

func (c *Client) Supports(request ingest.Request) bool {
	if request.Interval != "1h" && request.Interval != "1d" {
		return false
	}

	switch request.Quote {
	case "EUR", "USDT":
	default:
		return false
	}

	cutoff := c.now().UTC().Truncate(24 * time.Hour)
	return !request.EndTime.UTC().After(cutoff)
}

func (c *Client) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if !request.EndTime.After(request.StartTime) {
		return nil, nil
	}

	exchange, ok := exchangeForQuote(request.Quote)
	if !ok {
		return nil, fmt.Errorf("unsupported quote %s", request.Quote)
	}

	tf, err := timeframeSuffix(request.Interval)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/cdd/%s_%s_%s.csv", c.baseURL, exchange, strings.ToUpper(request.Pair), tf)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("cryptodatadownload status %d", resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1

	items := make([]candle.Candle, 0)
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(record) < 8 {
			continue
		}

		tsRaw, err := strconv.ParseInt(strings.TrimSpace(record[0]), 10, 64)
		if err != nil {
			continue
		}
		ts := normalizeTimestamp(tsRaw)
		if ts < request.StartTime.UTC().Unix() || ts >= request.EndTime.UTC().Unix() {
			continue
		}

		open, err := strconv.ParseFloat(strings.TrimSpace(record[3]), 64)
		if err != nil {
			return nil, err
		}
		high, err := strconv.ParseFloat(strings.TrimSpace(record[4]), 64)
		if err != nil {
			return nil, err
		}
		low, err := strconv.ParseFloat(strings.TrimSpace(record[5]), 64)
		if err != nil {
			return nil, err
		}
		closePrice, err := strconv.ParseFloat(strings.TrimSpace(record[6]), 64)
		if err != nil {
			return nil, err
		}
		volume, err := strconv.ParseFloat(strings.TrimSpace(record[7]), 64)
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

	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp < items[j].Timestamp
	})

	if len(items) == 0 {
		return nil, fmt.Errorf("no cryptodatadownload candles for %s %s", request.Pair, request.Interval)
	}
	return items, nil
}

func exchangeForQuote(quote string) (string, bool) {
	switch quote {
	case "EUR":
		return "Bitfinex", true
	case "USDT":
		return "Binance", true
	default:
		return "", false
	}
}

func timeframeSuffix(interval string) (string, error) {
	switch interval {
	case "1h":
		return "1h", nil
	case "1d":
		return "d", nil
	default:
		return "", fmt.Errorf("unsupported interval %s", interval)
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
