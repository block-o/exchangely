package registry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

var ErrNoSource = errors.New("no supported market source")
var ErrNoData = errors.New("market source returned no candles")

// Registry tries the configured market sources in order until one returns usable candles.
type Registry struct {
	sources []ingest.Source
}

// New builds a source registry while dropping nil adapters so partial configs stay valid.
func New(sources ...ingest.Source) *Registry {
	filtered := make([]ingest.Source, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filtered = append(filtered, source)
		}
	}

	return &Registry{sources: filtered}
}

// FetchCandles probes compatible sources in priority order and treats empty responses as fallthrough.
func (r *Registry) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	var errs []error
	attempted := false
	emptyResult := false
	for _, source := range r.sources {
		if !source.Supports(request) {
			continue
		}
		attempted = true
		startedAt := time.Now()
		slog.Info("market source fetch started",
			"source", source.Name(),
			"pair", request.Pair,
			"interval", request.Interval,
			"window_start", request.StartTime.UTC().Format(time.RFC3339),
			"window_end", request.EndTime.UTC().Format(time.RFC3339),
		)

		items, err := source.FetchCandles(ctx, request)
		if err == nil && len(items) > 0 {
			slog.Info("market source fetch completed",
				"source", source.Name(),
				"pair", request.Pair,
				"interval", request.Interval,
				"candle_count", len(items),
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
			return items, nil
		}
		if err == nil {
			slog.Info("market source returned no candles",
				"source", source.Name(),
				"pair", request.Pair,
				"interval", request.Interval,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
			emptyResult = true
			continue
		}
		slog.Warn("market source fetch failed",
			"source", source.Name(),
			"pair", request.Pair,
			"interval", request.Interval,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
	}

	if !attempted {
		slog.Warn("no market source supports request",
			"pair", request.Pair,
			"interval", request.Interval,
			"window_start", request.StartTime.UTC().Format(time.RFC3339),
			"window_end", request.EndTime.UTC().Format(time.RFC3339),
		)
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
