package binancevision

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

var errArchiveNotFound = errors.New("binance vision archive not found")

type Client struct {
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://data.binance.vision"
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
	return "binancevision"
}

func (c *Client) Supports(request ingest.Request) bool {
	switch request.Interval {
	case "1h", "1d":
	default:
		return false
	}

	switch request.Quote {
	case "USDT", "EUR":
		archiveCutoff := dayStart(c.now().UTC())
		return !request.EndTime.UTC().After(archiveCutoff)
	default:
		return false
	}
}

func (c *Client) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if !request.EndTime.After(request.StartTime) {
		return nil, nil
	}

	symbol := strings.ToUpper(request.Base + request.Quote)
	seen := make(map[int64]struct{})
	result := make([]candle.Candle, 0)
	var errs []error

	for day := dayStart(request.StartTime.UTC()); day.Before(request.EndTime.UTC()); day = day.AddDate(0, 0, 1) {
		items, err := c.fetchDailyArchive(ctx, symbol, request, day)
		if err != nil {
			if !errors.Is(err, errArchiveNotFound) {
				errs = append(errs, err)
			}
			continue
		}

		for _, item := range items {
			if _, ok := seen[item.Timestamp]; ok {
				continue
			}
			seen[item.Timestamp] = struct{}{}
			result = append(result, item)
		}
	}

	if len(result) > 0 {
		return result, nil
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return nil, fmt.Errorf("no binance vision archives available for %s %s", request.Pair, request.Interval)
}

func (c *Client) fetchDailyArchive(ctx context.Context, symbol string, request ingest.Request, day time.Time) ([]candle.Candle, error) {
	path := fmt.Sprintf(
		"/data/spot/daily/klines/%s/%s/%s-%s-%s.zip",
		symbol,
		request.Interval,
		symbol,
		request.Interval,
		day.Format("2006-01-02"),
	)
	return c.fetchArchive(ctx, path, request)
}

func (c *Client) fetchArchive(ctx context.Context, path string, request ingest.Request) ([]candle.Candle, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errArchiveNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance vision status %d for %s", resp.StatusCode, path)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	reader, err := zip.NewReader(strings.NewReader(string(payload)), int64(len(payload)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) == 0 {
		return nil, fmt.Errorf("binance vision archive %s is empty", path)
	}

	file, err := reader.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	csvReader := csv.NewReader(file)
	csvReader.FieldsPerRecord = -1

	items := make([]candle.Candle, 0)
	for {
		record, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if len(record) < 6 {
			continue
		}

		openTime, err := strconv.ParseInt(record[0], 10, 64)
		if err != nil {
			// Skip header rows if Binance ever adds them.
			continue
		}
		ts := normalizeBinanceTimestamp(openTime)
		if ts < request.StartTime.UTC().Unix() || ts >= request.EndTime.UTC().Unix() {
			continue
		}

		open, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return nil, err
		}
		high, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			return nil, err
		}
		low, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			return nil, err
		}
		closePrice, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			return nil, err
		}
		volume, err := strconv.ParseFloat(record[5], 64)
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

func normalizeBinanceTimestamp(value int64) int64 {
	switch {
	case value >= 1_000_000_000_000_000:
		return value / 1_000_000
	case value >= 1_000_000_000_000:
		return value / 1_000
	default:
		return value
	}
}

func dayStart(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
