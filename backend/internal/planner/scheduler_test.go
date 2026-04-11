package planner

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

func TestBuildInitialBackfillTasksPartitionsByPairAndInterval(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	// Test hourly backfill for a single pair — tasks walk backwards from yesterday.
	tasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyBackfillCompleted: false,
		},
	}, make(map[string]map[string]bool), nil, now, 10)

	if len(tasks) != 10 {
		t.Fatalf("expected 10 backfill tasks (limited), got %d", len(tasks))
	}

	for _, item := range tasks {
		if item.Pair != "BTCEUR" || item.Interval != "1h" {
			t.Fatalf("expected BTCEUR hourly task, got %+v", item)
		}
		if !item.WindowEnd.After(item.WindowStart) {
			t.Fatalf("expected positive task window for %s", item.ID)
		}
	}

	// Verify daily tasks only appear after hourly backfill is complete.
	dailyTasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  false,
		},
	}, make(map[string]map[string]bool), nil, now, 5)

	if len(dailyTasks) == 0 {
		t.Fatal("expected daily backfill tasks after hourly completion")
	}
	for _, item := range dailyTasks {
		if item.Interval != "1d" {
			t.Fatalf("expected daily interval after hourly completion, got %+v", item)
		}
	}
}

func TestBuildRealtimeTasksStartBeforeBackfillCompletion(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
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

	if len(tasks) != 2 {
		t.Fatalf("expected 2 realtime tasks, got %d", len(tasks))
	}
	foundETHRealtime := false
	for _, task := range tasks {
		if task.Pair == "ETHUSD" && task.Type == "live_ticker" {
			foundETHRealtime = true
		}
	}
	if !foundETHRealtime {
		t.Fatalf("expected realtime task for ETHUSD before hourly backfill completion")
	}
}

func TestBuildInitialBackfillTasksStopsAtRealtimeCutover(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 12, 34, 0, 0, time.UTC)
	realtimeStarted := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)

	// Backfill walks backwards. The ceiling is min(yesterday=April 4, realtimeStarted=April 3 10:00) = April 3 10:00.
	// HourlyLastSynced is at April 3 08:00 (below ceiling), so cursor = 08:00.
	// We expect tasks walking backwards from 08:00 indefinitely, but we limit to 2.
	tasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        time.Date(2026, 4, 3, 8, 0, 0, 0, time.UTC),
			HourlyRealtimeStartedAt: realtimeStarted,
			HourlyBackfillCompleted: false,
		},
	}, make(map[string]map[string]bool), nil, now, 2)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 backfill tasks (limited), got %d", len(tasks))
	}
	// First task (most recent) should be 07:00-08:00
	if !tasks[0].WindowStart.Equal(time.Date(2026, 4, 3, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected first task to start at 07:00, got %s", tasks[0].WindowStart)
	}
	// Second task should be 06:00-07:00
	if !tasks[1].WindowStart.Equal(time.Date(2026, 4, 3, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected second task to start at 06:00, got %s", tasks[1].WindowStart)
	}
}

func TestBuildConsolidationTasksIncludesOnlyFullyCaughtUpPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
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

// TestRealtimeTasksUseStableIDPerPair verifies that the same pair always
// gets the same task ID regardless of when BuildRealtimeTasks is called.
// This ensures only one realtime task per pair can exist in the queue at a time.
func TestRealtimeTasksUseStableIDPerPair(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)

	caughtUp := map[string]SyncState{
		"BTCEUR": {
			HourlyLastSynced:        time.Now().UTC(),
			DailyLastSynced:         time.Now().UTC().Truncate(24 * time.Hour),
			HourlyBackfillCompleted: true,
			DailyBackfillCompleted:  true,
		},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	// Two calls at different times should produce the same realtime task ID.
	t1 := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 2, 12, 0, 10, 0, time.UTC)

	tasks1 := scheduler.BuildRealtimeTasks(pairs, caughtUp, t1)
	tasks2 := scheduler.BuildRealtimeTasks(pairs, caughtUp, t2)

	if len(tasks1) != 1 || len(tasks2) != 1 {
		t.Fatalf("expected 1 realtime task each, got %d and %d", len(tasks1), len(tasks2))
	}
	if tasks1[0].Interval != "realtime" {
		t.Fatalf("expected realtime interval, got %q", tasks1[0].Interval)
	}
	// Stable ID — same pair always gets the same realtime task ID.
	if tasks1[0].ID != tasks2[0].ID {
		t.Fatalf("expected same task ID for same pair across ticks, got %q vs %q", tasks1[0].ID, tasks2[0].ID)
	}
	expectedID := "live_ticker:BTCEUR:realtime"
	if tasks1[0].ID != expectedID {
		t.Fatalf("expected ID %q, got %q", expectedID, tasks1[0].ID)
	}
}

// TestNewSchedulerDefaultsPollInterval verifies that a zero or negative
// pollInterval falls back to the 5-second default.
func TestNewSchedulerDefaultsPollInterval(t *testing.T) {
	s := NewScheduler(0, 0)
	if s.realtimePollInterval != 5*time.Second {
		t.Fatalf("expected 5s default, got %v", s.realtimePollInterval)
	}
	s2 := NewScheduler(-1*time.Second, -1*time.Second)
	if s2.realtimePollInterval != 5*time.Second {
		t.Fatalf("expected 5s default for negative, got %v", s2.realtimePollInterval)
	}
}

// TestBuildCleanupTask verifies that the cleanup task uses a stable ID
// so only one pending/running cleanup task exists at a time.
func TestBuildCleanupTask(t *testing.T) {
	s := NewScheduler(5*time.Second, 5*time.Minute)
	now1 := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	now2 := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	task1 := s.BuildCleanupTask(now1)
	task2 := s.BuildCleanupTask(now2)

	if task1.Type != "task_cleanup" {
		t.Errorf("expected cleanup type, got %s", task1.Type)
	}

	// Stable ID — same across days so only one can be pending at a time.
	if task1.ID != task2.ID {
		t.Errorf("expected same stable ID across days, got %s vs %s", task1.ID, task2.ID)
	}

	expectedID := "task_cleanup:daily"
	if task1.ID != expectedID {
		t.Errorf("expected ID %s, got %s", expectedID, task1.ID)
	}
}

