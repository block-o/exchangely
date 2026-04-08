// Package provider contains the market data source selection and request contracts
// used by backfill, realtime, and validator flows.
package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

// Request describes a candle fetch window for a specific tracked pair.
type Request struct {
	TaskID    string // optional; threaded into log lines when set
	Pair      string
	Base      string
	Quote     string
	Interval  string
	StartTime time.Time
	EndTime   time.Time
}

// Capability describes the data-fetch profile of a market source.
// Providers declare their capabilities so the registry can skip obviously
// incompatible sources without calling Supports().
type Capability int

const (
	// CapHistorical indicates the source serves archived/historical data.
	CapHistorical Capability = 1 << iota
	// CapRealtime indicates the source serves recent/live market data.
	CapRealtime
)

// Source is the market data adapter contract used by backfill, realtime, and validator flows.
type Source interface {
	Name() string
	Capabilities() Capability
	Supports(request Request) bool
	FetchCandles(ctx context.Context, request Request) ([]candle.Candle, error)
}

// ErrNoSource indicates that no configured historical provider can serve the request.
var ErrNoSource = errors.New("no supported market source")

// ErrNoData indicates that compatible providers were tried but all returned empty results.
var ErrNoData = errors.New("market source returned no candles")

// Registry tries the configured market sources in order until one returns usable candles.
type Registry struct {
	sources    []Source
	requireCap Capability
}

// NewRegistry builds a source registry while dropping nil adapters so partial configs stay valid.
func NewRegistry(sources ...Source) *Registry {
	filtered := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filtered = append(filtered, source)
		}
	}

	return &Registry{sources: filtered}
}

// WithCapability returns a lightweight registry view that only considers sources
// whose declared Capabilities include every bit in the required mask.
// The underlying source slice is shared, not copied.
func (r *Registry) WithCapability(cap Capability) *Registry {
	return &Registry{sources: r.sources, requireCap: cap}
}

// capLabel returns a short human-readable tag for the active capability filter.
func capLabel(c Capability) string {
	switch {
	case c&CapHistorical != 0 && c&CapRealtime != 0:
		return "historical+realtime"
	case c&CapHistorical != 0:
		return "historical"
	case c&CapRealtime != 0:
		return "realtime"
	default:
		return "any"
	}
}

// FetchCandles probes compatible sources in priority order and treats empty responses as fallthrough.
// Sources that do not satisfy the registry's required capability mask are skipped before Supports is called.
func (r *Registry) FetchCandles(ctx context.Context, request Request) ([]candle.Candle, error) {
	mode := capLabel(r.requireCap)
	windowStart := request.StartTime.UTC().Format(time.RFC3339)
	windowEnd := request.EndTime.UTC().Format(time.RFC3339)

	// Build a reusable set of log attributes so task_id is present when set.
	baseAttrs := []any{
		"fetch_mode", mode,
		"pair", request.Pair,
		"interval", request.Interval,
		"window_start", windowStart,
		"window_end", windowEnd,
	}
	if request.TaskID != "" {
		baseAttrs = append([]any{"task_id", request.TaskID}, baseAttrs...)
	}

	var errs []error
	attempted := false
	emptyResult := false
	for _, source := range r.sources {
		if r.requireCap != 0 && source.Capabilities()&r.requireCap != r.requireCap {
			continue
		}
		if !source.Supports(request) {
			continue
		}
		attempted = true
		startedAt := time.Now()
		slog.Debug("market source fetch started", append(append([]any{}, baseAttrs...), "source", source.Name())...)

		items, err := source.FetchCandles(ctx, request)
		if err == nil && len(items) > 0 {
			slog.Info("market source fetch completed", append(append([]any{}, baseAttrs...),
				"source", source.Name(),
				"candle_count", len(items),
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)...)
			return items, nil
		}
		if err == nil {
			slog.Debug("market source returned no candles", append(append([]any{}, baseAttrs...),
				"source", source.Name(),
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)...)
			emptyResult = true
			continue
		}
		slog.Warn("market source fetch failed", append(append([]any{}, baseAttrs...),
			"source", source.Name(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)...)
		errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
	}

	if !attempted {
		slog.Warn("no market source supports request", baseAttrs...)
		return nil, ErrNoSource
	}

	if emptyResult {
		errs = append(errs, ErrNoData)
	}

	return nil, errors.Join(errs...)
}

// ParsePairSymbol splits a tracked pair symbol into base and quote assets for source adapters.
func ParsePairSymbol(symbol string) (base string, quote string, err error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	quotes := []string{"USDT", "USD", "EUR"}

	for _, candidate := range quotes {
		if strings.HasSuffix(symbol, candidate) {
			base = strings.TrimSuffix(symbol, candidate)
			if base == "" {
				break
			}
			return base, candidate, nil
		}
	}

	return "", "", fmt.Errorf("unsupported pair symbol %q", symbol)
}
