package planner

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

func BenchmarkScheduler_BuildInitialBackfillTasks_Massive(b *testing.B) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour)
	// Force 1h granularity
	scheduler.backfillWindow1H = time.Hour

	now := time.Now().UTC()

	pairs := make([]pair.Pair, 50)
	syncStates := make(map[string]SyncState)
	for i := 0; i < 50; i++ {
		symbol := "PAIR" + string(rune(i))
		pairs[i] = pair.Pair{Symbol: symbol}
		syncStates[symbol] = SyncState{
			HourlyBackfillCompleted: false,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, syncStates, make(map[string]map[string]bool), nil, now, 1000)
		if len(tasks) == 0 {
			b.Fatal("expected tasks")
		}
	}
}

func TestScheduler_StressTaskGeneration(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour)

	now := time.Now().UTC()

	pairs := make([]pair.Pair, 20)
	syncStates := make(map[string]SyncState)
	for i := 0; i < 20; i++ {
		symbol := "STRESS" + string(rune(i))
		pairs[i] = pair.Pair{Symbol: symbol}
		syncStates[symbol] = SyncState{
			HourlyBackfillCompleted: false,
		}
	}

	// Without a floor, we use the limited variant to cap generation.
	// Request a large batch to stress the generator.
	limit := 50000
	startGen := time.Now()
	tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, syncStates, make(map[string]map[string]bool), nil, now, limit)
	duration := time.Since(startGen)

	t.Logf("Generated %d tasks in %v", len(tasks), duration)

	if len(tasks) != limit {
		t.Errorf("expected %d tasks (capped by limit), got %d", limit, len(tasks))
	}

	if duration > 10*time.Second {
		t.Errorf("Task generation too slow: %v", duration)
	}
}
