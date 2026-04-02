package kraken

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

var pairMap = map[string]string{
	"BTCEUR":  "XBTEUR",
	"ETHEUR":  "ETHEUR",
	"SOLEUR":  "SOLEUR",
	"XRPEUR":  "XRPEUR",
	"ADAEUR":  "ADAEUR",
	"LINKEUR": "LINKEUR",
	"AVAXEUR": "AVAXEUR",
	"DOGEEUR": "DOGEEUR",
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

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
	}
}

func (c *Client) Name() string {
	return "kraken"
}

func (c *Client) Supports(request ingest.Request) bool {
	_, ok := pairMap[request.Pair]
	return ok && request.Interval != ""
}

func (c *Client) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	pairCode, ok := pairMap[request.Pair]
	if !ok {
		return nil, fmt.Errorf("pair %s is not mapped for kraken", request.Pair)
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
	defer resp.Body.Close()

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
		return nil, fmt.Errorf("kraken error: %s", strings.Join(payload.Error, ", "))
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

func krakenInterval(interval string) (int, error) {
	switch interval {
	case "1h":
		return 60, nil
	case "1d":
		return 1440, nil
	default:
		return 0, fmt.Errorf("unsupported interval %q", interval)
	}
}

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
