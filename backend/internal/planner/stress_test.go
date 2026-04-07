package planner

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

func BenchmarkScheduler_BuildInitialBackfillTasks_Massive(b *testing.B) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)
	// Force 1h granularity
	scheduler.backfillWindow1H = time.Hour

	now := time.Now().UTC()
	start := now.AddDate(-3, 0, 0) // 3 years ago

	pairs := make([]pair.Pair, 50)
	syncStates := make(map[string]SyncState)
	for i := 0; i < 50; i++ {
		symbol := "PAIR" + string(rune(i))
		pairs[i] = pair.Pair{Symbol: symbol}
		syncStates[symbol] = SyncState{
			HourlyLastSynced:        start,
			HourlyBackfillCompleted: false,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tasks := scheduler.BuildInitialBackfillTasks(pairs, syncStates, make(map[string]map[string]bool), now)
		if len(tasks) == 0 {
			b.Fatal("expected tasks")
		}
	}
}

func TestScheduler_StressTaskGeneration(t *testing.T) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)

	now := time.Now().UTC()
	// 5 years of missing data for 20 pairs
	start := now.AddDate(-5, 0, 0)

	pairs := make([]pair.Pair, 20)
	syncStates := make(map[string]SyncState)
	for i := 0; i < 20; i++ {
		symbol := "STRESS" + string(rune(i))
		pairs[i] = pair.Pair{Symbol: symbol}
		syncStates[symbol] = SyncState{
			HourlyLastSynced:        start,
			HourlyBackfillCompleted: false,
		}
	}

	startGen := time.Now()
	tasks := scheduler.BuildInitialBackfillTasks(pairs, syncStates, make(map[string]map[string]bool), now)
	duration := time.Since(startGen)

	t.Logf("Generated %d tasks in %v", len(tasks), duration)

	// We expect ~20 pairs * 5 years * 365 days * 24 hours tasks
	// 20 * 5 * 365 * 24 = 876,000 tasks
	if len(tasks) < 800000 {
		t.Errorf("expected around 876k tasks, got %d", len(tasks))
	}

	if duration > 30*time.Second {
		t.Errorf("Task generation too slow: %v", duration)
	}
}
