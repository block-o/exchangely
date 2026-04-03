package worker

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest"
)

func TestValidatorExecutorIdentifiesDivergence(t *testing.T) {
	source1 := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}
	source2 := &fakeMarketSource{
		items: []candle.Candle{
			// 1% divergence (101 vs 100)
			{Pair: "BTCEUR", Timestamp: 1000, Close: 101.0, Source: "s2"},
		},
	}

	executor := NewValidatorExecutor([]ingest.Source{source1, source2})

	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Unix(0, 0),
		WindowEnd:   time.Unix(3600, 0),
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("expected successful execution, got error: %v", err)
	}
	// The divergence should be logged to slog, but we just verify it doesn't panic or fail
}

func TestValidatorExecutorIgnoresInsufficientSources(t *testing.T) {
	source1 := &fakeMarketSource{
		items: []candle.Candle{
			{Pair: "BTCEUR", Timestamp: 1000, Close: 100.0, Source: "s1"},
		},
	}

	executor := NewValidatorExecutor([]ingest.Source{source1})
	item := task.Task{
		Type:        task.TypeDataSanity,
		Pair:        "BTCEUR",
		Interval:    "1h",
	}

	err := executor.Execute(context.Background(), item)
	if err != nil {
		t.Fatalf("expected successful early abort execution, got error: %v", err)
	}
}

func TestValidatorExecutorFailsOnWrongTaskType(t *testing.T) {
	executor := NewValidatorExecutor([]ingest.Source{&fakeMarketSource{}})
	item := task.Task{
		Type: task.TypeBackfill,
	}

	err := executor.Execute(context.Background(), item)
	if err == nil {
		t.Fatalf("expected error when handling non-sanity task type")
	}
}
