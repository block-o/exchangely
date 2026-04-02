package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

var ErrNoSource = errors.New("no supported market source")

type Registry struct {
	sources []ingest.Source
}

func New(sources ...ingest.Source) *Registry {
	filtered := make([]ingest.Source, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filtered = append(filtered, source)
		}
	}

	return &Registry{sources: filtered}
}

func (r *Registry) FetchCandles(ctx context.Context, request ingest.Request) ([]candle.Candle, error) {
	var errs []error
	for _, source := range r.sources {
		if !source.Supports(request) {
			continue
		}

		items, err := source.FetchCandles(ctx, request)
		if err == nil {
			return items, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
	}

	if len(errs) == 0 {
		return nil, ErrNoSource
	}

	return nil, errors.Join(errs...)
}

func ParsePairSymbol(symbol string) (base string, quote string, err error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	quotes := []string{"USDT", "EUR"}

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