// TestBackfillWalksBackwardsFromYesterday verifies that backfill tasks are
// generated from yesterday going backwards into the past.
func TestBackfillWalksBackwardsFromYesterday(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
	}, make(map[string]map[string]bool), nil, now, 5)

	if len(tasks) != 5 {
		t.Fatalf("expected 5 backfill tasks (limited), got %d", len(tasks))
	}

	// First task should be the most recent (closest to yesterday).
	first := tasks[0]
	if !first.WindowEnd.Equal(yesterday) {
		t.Fatalf("expected first task to end at yesterday %s, got %s", yesterday, first.WindowEnd)
	}

	// All tasks should be ordered most-recent first (descending WindowStart).
	for i := 1; i < len(tasks); i++ {
		if !tasks[i].WindowStart.Before(tasks[i-1].WindowStart) {
			t.Fatalf("expected descending order, task %d starts at %s but task %d starts at %s",
				i-1, tasks[i-1].WindowStart, i, tasks[i].WindowStart)
		}
	}
}

// TestBackfillProbeEmitsOncePerPairPerDay verifies that the daily backfill
// probe generates one task per pair keyed by the calendar day.
func TestBackfillProbeEmitsOncePerPairPerDay(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 30, 0, 0, time.UTC)
	synced := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildBackfillProbeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
		"ETHUSD": {HourlyLastSynced: synced},
	}, now)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 probe tasks, got %d", len(tasks))
	}

	// Probe should target the hour just before the oldest synced point.
	if !tasks[0].WindowEnd.Equal(synced) {
		t.Fatalf("expected probe to end at synced %s, got %s", synced, tasks[0].WindowEnd)
	}
	if !tasks[0].WindowStart.Equal(synced.Add(-time.Hour)) {
		t.Fatalf("expected probe to start 1h before synced, got %s", tasks[0].WindowStart)
	}

	// Same day should produce the same ID (idempotent).
	later := time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC)
	tasks2 := scheduler.BuildBackfillProbeTasks([]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{"BTCEUR": {HourlyLastSynced: synced}}, later)
	if tasks2[0].ID != tasks[0].ID {
		t.Fatalf("expected same probe ID within same day, got %q vs %q", tasks[0].ID, tasks2[0].ID)
	}

	// Different day should produce a different ID.
	nextDay := time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC)
	tasks3 := scheduler.BuildBackfillProbeTasks([]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{"BTCEUR": {HourlyLastSynced: synced}}, nextDay)
	if tasks3[0].ID == tasks[0].ID {
		t.Fatalf("expected different probe ID on different day, both got %q", tasks[0].ID)
	}

	// No probe if HourlyLastSynced is zero.
	noSynced := scheduler.BuildBackfillProbeTasks([]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{"BTCEUR": {}}, now)
	if len(noSynced) != 0 {
		t.Fatalf("expected no probe for pair without sync state, got %d", len(noSynced))
	}
}

// ---------------------------------------------------------------------------
// Realtime dedup — stable ID tests
// ---------------------------------------------------------------------------

// TestRealtimeTaskIDIsStableAcrossHours verifies that the realtime task ID
// does not change when the hour rolls over. The ID is purely pair-based.
func TestRealtimeTaskIDIsStableAcrossHours(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	hour1 := time.Date(2026, 4, 2, 11, 30, 0, 0, time.UTC)
	hour2 := time.Date(2026, 4, 2, 12, 30, 0, 0, time.UTC)

	tasks1 := scheduler.BuildRealtimeTasks(pairs, state, hour1)
	tasks2 := scheduler.BuildRealtimeTasks(pairs, state, hour2)

	rt1 := findTaskByType(tasks1, "live_ticker")
	rt2 := findTaskByType(tasks2, "live_ticker")
	if rt1 == nil || rt2 == nil {
		t.Fatal("expected realtime tasks in both calls")
	}
	if rt1.ID != rt2.ID {
		t.Fatalf("expected stable ID across hours, got %q vs %q", rt1.ID, rt2.ID)
	}
}

// TestRealtimeTaskIDDiffersBetweenPairs verifies that different pairs get
// different stable IDs so they don't collide in the task table.
func TestRealtimeTaskIDDiffersBetweenPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	state := map[string]SyncState{
		"BTCEUR": {},
		"ETHUSD": {},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}, {Symbol: "ETHUSD"}}

	tasks := scheduler.BuildRealtimeTasks(pairs, state, now)

	ids := map[string]bool{}
	for _, item := range tasks {
		if item.Type == "live_ticker" {
			if ids[item.ID] {
				t.Fatalf("duplicate realtime task ID %q across pairs", item.ID)
			}
			ids[item.ID] = true
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 distinct realtime IDs, got %d", len(ids))
	}
}

// TestRealtimeTaskWindowUpdatesEachTick verifies that even though the ID is
// stable, the WindowStart/WindowEnd reflect the current hour so the exchange
// API receives the right time context.
func TestRealtimeTaskWindowUpdatesEachTick(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	state := map[string]SyncState{"BTCEUR": {}}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	t1 := time.Date(2026, 4, 2, 11, 45, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 2, 12, 15, 0, 0, time.UTC)

	rt1 := findTaskByType(scheduler.BuildRealtimeTasks(pairs, state, t1), "live_ticker")
	rt2 := findTaskByType(scheduler.BuildRealtimeTasks(pairs, state, t2), "live_ticker")

	if rt1.WindowStart.Equal(rt2.WindowStart) {
		t.Fatal("expected different WindowStart across hours")
	}
	expectedStart1 := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	expectedStart2 := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	if !rt1.WindowStart.Equal(expectedStart1) {
		t.Fatalf("expected WindowStart %s, got %s", expectedStart1, rt1.WindowStart)
	}
	if !rt2.WindowStart.Equal(expectedStart2) {
		t.Fatalf("expected WindowStart %s, got %s", expectedStart2, rt2.WindowStart)
	}
}

