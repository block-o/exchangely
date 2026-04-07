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

type GapValidatorExecutor struct {
	candles  CandleStore
	coverage CoverageWriter
}

func NewGapValidatorExecutor(candles CandleStore, coverage CoverageWriter) *GapValidatorExecutor {
	return &GapValidatorExecutor{
		candles:  candles,
		coverage: coverage,
	}
}

func (e *GapValidatorExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeGapValidation {
		return fmt.Errorf("gap validator received non-validation task %q", item.Type)
	}

	day := item.WindowStart.UTC().Truncate(24 * time.Hour)
	nextDay := day.Add(24 * time.Hour)

	slog.Info("data gap validation started", "pair", item.Pair, "day", day.Format("2006-01-02"))

	// 1. Check for Daily candle
	daily, err := e.candles.RawCandles(ctx, item.Pair, "1d", day, nextDay)
	if err != nil {
		return fmt.Errorf("check daily candle: %w", err)
	}
	if len(daily) == 0 {
		return fmt.Errorf("daily candle missing for %s", day.Format("2006-01-02"))
	}

	// 2. Check for 24 Hourly candles
	hourly, err := e.candles.HourlyCandles(ctx, item.Pair, day, nextDay)
	if err != nil {
		return fmt.Errorf("check hourly candles: %w", err)
	}

	// We need 24 unique hours.
	hours := make(map[int64]struct{})
	for _, c := range hourly {
		ts := time.Unix(c.Timestamp, 0).UTC().Truncate(time.Hour).Unix()
		hours[ts] = struct{}{}
	}

	if len(hours) < 24 {
		return fmt.Errorf("incomplete hourly coverage: %d/24 for %s", len(hours), day.Format("2006-01-02"))
	}

	// 3. Mark as complete
	if err := e.coverage.MarkDayComplete(ctx, item.Pair, day); err != nil {
		return fmt.Errorf("mark day complete: %w", err)
	}

	slog.Info("data gap validation complete: day marked as fully populated", "pair", item.Pair, "day", day.Format("2006-01-02"))
	return nil
}
