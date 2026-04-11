package cryptodatadownload

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	graph      *availabilityGraph
	now        func() time.Time
}

func NewClient(baseURL string, apiBaseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://www.cryptodatadownload.com"
	}
	if apiBaseURL == "" {
		apiBaseURL = "https://api.cryptodatadownload.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		graph:      fetchAvailabilityGraph(apiBaseURL, httpClient),
		now:        time.Now,
	}
}

func (c *Client) Name() string {
	return "cryptodatadownload"
}

func (c *Client) Capabilities() provider.Capability {
	return provider.CapHistorical
}

func (c *Client) Supports(request provider.Request) bool {
	if request.Interval != "1h" && request.Interval != "1d" {
		return false
	}

	cutoff := c.now().UTC().Truncate(24 * time.Hour)
	if request.EndTime.UTC().After(cutoff) {
		return false
	}

	if c.graph == nil || len(c.graph.entries) == 0 {
		return false
	}

	tf := mapTimeframe(request.Interval)
	_, ok := c.graph.resolve(normalizeSymbol(request.Pair), tf, request.StartTime.UTC(), request.EndTime.UTC())
	return ok
}

func (c *Client) FetchCandles(ctx context.Context, request provider.Request) ([]candle.Candle, error) {
	if !c.Supports(request) {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}
	if !request.EndTime.After(request.StartTime) {
		return nil, nil
	}

	fileURL, ok := c.graph.resolve(normalizeSymbol(request.Pair), mapTimeframe(request.Interval), request.StartTime.UTC(), request.EndTime.UTC())
	if !ok {
		return nil, fmt.Errorf("unsupported request %s %s", request.Pair, request.Interval)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
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

// Availability graph types and helpers

type availabilityGraph struct {
	// entries maps "SYMBOL:TIMEFRAME" -> []availabilityEntry sorted by provider priority
	entries map[string][]availabilityEntry
}

type availabilityEntry struct {
	Exchange  string
	StartDate time.Time
	EndDate   time.Time
	FileURL   string
}

// Global endpoint response (/v1/data/ohlc/all/available)
type globalAvailabilityResponse struct {
	Data []globalAvailabilityRecord `json:"data"`
}

type globalAvailabilityRecord struct {
	Symbol    string `json:"symbol"`
	Exchange  string `json:"exchange"`
	Timeframe string `json:"timeframe"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Type      string `json:"type"`
	Page      string `json:"page"`
}

// Per-provider endpoint response (/v1/data/ohlc/<provider>/available)
type providerAvailabilityResponse struct {
	Data []providerAvailabilityRecord `json:"data"`
}

type providerAvailabilityRecord struct {
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	File      string `json:"file"`
}

// normalizeSymbol converts any symbol format to uppercase concatenated form.
// "BTC/USDT" → "BTCUSDT", "btcusdt" → "BTCUSDT"
func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
}

// mapTimeframe converts backfill interval to CDD API timeframe.
// "1h" → "hour", "1d" → "day"
func mapTimeframe(interval string) string {
	switch interval {
	case "1h":
		return "hour"
	case "1d":
		return "day"
	default:
		return ""
	}
}

// providerPriority returns the sort rank for a given exchange name.
// Binance=0, Bitstamp=1, Bitfinex=2, unknown=3
func providerPriority(exchange string) int {
	switch {
	case strings.EqualFold(exchange, "binance"):
		return 0
	case strings.EqualFold(exchange, "bitstamp"):
		return 1
	case strings.EqualFold(exchange, "bitfinex"):
		return 2
	default:
		return 3
	}
}

// fetchAvailabilityGraph performs the two-level CDD API discovery and builds
// an in-memory availability graph. On any global-level error it returns an
// empty graph. Per-provider errors are logged and that provider is skipped.
func fetchAvailabilityGraph(apiBaseURL string, httpClient *http.Client) *availabilityGraph {
	emptyGraph := &availabilityGraph{entries: make(map[string][]availabilityEntry)}
	priorityProviders := []string{"binance", "bitstamp", "bitfinex"}

	// Step 1: Fetch global endpoint.
	globalURL := strings.TrimRight(apiBaseURL, "/") + "/v1/data/ohlc/all/available"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, globalURL, nil)
	if err != nil {
		slog.Warn("cdd: failed to create global availability request", "error", err)
		return emptyGraph
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Warn("cdd: failed to fetch global availability", "url", globalURL, "error", err)
		return emptyGraph
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("cdd: global availability returned non-200", "url", globalURL, "status", resp.StatusCode)
		return emptyGraph
	}

	var globalResp globalAvailabilityResponse
	if err := json.NewDecoder(resp.Body).Decode(&globalResp); err != nil {
		slog.Warn("cdd: failed to parse global availability JSON", "error", err)
		return emptyGraph
	}

	// Step 2: Filter to Spot entries only.
	// Step 3: Extract unique provider names (lowercased), intersect with priority set.
	discoveredProviders := make(map[string]bool)
	for _, rec := range globalResp.Data {
		if !strings.EqualFold(rec.Type, "Spot") {
			continue
		}
		discoveredProviders[strings.ToLower(rec.Exchange)] = true
	}

	relevantProviders := make([]string, 0, len(priorityProviders))
	for _, p := range priorityProviders {
		if discoveredProviders[p] {
			relevantProviders = append(relevantProviders, p)
		}
	}

	// Step 4-6: Fetch per-provider endpoints, parse, and index.
	graph := &availabilityGraph{entries: make(map[string][]availabilityEntry)}

	for _, provider := range relevantProviders {
		providerURL := strings.TrimRight(apiBaseURL, "/") + "/v1/data/ohlc/" + provider + "/available"
		pCtx, pCancel := context.WithTimeout(context.Background(), 30*time.Second)

		pReq, err := http.NewRequestWithContext(pCtx, http.MethodGet, providerURL, nil)
		if err != nil {
			slog.Warn("cdd: failed to create provider availability request", "provider", provider, "error", err)
			pCancel()
			continue
		}

		pResp, err := httpClient.Do(pReq)
		if err != nil {
			slog.Warn("cdd: failed to fetch provider availability", "provider", provider, "url", providerURL, "error", err)
			pCancel()
			continue
		}

		if pResp.StatusCode != http.StatusOK {
			slog.Warn("cdd: provider availability returned non-200", "provider", provider, "url", providerURL, "status", pResp.StatusCode)
			_ = pResp.Body.Close()
			pCancel()
			continue
		}

		var provResp providerAvailabilityResponse
		if err := json.NewDecoder(pResp.Body).Decode(&provResp); err != nil {
			slog.Warn("cdd: failed to parse provider availability JSON", "provider", provider, "error", err)
			_ = pResp.Body.Close()
			pCancel()
			continue
		}
		_ = pResp.Body.Close()
		pCancel()

		// Step 5: Normalize symbols, parse dates, store entries.
		for _, rec := range provResp.Data {
			startDate, err := parseAvailabilityDate(rec.StartDate)
			if err != nil {
				slog.Warn("cdd: failed to parse start_date", "provider", provider, "symbol", rec.Symbol, "raw", rec.StartDate, "error", err)
				continue
			}
			endDate, err := parseAvailabilityDate(rec.EndDate)
			if err != nil {
				slog.Warn("cdd: failed to parse end_date", "provider", provider, "symbol", rec.Symbol, "raw", rec.EndDate, "error", err)
				continue
			}

			sym := normalizeSymbol(rec.Symbol)
			key := sym + ":" + rec.Timeframe

			graph.entries[key] = append(graph.entries[key], availabilityEntry{
				Exchange:  provider,
				StartDate: startDate,
				EndDate:   endDate,
				FileURL:   rec.File,
			})
		}
	}

	// Step 6: Sort entries per key by provider priority.
	for key := range graph.entries {
		sort.Slice(graph.entries[key], func(i, j int) bool {
			return providerPriority(graph.entries[key][i].Exchange) < providerPriority(graph.entries[key][j].Exchange)
		})
	}

	return graph
}

// resolve returns the file URL of the highest-priority entry whose date range
// overlaps the request window [start, end). Returns ("", false) when no match.
func (g *availabilityGraph) resolve(symbol, timeframe string, start, end time.Time) (string, bool) {
	entries, ok := g.entries[symbol+":"+timeframe]
	if !ok {
		return "", false
	}
	for _, e := range entries {
		if start.Before(e.EndDate) && end.After(e.StartDate) {
			return e.FileURL, true
		}
	}
	return "", false
}

// parseAvailabilityDate tries both "2006-01-02 15:04:05" and "2006-01-02" formats.
func parseAvailabilityDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %q", raw)
}