// TestRealtimeTaskSkipsPairWithoutSyncState verifies that pairs not yet
// present in the sync state map are skipped (no crash, no task).
func TestRealtimeTaskSkipsPairWithoutSyncState(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildRealtimeTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{}, // empty — BTCEUR not present
		now,
	)

	for _, item := range tasks {
		if item.Type == "live_ticker" {
			t.Fatalf("did not expect realtime task for pair without sync state, got %+v", item)
		}
	}
}

// TestIntegrityCheckTasksUseStablePerPairIDs verifies that integrity check
// tasks use stable per-pair sweep IDs and only emit for pairs with coverage.
func TestIntegrityCheckTasksUseStablePerPairIDs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	pairs := []pair.Pair{{Symbol: "BTCEUR"}, {Symbol: "ETHUSD"}}

	// BTCEUR has backfill complete, ETHUSD has no coverage yet.
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, HourlyLastSynced: synced},
		"ETHUSD": {HourlyBackfillCompleted: false},
	}

	tasks := scheduler.BuildIntegrityCheckTasks(pairs, state, make(map[string]map[string]bool), now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 integrity task (BTCEUR only), got %d", len(tasks))
	}
	if tasks[0].Pair != "BTCEUR" {
		t.Fatalf("expected BTCEUR, got %s", tasks[0].Pair)
	}
	expectedID := "integrity_check:BTCEUR:sweep"
	if tasks[0].ID != expectedID {
		t.Fatalf("expected ID %q, got %q", expectedID, tasks[0].ID)
	}

	// Same call should produce the same ID (stable).
	tasks2 := scheduler.BuildIntegrityCheckTasks(pairs, state, make(map[string]map[string]bool), now.Add(time.Hour))
	if tasks2[0].ID != tasks[0].ID {
		t.Fatalf("expected stable ID across ticks, got %q vs %q", tasks[0].ID, tasks2[0].ID)
	}
}

// TestIntegrityCheckSkipsFullyVerifiedPairs verifies that pairs with all
// days already verified don't get new integrity tasks.
func TestIntegrityCheckSkipsFullyVerifiedPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, HourlyLastSynced: synced},
	}

	// All days between synced (Apr 3) and yesterday (Apr 4) are verified.
	verified := map[string]map[string]bool{
		"BTCEUR": {
			"2026-04-03": true,
			"2026-04-04": true,
		},
	}

	tasks := scheduler.BuildIntegrityCheckTasks(pairs, state, verified, now)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 integrity tasks for fully verified pair, got %d", len(tasks))
	}
}

// ---------------------------------------------------------------------------
// Backfill probe tests
// ---------------------------------------------------------------------------

// TestBackfillProbeTargetsHourBeforeOldestSynced verifies the probe window
// is exactly one hour before HourlyLastSynced.
func TestBackfillProbeTargetsHourBeforeOldestSynced(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildBackfillProbeTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{"BTCEUR": {HourlyLastSynced: synced}},
		now,
	)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 probe task, got %d", len(tasks))
	}
	probe := tasks[0]
	if probe.Type != "historical_backfill" {
		t.Fatalf("expected historical_backfill type, got %q", probe.Type)
	}
	if probe.Interval != "1h" {
		t.Fatalf("expected 1h interval, got %q", probe.Interval)
	}
	expectedStart := time.Date(2026, 1, 15, 7, 0, 0, 0, time.UTC)
	if !probe.WindowStart.Equal(expectedStart) {
		t.Fatalf("expected probe start %s, got %s", expectedStart, probe.WindowStart)
	}
	if !probe.WindowEnd.Equal(synced) {
		t.Fatalf("expected probe end %s, got %s", synced, probe.WindowEnd)
	}
}

// TestBackfillProbeIDIsIdempotentWithinDay verifies that multiple calls on
// the same calendar day produce the same task ID (so Enqueue deduplicates).
func TestBackfillProbeIDIsIdempotentWithinDay(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	synced := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	state := map[string]SyncState{"BTCEUR": {HourlyLastSynced: synced}}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	morning := time.Date(2026, 4, 5, 6, 0, 0, 0, time.UTC)
	evening := time.Date(2026, 4, 5, 22, 0, 0, 0, time.UTC)

	t1 := scheduler.BuildBackfillProbeTasks(pairs, state, morning)
	t2 := scheduler.BuildBackfillProbeTasks(pairs, state, evening)

	if t1[0].ID != t2[0].ID {
		t.Fatalf("expected same probe ID within same day, got %q vs %q", t1[0].ID, t2[0].ID)
	}
}

// TestBackfillProbeIDChangesAcrossDays verifies that the probe gets a fresh
// ID each calendar day so it re-runs daily.
func TestBackfillProbeIDChangesAcrossDays(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	synced := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	state := map[string]SyncState{"BTCEUR": {HourlyLastSynced: synced}}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	day1 := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	t1 := scheduler.BuildBackfillProbeTasks(pairs, state, day1)
	t2 := scheduler.BuildBackfillProbeTasks(pairs, state, day2)

	if t1[0].ID == t2[0].ID {
		t.Fatalf("expected different probe IDs across days, both got %q", t1[0].ID)
	}
}

// TestBackfillProbeSkipsFreshPairs verifies that pairs with no sync history
// (HourlyLastSynced is zero) don't get a probe — there's nothing to extend.
func TestBackfillProbeSkipsFreshPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildBackfillProbeTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{"BTCEUR": {}},
		now,
	)
	if len(tasks) != 0 {
		t.Fatalf("expected no probe for fresh pair, got %d", len(tasks))
	}
}

