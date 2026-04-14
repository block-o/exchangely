package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

// IntegrityResult captures the outcome of a single day's integrity check.
type IntegrityResult struct {
	PairSymbol      string
	Day             time.Time
	Verified        bool
	GapCount        int
	DivergenceCount int
	SourcesChecked  int
	ErrorMessage    string
}

// IntegrityWriter persists which days have been verified by the integrity check.
type IntegrityWriter interface {
	MarkDayVerified(ctx context.Context, pairSymbol string, day time.Time) error
}

// IntegrityResultWriter persists the full outcome of an integrity check day.
type IntegrityResultWriter interface {
	RecordResult(ctx context.Context, result IntegrityResult) error
}

// IntegrityReader provides the set of already-verified days per pair.
type IntegrityReader interface {
	GetAllVerifiedDays(ctx context.Context) (map[string]map[string]bool, error)
}

type ValidatorExecutor struct {
	sources          []provider.Source
	integrity        IntegrityWriter
	resultWriter     IntegrityResultWriter
	integrityReader  IntegrityReader
	minSources       int
	maxDivergencePct float64
	maxDaysPerRun    int
}

type ValidatorOptions struct {
	MinSources       int
	MaxDivergencePct float64
	MaxDaysPerRun    int
}

func NewValidatorExecutor(sources []provider.Source, integrity IntegrityWriter, integrityReader IntegrityReader, opts ValidatorOptions) *ValidatorExecutor {
	filtered := make([]provider.Source, 0, len(sources))
	for _, s := range sources {
		if s != nil {
			filtered = append(filtered, s)
		}
	}
	if opts.MinSources < 2 {
		opts.MinSources = 2
	}
	if opts.MaxDivergencePct <= 0 {
		opts.MaxDivergencePct = 2.0
	}
	if opts.MaxDaysPerRun <= 0 {
		opts.MaxDaysPerRun = 7
	}
	return &ValidatorExecutor{
		sources:          filtered,
		integrity:        integrity,
		integrityReader:  integrityReader,
		minSources:       opts.MinSources,
		maxDivergencePct: opts.MaxDivergencePct,
		maxDaysPerRun:    opts.MaxDaysPerRun,
	}
}

// SetResultWriter configures the detailed result writer. When set, the executor
// persists gap/divergence counts for every checked day, not just verified ones.
func (v *ValidatorExecutor) SetResultWriter(w IntegrityResultWriter) {
	v.resultWriter = w
}

// dayCheckResult holds the outcome of verifying a single day.
type dayCheckResult struct {
	gapCount        int
	divergenceCount int
	sourcesChecked  int
	err             error
}

// Execute walks the unverified date range for the task's pair, checking up to
// maxDaysPerRun days per execution. Days that pass verification are marked in
// the integrity_coverage table so they are skipped on subsequent runs. When a
// result writer is configured, failed days are also persisted with details.
func (v *ValidatorExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeDataSanity {
		return fmt.Errorf("validator received non-sanity task %q", item.Type)
	}

	slog.Info("integrity sweep started", "pair", item.Pair, "window_start", item.WindowStart, "window_end", item.WindowEnd)

	allVerified, err := v.integrityReader.GetAllVerifiedDays(ctx)
	if err != nil {
		return fmt.Errorf("load integrity coverage: %w", err)
	}
	pairVerified := allVerified[item.Pair]

	start := item.WindowStart.UTC().Truncate(24 * time.Hour)
	end := item.WindowEnd.UTC().Truncate(24 * time.Hour)
	checked := 0
	var lastErr error

	for cursor := start; cursor.Before(end) && checked < v.maxDaysPerRun; cursor = cursor.Add(24 * time.Hour) {
		dayKey := cursor.Format("2006-01-02")
		if pairVerified[dayKey] {
			continue
		}

		result := v.verifyDay(ctx, item.Pair, cursor)
		checked++

		if result.err != nil {
			slog.Warn("integrity check day failed", "pair", item.Pair, "day", dayKey,
				"gaps", result.gapCount, "divergences", result.divergenceCount,
				"sources", result.sourcesChecked, "error", result.err)
			lastErr = result.err

			if v.resultWriter != nil {
				if wErr := v.resultWriter.RecordResult(ctx, IntegrityResult{
					PairSymbol:      item.Pair,
					Day:             cursor,
					Verified:        false,
					GapCount:        result.gapCount,
					DivergenceCount: result.divergenceCount,
					SourcesChecked:  result.sourcesChecked,
					ErrorMessage:    result.err.Error(),
				}); wErr != nil {
					return fmt.Errorf("record failed result: %w", wErr)
				}
			}
			continue
		}

		if err := v.integrity.MarkDayVerified(ctx, item.Pair, cursor); err != nil {
			return fmt.Errorf("mark day verified: %w", err)
		}

		if v.resultWriter != nil {
			if wErr := v.resultWriter.RecordResult(ctx, IntegrityResult{
				PairSymbol:     item.Pair,
				Day:            cursor,
				Verified:       true,
				SourcesChecked: result.sourcesChecked,
			}); wErr != nil {
				return fmt.Errorf("record verified result: %w", wErr)
			}
		}

		slog.Info("integrity check day verified", "pair", item.Pair, "day", dayKey, "sources", result.sourcesChecked)
	}

	if lastErr != nil {
		return fmt.Errorf("integrity sweep had failures for %s: %w", item.Pair, lastErr)
	}

	slog.Info("integrity sweep completed", "pair", item.Pair, "days_checked", checked)
	return nil
}

