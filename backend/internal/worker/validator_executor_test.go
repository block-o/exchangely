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
			// 1% divergence (101 vs 100)
			{Pair: "BTCEUR", Timestamp: 1000, Close: 101.0, Source: "s2"},
		},
	}

	executor := NewValidatorExecutor([]provider.Source{source1, source2}, ValidatorOptions{})

	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Unix(0, 0),
		WindowEnd:   time.Unix(3600, 0),
	}

	err := executor.Execute(context.Background(), item)
	if err == nil {
		t.Fatal("expected divergence to fail the validator task")
	}
	if !strings.Contains(err.Error(), "price divergences") {
		t.Fatalf("expected divergence summary error, got: %v", err)
	}
}

func TestValidatorExecutorIgnoresInsufficientSources(t *testing.T) {
	source1 := &fakeMarketSource{
		name: "s1",
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}

	executor := NewValidatorExecutor([]provider.Source{source1}, ValidatorOptions{})
	item := task.Task{
		Type:     task.TypeDataSanity,
		Pair:     "BTCEUR",
		Interval: "1h",
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("expected successful early abort execution, got error: %v", err)
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

	executor := NewValidatorExecutor([]provider.Source{source1, source2}, ValidatorOptions{})
	err := executor.Execute(context.Background(), task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Unix(0, 0),
		WindowEnd:   time.Unix(3600, 0),
	})
	if err == nil {
		t.Fatal("expected coverage gap to fail the validator task")
	}
	if !strings.Contains(err.Error(), "source gaps") {
		t.Fatalf("expected gap summary error, got: %v", err)
	}
}

func TestValidatorExecutorFailsOnWrongTaskType(t *testing.T) {
	executor := NewValidatorExecutor([]provider.Source{&fakeMarketSource{}}, ValidatorOptions{})
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
	executor := NewValidatorExecutor([]provider.Source{source1, source2}, ValidatorOptions{
		MinSources:       2,
		MaxDivergencePct: 1.0,
	})
	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Unix(0, 0),
		WindowEnd:   time.Unix(3600, 0),
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("expected divergence below configured threshold to pass, got %v", err)
	}
}