// TestBackfillProbeEmitsForMultiplePairs verifies that each pair gets its own
// independent probe task.
func TestBackfillProbeEmitsForMultiplePairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildBackfillProbeTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}, {Symbol: "ETHUSD"}, {Symbol: "XRPUSD"}},
		map[string]SyncState{
			"BTCEUR": {HourlyLastSynced: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
			"ETHUSD": {HourlyLastSynced: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)},
			"XRPUSD": {}, // fresh — should be skipped
		},
		now,
	)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 probe tasks (XRPUSD skipped), got %d", len(tasks))
	}

	seenPairs := map[string]bool{}
	for _, item := range tasks {
		seenPairs[item.Pair] = true
	}
	if !seenPairs["BTCEUR"] || !seenPairs["ETHUSD"] {
		t.Fatalf("expected probes for BTCEUR and ETHUSD, got %v", seenPairs)
	}
}

// TestBackfillProbeUsesCorrectPairCursor verifies that each pair's probe
// targets its own HourlyLastSynced, not a shared cursor.
func TestBackfillProbeUsesCorrectPairCursor(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)

	btcSynced := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	ethSynced := time.Date(2025, 8, 20, 5, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildBackfillProbeTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}, {Symbol: "ETHUSD"}},
		map[string]SyncState{
			"BTCEUR": {HourlyLastSynced: btcSynced},
			"ETHUSD": {HourlyLastSynced: ethSynced},
		},
		now,
	)

	for _, item := range tasks {
		switch item.Pair {
		case "BTCEUR":
			if !item.WindowEnd.Equal(btcSynced) {
				t.Fatalf("BTCEUR probe should end at %s, got %s", btcSynced, item.WindowEnd)
			}
		case "ETHUSD":
			if !item.WindowEnd.Equal(ethSynced) {
				t.Fatalf("ETHUSD probe should end at %s, got %s", ethSynced, item.WindowEnd)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func findTaskByType(tasks []task.Task, taskType string) *task.Task {
	for i := range tasks {
		if tasks[i].Type == taskType {
			return &tasks[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Task description tests
// ---------------------------------------------------------------------------

// TestBackfillTasksHaveDescriptions verifies that backfill tasks generated by
// the scheduler include a non-empty description with the interval and window.
func TestBackfillTasksHaveDescriptions(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
	}, make(map[string]map[string]bool), nil, now, 3)

	for _, item := range tasks {
		if item.Description == "" {
			t.Fatalf("expected non-empty description for backfill task %s", item.ID)
		}
		if item.Interval != "1h" {
			t.Fatalf("expected 1h interval, got %s", item.Interval)
		}
		// Description should mention the interval
		if len(item.Description) < 10 {
			t.Fatalf("description too short: %q", item.Description)
		}
	}
}

// TestConsolidationTasksHaveDescriptions verifies consolidation tasks include
// a description mentioning the target day.
func TestConsolidationTasksHaveDescriptions(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildConsolidationTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true},
	}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 consolidation task, got %d", len(tasks))
	}
	if tasks[0].Description == "" {
		t.Fatal("expected non-empty description for consolidation task")
	}
}

// TestCleanupTaskHasDescription verifies the cleanup task gets a description.
func TestCleanupTaskHasDescription(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	item := scheduler.BuildCleanupTask(now)
	if item.Description == "" {
		t.Fatal("expected non-empty description for cleanup task")
	}
}

// TestNewsFetchTaskHasDescription verifies the news fetch tasks get descriptions.
func TestNewsFetchTasksHaveDescriptions(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	items := scheduler.BuildNewsFetchTasks(now)
	if len(items) != 3 {
		t.Fatalf("expected 3 news fetch tasks (one per source), got %d", len(items))
	}
	for _, item := range items {
		if item.Description == "" {
			t.Fatalf("expected non-empty description for news fetch task %s", item.ID)
		}
		if item.Type != task.TypeNewsFetch {
			t.Fatalf("expected news_fetch type, got %s", item.Type)
		}
	}
}

// TestNewsFetchTasksUseStableIDs verifies that news fetch tasks use stable
// per-source IDs so only one pending/running task per source can exist.
func TestNewsFetchTasksUseStableIDs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	t1 := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 5, 14, 5, 0, 0, time.UTC)

	tasks1 := scheduler.BuildNewsFetchTasks(t1)
	tasks2 := scheduler.BuildNewsFetchTasks(t2)

	if len(tasks1) != len(tasks2) {
		t.Fatalf("expected same count, got %d vs %d", len(tasks1), len(tasks2))
	}

	for i := range tasks1 {
		if tasks1[i].ID != tasks2[i].ID {
			t.Fatalf("expected stable ID across ticks, got %q vs %q", tasks1[i].ID, tasks2[i].ID)
		}
	}

	// Verify IDs are distinct per source.
	ids := map[string]bool{}
	for _, item := range tasks1 {
		if ids[item.ID] {
			t.Fatalf("duplicate news fetch task ID %q", item.ID)
		}
		ids[item.ID] = true
	}
}

// TestRealtimeTasksHaveNoDescription verifies that live_ticker tasks
// intentionally have an empty description (they're self-explanatory).
func TestRealtimeTasksHaveNoDescription(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildRealtimeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true},
	}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 realtime task, got %d", len(tasks))
	}
	if tasks[0].Description != "" {
		t.Fatalf("expected empty description for live_ticker task, got %q", tasks[0].Description)
	}
}

// TestIntegrityCheckTasksHaveDescriptions verifies that integrity_check tasks
// emitted by BuildIntegrityCheckTasks include a description.
func TestIntegrityCheckTasksHaveDescriptions(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildIntegrityCheckTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, HourlyLastSynced: synced},
	}, make(map[string]map[string]bool), now)

	if len(tasks) == 0 {
		t.Fatal("expected at least one integrity check task")
	}
	for _, item := range tasks {
		if item.Description == "" {
			t.Fatal("expected non-empty description for integrity check task")
		}
	}
}

