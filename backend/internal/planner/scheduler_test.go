package planner

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

func TestBuildInitialBackfillTasksPartitionsByPairAndInterval(t *testing.T) {
	scheduler := NewScheduler(2 * time.Minute)
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildInitialBackfillTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSDT"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        now.Add(-48 * time.Hour),
			HourlyBackfillCompleted: false,
		},
	}, now)

	if len(tasks) == 0 {
		t.Fatal("expected backfill tasks")
	}

	seenPairIntervals := map[string]bool{}
	for _, item := range tasks {
		key := item.Pair + ":" + item.Interval
		seenPairIntervals[key] = true
		if !item.WindowEnd.After(item.WindowStart) {
			t.Fatalf("expected positive task window for %s", item.ID)
		}
		if item.Interval == "" {
			t.Fatalf("expected interval to be encoded for %s", item.ID)
		}
	}

	if !seenPairIntervals["BTCEUR:1h"] {
		t.Fatal("expected BTCEUR hourly tasks")
	}
	if seenPairIntervals["BTCEUR:1d"] || seenPairIntervals["ETHUSDT:1d"] {
		t.Fatal("did not expect daily tasks before hourly backfill completion")
	}
	if !seenPairIntervals["ETHUSDT:1h"] {
		t.Fatal("expected ETHUSDT hourly tasks")
	}
}

func TestBuildRealtimeTasksIncludesOnlyCaughtUpPairs(t *testing.T) {
	scheduler := NewScheduler(2 * time.Minute)
	now := time.Date(2026, 4, 2, 12, 34, 0, 0, time.UTC)

	tasks := scheduler.BuildRealtimeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSDT"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        now,
			DailyLastSynced:         now.Truncate(24 * time.Hour),
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  true,
		},
		"ETHUSDT": {
			HourlyLastSynced:        now,
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  false,
		},
	}, now)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 realtime tasks, got %d", len(tasks))
	}
	foundETH := false
	for _, task := range tasks {
		if task.Pair == "ETHUSDT" {
			foundETH = true
		}
	}
	if !foundETH {
		t.Fatalf("expected realtime task for ETHUSDT since hourly backfill is complete")
	}
}

// TestRealtimeTasksGenerateDistinctIDsPerPollWindow verifies that calling
// BuildRealtimeTasks at two different times within the same hour produces
// tasks with different IDs. Before the sub-hour polling fix, both calls
// would generate the same ID (truncated to the hour), causing the planner's
// Enqueue to silently drop the second one—leaving prices stale for up to 1 hour.
func TestRealtimeTasksGenerateDistinctIDsPerPollWindow(t *testing.T) {
	scheduler := NewScheduler(2 * time.Minute)

	caughtUp := map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        time.Now().UTC(),
			DailyLastSynced:         time.Now().UTC().Truncate(24 * time.Hour),
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  true,
		},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	// Two times within the same hour but in different 2-minute windows.
	t1 := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 2, 12, 2, 0, 0, time.UTC)

	tasks1 := scheduler.BuildRealtimeTasks(pairs, caughtUp, t1)
	tasks2 := scheduler.BuildRealtimeTasks(pairs, caughtUp, t2)

	if len(tasks1) != 1 || len(tasks2) != 1 {
		t.Fatalf("expected 1 task each, got %d and %d", len(tasks1), len(tasks2))
	}
	if tasks1[0].ID == tasks2[0].ID {
		t.Fatalf("expected distinct task IDs across poll windows, both got %q", tasks1[0].ID)
	}

	// But two calls within the SAME 2-minute window should produce the same ID.
	t3 := time.Date(2026, 4, 2, 12, 0, 30, 0, time.UTC) // 30s into the same window as t1
	tasks3 := scheduler.BuildRealtimeTasks(pairs, caughtUp, t3)
	if len(tasks3) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks3))
	}
	if tasks1[0].ID != tasks3[0].ID {
		t.Fatalf("expected same task ID within same poll window, got %q vs %q", tasks1[0].ID, tasks3[0].ID)
	}
}

// TestNewSchedulerDefaultsPollInterval verifies that a zero or negative
// pollInterval falls back to the 2-minute default.
func TestNewSchedulerDefaultsPollInterval(t *testing.T) {
	s := NewScheduler(0)
	if s.realtimePollInterval != 2*time.Minute {
		t.Fatalf("expected 2m default, got %v", s.realtimePollInterval)
	}
	s2 := NewScheduler(-1 * time.Second)
	if s2.realtimePollInterval != 2*time.Minute {
		t.Fatalf("expected 2m default for negative, got %v", s2.realtimePollInterval)
	}
}
