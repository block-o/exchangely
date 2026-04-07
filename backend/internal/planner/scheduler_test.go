package planner

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

func TestBuildInitialBackfillTasksPartitionsByPairAndInterval(t *testing.T) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildInitialBackfillTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        now.Add(-48 * time.Hour),
			HourlyBackfillCompleted: false,
		},
	}, make(map[string]map[string]bool), now)

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
	if seenPairIntervals["BTCEUR:1d"] || seenPairIntervals["ETHUSD:1d"] {
		t.Fatal("did not expect daily tasks before hourly backfill completion")
	}
	if !seenPairIntervals["ETHUSD:1h"] {
		t.Fatal("expected ETHUSD hourly tasks")
	}
}

func TestBuildRealtimeTasksStartBeforeBackfillCompletion(t *testing.T) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 34, 0, 0, time.UTC)

	tasks := scheduler.BuildRealtimeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        now,
			DailyLastSynced:         now.Truncate(24 * time.Hour),
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  true,
		},
		"ETHUSD": {
			HourlyLastSynced:        now,
			HourlyBackfillCompleted: false,
			DailyBackfillCompleted:  false,
		},
	}, now)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks (2 realtime, 1 sanity), got %d", len(tasks))
	}
	foundETHRealtime := false
	foundETHIntegrity := false
	for _, task := range tasks {
		if task.Pair == "ETHUSD" && task.Type == "live_ticker" {
			foundETHRealtime = true
		}
		if task.Pair == "ETHUSD" && task.Type == "integrity_check" {
			foundETHIntegrity = true
		}
	}
	if !foundETHRealtime {
		t.Fatalf("expected realtime task for ETHUSD before hourly backfill completion")
	}
	if foundETHIntegrity {
		t.Fatalf("did not expect integrity task for ETHUSD before live coverage matures")
	}
}

func TestBuildInitialBackfillTasksStopsAtRealtimeCutover(t *testing.T) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 34, 0, 0, time.UTC)
	realtimeStarted := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildInitialBackfillTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC),
			HourlyRealtimeStartedAt: realtimeStarted,
			HourlyBackfillCompleted: false,
		},
	}, make(map[string]map[string]bool), now)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 capped backfill tasks, got %d", len(tasks))
	}
	// With 1h granularity, tasks[1] should end at realtimeStarted (10:00)
	if !tasks[1].WindowEnd.Equal(realtimeStarted) {
		t.Fatalf("expected backfill to stop at realtime cutover %s, got %s", realtimeStarted, tasks[1].WindowEnd)
	}
}

func TestBuildConsolidationTasksIncludesOnlyFullyCaughtUpPairs(t *testing.T) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 34, 0, 0, time.UTC)

	tasks := scheduler.BuildConsolidationTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  true,
		},
		"ETHUSD": {
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  false,
		},
	}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 consolidation task, got %d", len(tasks))
	}
	if tasks[0].Type != "consolidation" || tasks[0].Pair != "BTCEUR" || tasks[0].Interval != "1d" {
		t.Fatalf("unexpected consolidation task: %+v", tasks[0])
	}
	expectedStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if !tasks[0].WindowStart.Equal(expectedStart) || !tasks[0].WindowEnd.Equal(expectedEnd) {
		t.Fatalf("unexpected consolidation window: %+v", tasks[0])
	}
}

// TestRealtimeTasksGenerateDistinctIDsPerPollWindow verifies that calling
// BuildRealtimeTasks at two different times within the same hour produces
// tasks with different IDs. Before the sub-hour polling fix, both calls
// would generate the same ID (truncated to the hour), causing the planner's
// Enqueue to silently drop the second one—leaving prices stale for up to 1 hour.
func TestRealtimeTasksGenerateDistinctIDsPerPollWindow(t *testing.T) {
	scheduler := NewScheduler(2*time.Minute, 5*time.Minute)

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

	if len(tasks1) != 2 || len(tasks2) != 2 {
		t.Fatalf("expected 2 tasks each (1 realtime, 1 sanity), got %d and %d", len(tasks1), len(tasks2))
	}
	if tasks1[0].Interval != "realtime" {
		t.Fatalf("expected realtime interval, got %q", tasks1[0].Interval)
	}
	if tasks1[0].ID == tasks2[0].ID {
		t.Fatalf("expected distinct task IDs across poll windows, both got %q", tasks1[0].ID)
	}

	// But two calls within the SAME 2-minute window should produce the same ID.
	t3 := time.Date(2026, 4, 2, 12, 0, 30, 0, time.UTC) // 30s into the same window as t1
	tasks3 := scheduler.BuildRealtimeTasks(pairs, caughtUp, t3)
	// Each tick emits 2 tasks: 1 live_ticker + 1 integrity_check.
	if len(tasks3) != 2 {
		t.Fatalf("expected 2 tasks (realtime + sanity), got %d", len(tasks3))
	}
	// The live_ticker ID (index 0) must be the same across calls in the same poll window.
	if tasks1[0].ID != tasks3[0].ID {
		t.Fatalf("expected same task ID within same poll window, got %q vs %q", tasks1[0].ID, tasks3[0].ID)
	}
}

// TestNewSchedulerDefaultsPollInterval verifies that a zero or negative
// pollInterval falls back to the 2-minute default.
func TestNewSchedulerDefaultsPollInterval(t *testing.T) {
	s := NewScheduler(0, 0)
	if s.realtimePollInterval != 2*time.Minute {
		t.Fatalf("expected 2m default, got %v", s.realtimePollInterval)
	}
	s2 := NewScheduler(-1*time.Second, -1*time.Second)
	if s2.realtimePollInterval != 2*time.Minute {
		t.Fatalf("expected 2m default for negative, got %v", s2.realtimePollInterval)
	}
}

// TestBuildCleanupTask verifies that the cleanup task is correctly generated
// with a unique ID per day.
func TestBuildCleanupTask(t *testing.T) {
	s := NewScheduler(2*time.Minute, 5*time.Minute)
	now1 := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	now2 := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	task1 := s.BuildCleanupTask(now1)
	task2 := s.BuildCleanupTask(now2)

	if task1.Type != "task_cleanup" {
		t.Errorf("expected cleanup type, got %s", task1.Type)
	}

	if task1.ID == task2.ID {
		t.Errorf("expected distinct daily IDs, both got %s", task1.ID)
	}

	expectedID1 := "task_cleanup:daily:1775088000"
	if task1.ID != expectedID1 {
		t.Errorf("expected ID %s, got %s", expectedID1, task1.ID)
	}
}