// TestGapValidationTasksHaveDescriptions verifies gap validation tasks include
// a description.
func TestGapValidationTasksHaveDescriptions(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildGapValidationTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
	}, map[string]map[string]bool{}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 gap validation sweep task, got %d", len(tasks))
	}
	if tasks[0].Description == "" {
		t.Fatalf("expected non-empty description for gap validation task %s", tasks[0].ID)
	}
	expectedID := "gap_validation:BTCEUR:sweep"
	if tasks[0].ID != expectedID {
		t.Fatalf("expected ID %q, got %q", expectedID, tasks[0].ID)
	}
}

// TestBackfillProbeTasksHaveDescriptions verifies probe tasks include a description.
func TestBackfillProbeTasksHaveDescriptions(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildBackfillProbeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
	}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 probe task, got %d", len(tasks))
	}
	if tasks[0].Description == "" {
		t.Fatal("expected non-empty description for probe task")
	}
}

// ---------------------------------------------------------------------------
// Round-robin backfill distribution tests
// ---------------------------------------------------------------------------

// TestBackfillRoundRobinDistributesFairly verifies that when multiple pairs
// need backfill, the budget is distributed round-robin so no single pair
// monopolises the entire batch.
func TestBackfillRoundRobinDistributesFairly(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
		{Symbol: "XRPUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
		"XRPUSD": {HourlyBackfillCompleted: false},
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, state, make(map[string]map[string]bool), nil, now, 9)

	if len(tasks) != 9 {
		t.Fatalf("expected 9 backfill tasks, got %d", len(tasks))
	}

	// Count tasks per pair — each should get exactly 3 (9 / 3 pairs).
	counts := map[string]int{}
	for _, item := range tasks {
		counts[item.Pair]++
	}
	for _, sym := range []string{"BTCEUR", "ETHUSD", "XRPUSD"} {
		if counts[sym] != 3 {
			t.Fatalf("expected 3 tasks for %s, got %d (distribution: %v)", sym, counts[sym], counts)
		}
	}
}

// TestBackfillRoundRobinUnevenBudget verifies fair distribution when the
// budget doesn't divide evenly across pairs.
func TestBackfillRoundRobinUnevenBudget(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, state, make(map[string]map[string]bool), nil, now, 5)

	if len(tasks) != 5 {
		t.Fatalf("expected 5 backfill tasks, got %d", len(tasks))
	}

	counts := map[string]int{}
	for _, item := range tasks {
		counts[item.Pair]++
	}

	// With 5 tasks and 2 pairs, one gets 3 and the other gets 2.
	if counts["BTCEUR"] < 2 || counts["BTCEUR"] > 3 {
		t.Fatalf("expected BTCEUR to get 2-3 tasks, got %d", counts["BTCEUR"])
	}
	if counts["ETHUSD"] < 2 || counts["ETHUSD"] > 3 {
		t.Fatalf("expected ETHUSD to get 2-3 tasks, got %d", counts["ETHUSD"])
	}
}

// TestBackfillRoundRobinSkipsCompletedPairs verifies that pairs with completed
// hourly backfill don't starve other pairs of budget.
func TestBackfillRoundRobinSkipsCompletedPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true}, // fully done
		"ETHUSD": {HourlyBackfillCompleted: false},
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, state, make(map[string]map[string]bool), nil, now, 5)

	if len(tasks) != 5 {
		t.Fatalf("expected 5 backfill tasks, got %d", len(tasks))
	}

	// All tasks should go to ETHUSD since BTCEUR is fully complete.
	for _, item := range tasks {
		if item.Pair != "ETHUSD" {
			t.Fatalf("expected all tasks for ETHUSD, got %s for %s", item.Pair, item.ID)
		}
	}
}

// TestBackfillSkipsPairsWithActiveTasks verifies that pairs already having
// pending/running backfill tasks are excluded from new task generation,
// allowing the budget to go to pairs that actually need work.
func TestBackfillSkipsPairsWithActiveTasks(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
		{Symbol: "XRPUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
		"XRPUSD": {HourlyBackfillCompleted: false},
	}

	// BTCEUR and ETHUSD already have active tasks — only XRPUSD should get work.
	activePairs := map[string]bool{
		"BTCEUR": true,
		"ETHUSD": true,
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, state, make(map[string]map[string]bool), activePairs, now, 6)

	for _, item := range tasks {
		if item.Pair != "XRPUSD" {
			t.Fatalf("expected only XRPUSD tasks, got %s for %s", item.Pair, item.ID)
		}
	}
	if len(tasks) != 6 {
		t.Fatalf("expected 6 tasks for XRPUSD, got %d", len(tasks))
	}
}

// TestBackfillAllPairsActiveReturnsEmpty verifies that when all pairs have
// active backfill tasks, no new tasks are generated.
func TestBackfillAllPairsActiveReturnsEmpty(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
	}
	activePairs := map[string]bool{
		"BTCEUR": true,
		"ETHUSD": true,
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(pairs, state, make(map[string]map[string]bool), activePairs, now, 10)

	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks when all pairs are active, got %d", len(tasks))
	}
}

// ---------------------------------------------------------------------------
// Failed-task starvation scenario tests
// ---------------------------------------------------------------------------
// These tests simulate the multi-tick lifecycle where backfill tasks fail and
// verify that the budget shifts to other pairs instead of retrying the same
// ones indefinitely.

