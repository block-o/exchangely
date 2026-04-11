package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type CoverageWriter interface {
	MarkDayComplete(ctx context.Context, pairSymbol string, day time.Time) error
}

type CoverageReader interface {
	GetAllCompletedDays(ctx context.Context) (map[string]map[string]bool, error)
}

type GapValidatorExecutor struct {
	candles        CandleStore
	coverage       CoverageWriter
	coverageReader CoverageReader
	maxDaysPerRun  int
}

func NewGapValidatorExecutor(candles CandleStore, coverage CoverageWriter, coverageReader CoverageReader) *GapValidatorExecutor {
	return &GapValidatorExecutor{
		candles:        candles,
		coverage:       coverage,
		coverageReader: coverageReader,
		maxDaysPerRun:  30,
	}
}

// Execute walks the pair's uncovered date range (WindowStart to WindowEnd),
// checking up to maxDaysPerRun days per execution. Days that pass validation
// are marked in data_coverage so they are skipped on subsequent runs.
func (e *GapValidatorExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeGapValidation {
		return fmt.Errorf("gap validator received non-validation task %q", item.Type)
	}

	slog.Info("gap validation sweep started", "pair", item.Pair, "window_start", item.WindowStart.Format("2006-01-02"), "window_end", item.WindowEnd.Format("2006-01-02"))

	// Load current coverage for this pair.
	allCoverage, err := e.coverageReader.GetAllCompletedDays(ctx)
	if err != nil {
		return fmt.Errorf("load coverage: %w", err)
	}
	pairCoverage := allCoverage[item.Pair]

	start := item.WindowStart.UTC().Truncate(24 * time.Hour)
	end := item.WindowEnd.UTC().Truncate(24 * time.Hour)
	checked := 0
	verified := 0
	var lastErr error

	for cursor := start; cursor.Before(end) && checked < e.maxDaysPerRun; cursor = cursor.Add(24 * time.Hour) {
		dayKey := cursor.Format("2006-01-02")
		if pairCoverage[dayKey] {
			continue
		}

		err := e.validateDay(ctx, item.Pair, cursor)
		if err != nil {
			slog.Debug("gap validation day incomplete", "pair", item.Pair, "day", dayKey, "error", err)
			lastErr = err
			checked++
			continue
		}

		if err := e.coverage.MarkDayComplete(ctx, item.Pair, cursor); err != nil {
			return fmt.Errorf("mark day complete: %w", err)
		}
		slog.Info("gap validation day complete", "pair", item.Pair, "day", dayKey)
		checked++
		verified++
	}

	slog.Info("gap validation sweep completed", "pair", item.Pair, "days_checked", checked, "days_verified", verified)

	// Only fail the task if zero days were verified and there were errors.
	// This allows partial progress — verified days are persisted even if some fail.
	if verified == 0 && lastErr != nil {
		return fmt.Errorf("gap validation sweep found no complete days for %s: %w", item.Pair, lastErr)
	}

	return nil
}

func (e *GapValidatorExecutor) validateDay(ctx context.Context, pairSymbol string, day time.Time) error {
	nextDay := day.Add(24 * time.Hour)

	// Check for daily candle.
	daily, err := e.candles.RawCandles(ctx, pairSymbol, "1d", day, nextDay)
	if err != nil {
		return fmt.Errorf("check daily candle: %w", err)
	}
	if len(daily) == 0 {
		return fmt.Errorf("daily candle missing for %s", day.Format("2006-01-02"))
	}

	// Check for 24 hourly candles.
	hourly, err := e.candles.HourlyCandles(ctx, pairSymbol, day, nextDay)
	if err != nil {
		return fmt.Errorf("check hourly candles: %w", err)
	}

	hours := make(map[int64]struct{})
	for _, c := range hourly {
		ts := time.Unix(c.Timestamp, 0).UTC().Truncate(time.Hour).Unix()
		hours[ts] = struct{}{}
	}

	if len(hours) < 24 {
		return fmt.Errorf("incomplete hourly coverage: %d/24 for %s", len(hours), day.Format("2006-01-02"))
	}

	return nil
}
