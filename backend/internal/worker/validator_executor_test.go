package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

type mockIntegrityWriter struct {
	days []time.Time
}

func (m *mockIntegrityWriter) MarkDayVerified(ctx context.Context, pair string, day time.Time) error {
	m.days = append(m.days, day)
	return nil
}

type mockIntegrityReader struct {
	coverage map[string]map[string]bool
}

func (m *mockIntegrityReader) GetAllVerifiedDays(ctx context.Context) (map[string]map[string]bool, error) {
	if m.coverage == nil {
		return make(map[string]map[string]bool), nil
	}
	return m.coverage, nil
}

func TestValidatorExecutorIdentifiesDivergence(t *testing.T) {
	source1 := &fakeMarketSource{
		name: "s1",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}
	source2 := &fakeMarketSource{
		name: "s2",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 101.0, Source: "s2"},
		},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{})

	day := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day,
		WindowEnd:   day.Add(24 * time.Hour),
	}

	err := executor.Execute(context.Background(), item)
	if err == nil {
		t.Fatal("expected divergence to fail the validator task")
	}
	if !strings.Contains(err.Error(), "integrity sweep had failures") {
		t.Fatalf("expected sweep failure error, got: %v", err)
	}
}

func TestValidatorExecutorIgnoresInsufficientSources(t *testing.T) {
	source1 := &fakeMarketSource{
		name: "s1",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	executor := NewValidatorExecutor([]provider.Source{source1}, writer, reader, ValidatorOptions{})

	day := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day,
		WindowEnd:   day.Add(24 * time.Hour),
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("expected successful early abort execution, got error: %v", err)
	}
	// With insufficient sources, the day should still be marked verified (no error = pass).
	if len(writer.days) != 1 {
		t.Fatalf("expected 1 day marked verified, got %d", len(writer.days))
	}
}

func TestValidatorExecutorFailsOnCoverageGap(t *testing.T) {
	source1 := &fakeMarketSource{
		name: "s1",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
			{Pair: "BTCEUR", Timestamp: 2000, Close: 101.0, Source: "s1"},
		},
	}
	source2 := &fakeMarketSource{
		name: "s2",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s2"},
		},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{})

	day := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	err := executor.Execute(context.Background(), task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day,
		WindowEnd:   day.Add(24 * time.Hour),
	})
	if err == nil {
		t.Fatal("expected coverage gap to fail the validator task")
	}
}

func TestValidatorExecutorFailsOnWrongTaskType(t *testing.T) {
	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	executor := NewValidatorExecutor([]provider.Source{&fakeMarketSource{}}, writer, reader, ValidatorOptions{})
	item := task.Task{
		Type: task.TypeBackfill,
	}

	err := executor.Execute(context.Background(), item)
	if err == nil {
		t.Fatalf("expected error when handling non-sanity task type")
	}
}

func TestValidatorExecutorUsesConfiguredThresholds(t *testing.T) {
	source1 := &fakeMarketSource{
		name: "s1",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}
	source2 := &fakeMarketSource{
		name: "s2",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.6, Source: "s2"},
		},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{
		MinSources:       2,
		MaxDivergencePct: 1.0,
	})

	day := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day,
		WindowEnd:   day.Add(24 * time.Hour),
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("expected divergence below configured threshold to pass, got %v", err)
	}
	if len(writer.days) != 1 {
		t.Fatalf("expected 1 day marked verified, got %d", len(writer.days))
	}
}

func TestValidatorExecutorSkipsAlreadyVerifiedDays(t *testing.T) {
	day1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

	source1 := &fakeMarketSource{
		name:  "s1",
		items: []candle.Candle{{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"}},
	}
	source2 := &fakeMarketSource{
		name:  "s2",
		items: []candle.Candle{{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s2"}},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{
		coverage: map[string]map[string]bool{
			"BTCEUR": {
				day1.Format("2006-01-02"): true, // already verified
			},
		},
	}

	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{})

	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day1,
		WindowEnd:   day3,
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// day1 was already verified, so only day2 should be newly marked.
	if len(writer.days) != 1 {
		t.Fatalf("expected 1 newly verified day (day2), got %d", len(writer.days))
	}
	if !writer.days[0].Equal(day2) {
		t.Fatalf("expected day2 to be verified, got %s", writer.days[0])
	}
}

func TestValidatorExecutorRespectsMaxDaysPerRun(t *testing.T) {
	source1 := &fakeMarketSource{
		name:  "s1",
		items: []candle.Candle{{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"}},
	}
	source2 := &fakeMarketSource{
		name:  "s2",
		items: []candle.Candle{{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s2"}},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}

	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{
		MaxDaysPerRun: 2,
	})

	day1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	day5 := time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC)

	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day1,
		WindowEnd:   day5,
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should only verify 2 days (the cap), not all 5.
	if len(writer.days) != 2 {
		t.Fatalf("expected 2 verified days (capped), got %d", len(writer.days))
	}
}
