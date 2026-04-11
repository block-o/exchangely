package worker

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type mockCandleStore struct {
	raw    []candle.Candle
	hourly []candle.Candle
	err    error
}

func (m *mockCandleStore) RawCandles(ctx context.Context, pair, interval string, start, end time.Time) ([]candle.Candle, error) {
	return m.raw, m.err
}
func (m *mockCandleStore) HourlyCandles(ctx context.Context, pair string, start, end time.Time) ([]candle.Candle, error) {
	return m.hourly, m.err
}
func (m *mockCandleStore) UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error {
	return m.err
}
func (m *mockCandleStore) UpsertRawCandles(ctx context.Context, interval string, candles []candle.Candle) error {
	return m.err
}

type mockCoverageWriter struct {
	called bool
}

func (m *mockCoverageWriter) MarkDayComplete(ctx context.Context, pair string, day time.Time) error {
	m.called = true
	return nil
}

type mockCoverageReader struct {
	coverage map[string]map[string]bool
}

func (m *mockCoverageReader) GetAllCompletedDays(ctx context.Context) (map[string]map[string]bool, error) {
	if m.coverage == nil {
		return make(map[string]map[string]bool), nil
	}
	return m.coverage, nil
}

func TestGapValidatorExecutorReturnsErrorOnMissingData(t *testing.T) {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	taskItem := task.Task{
		Type:        task.TypeGapValidation,
		Pair:        "BTCEUR",
		WindowStart: now,
		WindowEnd:   now.Add(24 * time.Hour),
	}

	t.Run("daily candle missing", func(t *testing.T) {
		executor := NewGapValidatorExecutor(&mockCandleStore{raw: nil}, &mockCoverageWriter{}, &mockCoverageReader{})
		err := executor.Execute(context.Background(), taskItem)
		if err == nil {
			t.Fatal("expected error when daily candle is missing")
		}
	})

	t.Run("hourly candles incomplete", func(t *testing.T) {
		hourly := make([]candle.Candle, 23)
		for i := 0; i < 23; i++ {
			hourly[i] = candle.Candle{Timestamp: now.Add(time.Duration(i) * time.Hour).Unix()}
		}
		executor := NewGapValidatorExecutor(&mockCandleStore{
			raw:    []candle.Candle{{}},
			hourly: hourly,
		}, &mockCoverageWriter{}, &mockCoverageReader{})
		err := executor.Execute(context.Background(), taskItem)
		if err == nil {
			t.Fatal("expected error when hourly coverage is incomplete")
		}
	})

	t.Run("full coverage marks complete", func(t *testing.T) {
		hourly := make([]candle.Candle, 24)
		for i := 0; i < 24; i++ {
			hourly[i] = candle.Candle{Timestamp: now.Add(time.Duration(i) * time.Hour).Unix()}
		}
		writer := &mockCoverageWriter{}
		executor := NewGapValidatorExecutor(&mockCandleStore{
			raw:    []candle.Candle{{}},
			hourly: hourly,
		}, writer, &mockCoverageReader{})
		err := executor.Execute(context.Background(), taskItem)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !writer.called {
			t.Fatal("expected coverage to be marked complete")
		}
	})
}

func TestGapValidatorSkipsAlreadyCoveredDays(t *testing.T) {
	day1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)

	hourly := make([]candle.Candle, 24)
	for i := 0; i < 24; i++ {
		hourly[i] = candle.Candle{Timestamp: day2.Add(time.Duration(i) * time.Hour).Unix()}
	}

	writer := &mockCoverageWriter{}
	reader := &mockCoverageReader{
		coverage: map[string]map[string]bool{
			"BTCEUR": {
				day1.Format("2006-01-02"): true, // already covered
			},
		},
	}

	executor := NewGapValidatorExecutor(&mockCandleStore{
		raw:    []candle.Candle{{}},
		hourly: hourly,
	}, writer, reader)

	// Sweep from day1 to day3 — day1 is already covered, day2 has data.
	taskItem := task.Task{
		Type:        task.TypeGapValidation,
		Pair:        "BTCEUR",
		WindowStart: day1,
		WindowEnd:   day3,
	}

	err := executor.Execute(context.Background(), taskItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !writer.called {
		t.Fatal("expected day2 to be marked complete")
	}
}

func TestGapValidatorRejectsWrongTaskType(t *testing.T) {
	executor := NewGapValidatorExecutor(&mockCandleStore{}, &mockCoverageWriter{}, &mockCoverageReader{})
	err := executor.Execute(context.Background(), task.Task{Type: task.TypeBackfill})
	if err == nil {
		t.Fatal("expected error for wrong task type")
	}
}