func (v *ValidatorExecutor) verifyDay(ctx context.Context, pairSymbol string, day time.Time) dayCheckResult {
	base, quote, err := provider.ParsePairSymbol(pairSymbol)
	if err != nil {
		return dayCheckResult{err: err}
	}

	nextDay := day.Add(24 * time.Hour)
	results := make(map[string][]candle.Candle)
	var expectedCandleCount int

	for _, source := range v.sources {
		req := provider.Request{
			Pair:      pairSymbol,
			Base:      base,
			Quote:     quote,
			Interval:  "1h",
			StartTime: day,
			EndTime:   nextDay,
		}

		if !source.Supports(req) {
			continue
		}

		candles, err := source.FetchCandles(ctx, req)
		if err != nil {
			slog.Warn("integrity source fetch failed", "source", source.Name(), "pair", pairSymbol, "day", day.Format("2006-01-02"), "error", err)
			continue
		}

		results[source.Name()] = candles
		if len(candles) > expectedCandleCount {
			expectedCandleCount = len(candles)
		}
	}

	sourcesChecked := len(results)

	if sourcesChecked < v.minSources {
		slog.Debug("integrity check aborted (insufficient peer overlap)", "pair", pairSymbol, "day", day.Format("2006-01-02"), "available_sources", sourcesChecked, "required_sources", v.minSources)
		return dayCheckResult{sourcesChecked: sourcesChecked}
	}

	gapCount := 0
	for name, set := range results {
		if len(set) < expectedCandleCount {
			gapCount++
			slog.Warn("integrity GAP detected",
				"pair", pairSymbol,
				"day", day.Format("2006-01-02"),
				"source", name,
				"missing", expectedCandleCount-len(set),
			)
		}
	}

	byTime := make(map[int64]map[string]float64)
	for sourceName, set := range results {
		for _, c := range set {
			if byTime[c.Timestamp] == nil {
				byTime[c.Timestamp] = make(map[string]float64)
			}
			byTime[c.Timestamp][sourceName] = c.Close
		}
	}

	divergenceCount := 0
	for ts, sourceMap := range byTime {
		if len(sourceMap) < 2 {
			continue
		}

		var maxPrice, minPrice float64 = 0, math.MaxFloat64
		for _, price := range sourceMap {
			if price > maxPrice {
				maxPrice = price
			}
			if price < minPrice {
				minPrice = price
			}
		}

		if minPrice == 0 {
			continue
		}

		variance := ((maxPrice - minPrice) / minPrice) * 100.0
		if variance > v.maxDivergencePct {
			divergenceCount++
			slog.Warn("integrity DIVERGENCE detected",
				"pair", pairSymbol,
				"day", day.Format("2006-01-02"),
				"timestamp", ts,
				"variance_pct", fmt.Sprintf("%.2f%%", variance),
				"threshold_pct", v.maxDivergencePct,
			)
		}
	}

	if gapCount > 0 || divergenceCount > 0 {
		return dayCheckResult{
			gapCount:        gapCount,
			divergenceCount: divergenceCount,
			sourcesChecked:  sourcesChecked,
			err: fmt.Errorf(
				"integrity check failed for %s on %s: %d source gaps, %d price divergences",
				pairSymbol, day.Format("2006-01-02"), gapCount, divergenceCount,
			),
		}
	}

	return dayCheckResult{sourcesChecked: sourcesChecked}
}