// TestFailedTasksDoNotStarveRemainingPairs simulates the exact bug scenario:
// 12 pairs, budget of 9. Tick 1 distributes 9 tasks across 9 pairs. All 9
// fail and go back to pending (with retry delay). Tick 2 should skip those
// 9 pairs (they have active tasks) and give the full budget to the remaining 3.
func TestFailedTasksDoNotStarveRemainingPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	// 12 pairs, all needing hourly backfill.
	allPairs := make([]pair.Pair, 12)
	state := make(map[string]SyncState, 12)
	symbols := []string{
		"BTCEUR", "ETHUSD", "XRPUSD", "ADAEUR",
		"SOLUSD", "DOTUSD", "LINKEUR", "AVAXUSD",
		"MATICUSD", "UNIEUR", "AAVEUSD", "ATOMUSD",
	}
	for i, sym := range symbols {
		allPairs[i] = pair.Pair{Symbol: sym}
		state[sym] = SyncState{HourlyBackfillCompleted: false}
	}
	budget := 9
	emptyCoverage := make(map[string]map[string]bool)

	// --- Tick 1: no active pairs, fresh start ---
	tick1Tasks := scheduler.BuildInitialBackfillTasksLimited(allPairs, state, emptyCoverage, nil, now, budget)

	if len(tick1Tasks) != budget {
		t.Fatalf("tick 1: expected %d tasks, got %d", budget, len(tick1Tasks))
	}

	// Round-robin should spread across 9 pairs (1 task each with budget=9, 12 pairs).
	tick1Pairs := make(map[string]bool)
	for _, item := range tick1Tasks {
		tick1Pairs[item.Pair] = true
	}
	if len(tick1Pairs) != 9 {
		t.Fatalf("tick 1: expected 9 distinct pairs, got %d: %v", len(tick1Pairs), tick1Pairs)
	}

	// --- Tick 2: all 9 pairs from tick 1 have active (pending) tasks ---
	// Simulate: those 9 tasks failed and went back to pending with retry_at.
	// The planner queries ActiveBackfillPairs and gets those 9 back.
	tick2Tasks := scheduler.BuildInitialBackfillTasksLimited(allPairs, state, emptyCoverage, tick1Pairs, now, budget)

	// Only the 3 remaining pairs should receive tasks.
	tick2Pairs := make(map[string]bool)
	for _, item := range tick2Tasks {
		tick2Pairs[item.Pair] = true
		if tick1Pairs[item.Pair] {
			t.Fatalf("tick 2: pair %s had active tasks but still received new work", item.Pair)
		}
	}

	// Exactly 3 pairs should get work, and they should consume the full budget
	// (each gets budget/3 = 3 tasks).
	remainingPairs := make([]string, 0)
	for _, sym := range symbols {
		if !tick1Pairs[sym] {
			remainingPairs = append(remainingPairs, sym)
		}
	}
	if len(tick2Pairs) != len(remainingPairs) {
		t.Fatalf("tick 2: expected %d pairs to get work, got %d: %v",
			len(remainingPairs), len(tick2Pairs), tick2Pairs)
	}
	for _, sym := range remainingPairs {
		if !tick2Pairs[sym] {
			t.Fatalf("tick 2: expected pair %s to receive tasks", sym)
		}
	}
	if len(tick2Tasks) != budget {
		t.Fatalf("tick 2: expected full budget %d used by remaining pairs, got %d", budget, len(tick2Tasks))
	}
}

// TestFailedTasksBudgetRedistributesEvenly verifies that when some pairs
// have active tasks, the remaining pairs share the budget fairly via
// round-robin rather than the first remaining pair hogging everything.
func TestFailedTasksBudgetRedistributesEvenly(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
		{Symbol: "XRPUSD"},
		{Symbol: "ADAEUR"},
		{Symbol: "SOLUSD"},
		{Symbol: "DOTUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
		"XRPUSD": {HourlyBackfillCompleted: false},
		"ADAEUR": {HourlyBackfillCompleted: false},
		"SOLUSD": {HourlyBackfillCompleted: false},
		"DOTUSD": {HourlyBackfillCompleted: false},
	}

	// 4 out of 6 pairs have active tasks from a previous failed batch.
	activePairs := map[string]bool{
		"BTCEUR": true,
		"ETHUSD": true,
		"XRPUSD": true,
		"ADAEUR": true,
	}

	// Budget of 6 should be split evenly between SOLUSD and DOTUSD (3 each).
	tasks := scheduler.BuildInitialBackfillTasksLimited(
		pairs, state, make(map[string]map[string]bool), activePairs, now, 6,
	)

	if len(tasks) != 6 {
		t.Fatalf("expected 6 tasks, got %d", len(tasks))
	}

	counts := map[string]int{}
	for _, item := range tasks {
		counts[item.Pair]++
	}

	if counts["SOLUSD"] != 3 || counts["DOTUSD"] != 3 {
		t.Fatalf("expected 3 tasks each for SOLUSD and DOTUSD, got %v", counts)
	}
	if counts["BTCEUR"] != 0 || counts["ETHUSD"] != 0 || counts["XRPUSD"] != 0 || counts["ADAEUR"] != 0 {
		t.Fatalf("active pairs should not receive tasks, got %v", counts)
	}
}

// TestAllPairsFailedNoNewWork verifies that when every pair already has
// active backfill tasks (all failed and pending retry), the scheduler
// generates zero new tasks instead of re-flooding the queue.
func TestAllPairsFailedNoNewWork(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
		{Symbol: "XRPUSD"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
		"XRPUSD": {HourlyBackfillCompleted: false},
	}

	// Every pair has active tasks — nothing should be generated.
	activePairs := map[string]bool{
		"BTCEUR": true,
		"ETHUSD": true,
		"XRPUSD": true,
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(
		pairs, state, make(map[string]map[string]bool), activePairs, now, 9,
	)

	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks when all pairs have active work, got %d", len(tasks))
	}
}

