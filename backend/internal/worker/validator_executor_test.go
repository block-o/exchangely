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

type mockResultWriter struct {
	results []IntegrityResult
}

func (m *mockResultWriter) RecordResult(_ context.Context, r IntegrityResult) error {
	m.results = append(m.results, r)
	return nil
}

func TestValidatorExecutorRecordsVerifiedResult(t *testing.T) {
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
	rw := &mockResultWriter{}

	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{})
	executor.SetResultWriter(rw)

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
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rw.results) != 1 {
		t.Fatalf("expected 1 result recorded, got %d", len(rw.results))
	}

	r := rw.results[0]
	if !r.Verified {
		t.Fatal("expected result to be verified")
	}
	if r.PairSymbol != "BTCEUR" {
		t.Fatalf("expected pair BTCEUR, got %s", r.PairSymbol)
	}
	if r.SourcesChecked != 2 {
		t.Fatalf("expected 2 sources checked, got %d", r.SourcesChecked)
	}
	if r.GapCount != 0 || r.DivergenceCount != 0 {
		t.Fatalf("expected zero gaps/divergences for verified day, got gaps=%d divergences=%d", r.GapCount, r.DivergenceCount)
	}
	if r.ErrorMessage != "" {
		t.Fatalf("expected empty error message for verified day, got %q", r.ErrorMessage)
	}
}

func TestValidatorExecutorRecordsFailedResult(t *testing.T) {
	source1 := &fakeMarketSource{
		name: "s1",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}
	source2 := &fakeMarketSource{
		name: "s2",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 102.0, Source: "s2"},
		},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	rw := &mockResultWriter{}

	executor := NewValidatorExecutor([]provider.Source{source1, source2}, writer, reader, ValidatorOptions{})
	executor.SetResultWriter(rw)

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
		t.Fatal("expected error from divergence")
	}

	if len(rw.results) != 1 {
		t.Fatalf("expected 1 result recorded, got %d", len(rw.results))
	}

	r := rw.results[0]
	if r.Verified {
		t.Fatal("expected result to NOT be verified")
	}
	if r.DivergenceCount != 1 {
		t.Fatalf("expected 1 divergence, got %d", r.DivergenceCount)
	}
	if r.SourcesChecked != 2 {
		t.Fatalf("expected 2 sources checked, got %d", r.SourcesChecked)
	}
	if r.ErrorMessage == "" {
		t.Fatal("expected non-empty error message for failed day")
	}

	// MarkDayVerified should NOT have been called for a failed day.
	if len(writer.days) != 0 {
		t.Fatalf("expected 0 days marked verified, got %d", len(writer.days))
	}
}

func TestValidatorExecutorRecordsMixedResults(t *testing.T) {
	// day1: passes (same close prices), day2: fails (divergent prices)
	source1 := &fakeMarketSource{
		name:  "s1",
		items: []candle.Candle{{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"}},
	}
	source2Divergent := &fakeMarketSource{
		name:  "s2",
		items: []candle.Candle{{Pair: "BTCEUR", Timestamp: 1000, Close: 105.0, Source: "s2"}},
	}

	writer := &mockIntegrityWriter{}
	reader := &mockIntegrityReader{}
	rw := &mockResultWriter{}

	// Use the divergent source so both days see the same data.
	// day1 will pass because we set it as already verified in the reader.
	// day2 will fail because of divergence.
	day1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

	reader.coverage = map[string]map[string]bool{
		"BTCEUR": {day1.Format("2006-01-02"): true},
	}

	executor := NewValidatorExecutor([]provider.Source{source1, source2Divergent}, writer, reader, ValidatorOptions{})
	executor.SetResultWriter(rw)

	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: day1,
		WindowEnd:   day3,
	}

	err := executor.Execute(context.Background(), item)
	if err == nil {
		t.Fatal("expected error from day2 divergence")
	}

	// day1 was skipped (already verified), day2 should have a failed result.
	if len(rw.results) != 1 {
		t.Fatalf("expected 1 result (day2 failed), got %d", len(rw.results))
	}
	if rw.results[0].Verified {
		t.Fatal("expected day2 result to be failed, not verified")
	}
}

func TestValidatorExecutorNoResultWriterStillWorks(t *testing.T) {
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

	// No SetResultWriter call — should still work fine.
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writer.days) != 1 {
		t.Fatalf("expected 1 day verified, got %d", len(writer.days))
	}
}