// TestRecoveryAfterFailedPairsRetryExpires simulates the full 3-tick
// lifecycle: initial distribution → failure blocks re-enqueue → retry
// window expires and pairs become eligible again.
func TestRecoveryAfterFailedPairsRetryExpires(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
		{Symbol: "XRPUSD"},
		{Symbol: "ADAEUR"},
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: false},
		"XRPUSD": {HourlyBackfillCompleted: false},
		"ADAEUR": {HourlyBackfillCompleted: false},
	}
	emptyCoverage := make(map[string]map[string]bool)

	// --- Tick 1: fresh start, budget=3 ---
	tick1 := scheduler.BuildInitialBackfillTasksLimited(pairs, state, emptyCoverage, nil, now, 3)
	if len(tick1) != 3 {
		t.Fatalf("tick 1: expected 3 tasks, got %d", len(tick1))
	}
	tick1Active := make(map[string]bool)
	for _, item := range tick1 {
		tick1Active[item.Pair] = true
	}
	if len(tick1Active) != 3 {
		t.Fatalf("tick 1: expected 3 distinct pairs, got %d", len(tick1Active))
	}

	// --- Tick 2: those 3 pairs are active (failed, pending retry) ---
	tick2 := scheduler.BuildInitialBackfillTasksLimited(pairs, state, emptyCoverage, tick1Active, now, 3)
	if len(tick2) != 3 {
		t.Fatalf("tick 2: expected 3 tasks for remaining pair, got %d", len(tick2))
	}
	for _, item := range tick2 {
		if tick1Active[item.Pair] {
			t.Fatalf("tick 2: pair %s should be skipped (active), but got task %s", item.Pair, item.ID)
		}
	}

	// --- Tick 3: retry window expired, all pairs eligible again ---
	// (ActiveBackfillPairs returns empty because retries were picked up or exhausted)
	tick3 := scheduler.BuildInitialBackfillTasksLimited(pairs, state, emptyCoverage, nil, now, 4)
	if len(tick3) != 4 {
		t.Fatalf("tick 3: expected 4 tasks after recovery, got %d", len(tick3))
	}
	tick3Pairs := make(map[string]bool)
	for _, item := range tick3 {
		tick3Pairs[item.Pair] = true
	}
	if len(tick3Pairs) != 4 {
		t.Fatalf("tick 3: expected all 4 pairs to get work, got %d: %v", len(tick3Pairs), tick3Pairs)
	}
}

// TestMixedActiveAndCompletedPairsBudgetGoesToNeedy verifies that the budget
// correctly flows to pairs that need work when some pairs are active (failed)
// and others are fully completed.
func TestMixedActiveAndCompletedPairsBudgetGoesToNeedy(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{
		{Symbol: "BTCEUR"}, // active (failed, pending retry)
		{Symbol: "ETHUSD"}, // fully completed
		{Symbol: "XRPUSD"}, // needs work
		{Symbol: "ADAEUR"}, // needs work
		{Symbol: "SOLUSD"}, // active (failed, pending retry)
	}
	state := map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
		"ETHUSD": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true},
		"XRPUSD": {HourlyBackfillCompleted: false},
		"ADAEUR": {HourlyBackfillCompleted: false},
		"SOLUSD": {HourlyBackfillCompleted: false},
	}
	activePairs := map[string]bool{
		"BTCEUR": true,
		"SOLUSD": true,
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited(
		pairs, state, make(map[string]map[string]bool), activePairs, now, 6,
	)

	if len(tasks) != 6 {
		t.Fatalf("expected 6 tasks, got %d", len(tasks))
	}

	counts := map[string]int{}
	for _, item := range tasks {
		counts[item.Pair]++
	}

	// BTCEUR: skipped (active). ETHUSD: skipped (completed). SOLUSD: skipped (active).
	// Only XRPUSD and ADAEUR should get work: 3 each.
	if counts["BTCEUR"] != 0 {
		t.Fatalf("BTCEUR should be skipped (active), got %d tasks", counts["BTCEUR"])
	}
	if counts["ETHUSD"] != 0 {
		t.Fatalf("ETHUSD should be skipped (completed), got %d tasks", counts["ETHUSD"])
	}
	if counts["SOLUSD"] != 0 {
		t.Fatalf("SOLUSD should be skipped (active), got %d tasks", counts["SOLUSD"])
	}
	if counts["XRPUSD"] != 3 {
		t.Fatalf("expected 3 tasks for XRPUSD, got %d", counts["XRPUSD"])
	}
	if counts["ADAEUR"] != 3 {
		t.Fatalf("expected 3 tasks for ADAEUR, got %d", counts["ADAEUR"])
	}
}

// ---------------------------------------------------------------------------
// Gap validation sweep tests
// ---------------------------------------------------------------------------

// TestGapValidationSweepUsesStablePerPairID verifies that gap validation
// tasks use a stable per-pair sweep ID so only one can be pending at a time.
func TestGapValidationSweepUsesStablePerPairID(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	state := map[string]SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	t1 := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 5, 18, 0, 0, 0, time.UTC)

	tasks1 := scheduler.BuildGapValidationTasks(pairs, state, make(map[string]map[string]bool), t1)
	tasks2 := scheduler.BuildGapValidationTasks(pairs, state, make(map[string]map[string]bool), t2)

	if len(tasks1) != 1 || len(tasks2) != 1 {
		t.Fatalf("expected 1 task each, got %d and %d", len(tasks1), len(tasks2))
	}
	if tasks1[0].ID != tasks2[0].ID {
		t.Fatalf("expected stable ID across ticks, got %q vs %q", tasks1[0].ID, tasks2[0].ID)
	}
	if tasks1[0].ID != "gap_validation:BTCEUR:sweep" {
		t.Fatalf("expected sweep ID, got %q", tasks1[0].ID)
	}
}

// TestGapValidationSkipsFullyCoveredPairs verifies that pairs with all days
// already marked complete in data_coverage don't get new gap validation tasks.
func TestGapValidationSkipsFullyCoveredPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	state := map[string]SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
	}

	// All days between synced (Apr 3) and yesterday (Apr 4) are covered.
	coverage := map[string]map[string]bool{
		"BTCEUR": {
			"2026-04-03": true,
			"2026-04-04": true,
		},
	}

	tasks := scheduler.BuildGapValidationTasks(pairs, state, coverage, now)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 gap validation tasks for fully covered pair, got %d", len(tasks))
	}
}

// TestGapValidationSkipsPairsWithNoSyncHistory verifies that pairs without
// any sync history don't get gap validation tasks.
func TestGapValidationSkipsPairsWithNoSyncHistory(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	tasks := scheduler.BuildGapValidationTasks(pairs, map[string]SyncState{
		"BTCEUR": {},
	}, make(map[string]map[string]bool), now)

	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks for pair with no sync history, got %d", len(tasks))
	}
}

// TestGapValidationEmitsForMultiplePairs verifies that each pair with gaps
// gets its own independent sweep task.
func TestGapValidationEmitsForMultiplePairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	pairs := []pair.Pair{{Symbol: "BTCEUR"}, {Symbol: "ETHUSD"}}
	state := map[string]SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
		"ETHUSD": {HourlyLastSynced: synced},
	}

	tasks := scheduler.BuildGapValidationTasks(pairs, state, make(map[string]map[string]bool), now)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 gap validation tasks, got %d", len(tasks))
	}

	ids := map[string]bool{}
	for _, item := range tasks {
		ids[item.ID] = true
	}
	if !ids["gap_validation:BTCEUR:sweep"] || !ids["gap_validation:ETHUSD:sweep"] {
		t.Fatalf("expected sweep IDs for both pairs, got %v", ids)
	}
}

// TestGapValidationWindowSpansFullRange verifies that the sweep task window
// covers from the oldest synced point to yesterday.
func TestGapValidationWindowSpansFullRange(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildGapValidationTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{"BTCEUR": {HourlyLastSynced: synced}},
		make(map[string]map[string]bool),
		now,
	)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if !tasks[0].WindowStart.Equal(synced) {
		t.Fatalf("expected WindowStart %s, got %s", synced, tasks[0].WindowStart)
	}
	expectedEnd := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	if !tasks[0].WindowEnd.Equal(expectedEnd) {
		t.Fatalf("expected WindowEnd %s, got %s", expectedEnd, tasks[0].WindowEnd)
	}
}

// ---------------------------------------------------------------------------
// Integrity check sweep tests
// ---------------------------------------------------------------------------

// TestIntegrityCheckEmitsForRealtimeCoveredPairs verifies that pairs with
// realtime started (but not hourly backfill complete) still get integrity tasks.
func TestIntegrityCheckEmitsForRealtimeCoveredPairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildIntegrityCheckTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{
			"BTCEUR": {
				HourlyBackfillCompleted: false,
				HourlyRealtimeStartedAt: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
				HourlyLastSynced:        synced,
			},
		},
		make(map[string]map[string]bool),
		now,
	)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 integrity task for realtime-covered pair, got %d", len(tasks))
	}
}

// TestIntegrityCheckSkipsPairsWithNoCoverage verifies that pairs without
// backfill or realtime coverage don't get integrity tasks.
func TestIntegrityCheckSkipsPairsWithNoCoverage(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildIntegrityCheckTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}},
		map[string]SyncState{
			"BTCEUR": {HourlyBackfillCompleted: false},
		},
		make(map[string]map[string]bool),
		now,
	)

	if len(tasks) != 0 {
		t.Fatalf("expected 0 integrity tasks for uncovered pair, got %d", len(tasks))
	}
}

// TestIntegrityCheckEmitsForMultiplePairs verifies independent sweep tasks
// per pair.
func TestIntegrityCheckEmitsForMultiplePairs(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	synced := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildIntegrityCheckTasks(
		[]pair.Pair{{Symbol: "BTCEUR"}, {Symbol: "ETHUSD"}},
		map[string]SyncState{
			"BTCEUR": {HourlyBackfillCompleted: true, HourlyLastSynced: synced},
			"ETHUSD": {HourlyBackfillCompleted: true, HourlyLastSynced: synced},
		},
		make(map[string]map[string]bool),
		now,
	)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 integrity tasks, got %d", len(tasks))
	}
	ids := map[string]bool{}
	for _, item := range tasks {
		ids[item.ID] = true
	}
	if !ids["integrity_check:BTCEUR:sweep"] || !ids["integrity_check:ETHUSD:sweep"] {
		t.Fatalf("expected sweep IDs for both pairs, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// News fetch per-source tests
// ---------------------------------------------------------------------------

// TestNewsFetchTasksMatchSourceList verifies that the scheduler emits one
// task per known RSS source.
func TestNewsFetchTasksMatchSourceList(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildNewsFetchTasks(now)

	if len(tasks) != len(NewsSources) {
		t.Fatalf("expected %d tasks, got %d", len(NewsSources), len(tasks))
	}

	sources := map[string]bool{}
	for _, item := range tasks {
		sources[item.Pair] = true
	}
	for _, src := range NewsSources {
		if !sources[src] {
			t.Fatalf("expected task for source %q, not found", src)
		}
	}
}

// TestNewsFetchTasksHaveCorrectType verifies all news tasks have the right type.
func TestNewsFetchTasksHaveCorrectType(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	for _, item := range scheduler.BuildNewsFetchTasks(now) {
		if item.Type != task.TypeNewsFetch {
			t.Fatalf("expected type %q, got %q", task.TypeNewsFetch, item.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Realtime-only emission tests (no integrity in realtime batch)
// ---------------------------------------------------------------------------

// TestRealtimeTasksDoNotEmitIntegrityChecks verifies that BuildRealtimeTasks
// no longer emits integrity_check tasks (they are now separate sweep tasks).
func TestRealtimeTasksDoNotEmitIntegrityChecks(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)

	tasks := scheduler.BuildRealtimeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSD"},
	}, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true},
		"ETHUSD": {HourlyBackfillCompleted: true, DailyBackfillCompleted: true},
	}, now)

	for _, item := range tasks {
		if item.Type == task.TypeDataSanity {
			t.Fatalf("did not expect integrity_check in realtime batch, got %+v", item)
		}
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 realtime tasks (one per pair), got %d", len(tasks))
	}
}
