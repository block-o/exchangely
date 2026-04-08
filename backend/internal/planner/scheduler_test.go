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
	}, make(map[string]map[string]bool), now, 10)

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
	}, make(map[string]map[string]bool), now, 5)

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
	}, make(map[string]map[string]bool), now, 2)

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

	if len(tasks1) != 2 || len(tasks2) != 2 {
		t.Fatalf("expected 2 tasks each (1 realtime, 1 sanity), got %d and %d", len(tasks1), len(tasks2))
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

// TestBuildCleanupTask verifies that the cleanup task is correctly generated
// with a unique ID per day.
func TestBuildCleanupTask(t *testing.T) {
	s := NewScheduler(5*time.Second, 5*time.Minute)
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
	}, make(map[string]map[string]bool), now, 5)

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

// TestRealtimeTaskEmitsIntegrityOnlyWhenCovered verifies the integrity check
// is only emitted when the preceding hour is expected to be covered.
func TestRealtimeTaskEmitsIntegrityOnlyWhenCovered(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	// Case 1: hourly backfill not complete, no realtime started → no integrity.
	tasks := scheduler.BuildRealtimeTasks(pairs, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: false},
	}, now)
	if findTaskByType(tasks, "integrity_check") != nil {
		t.Fatal("did not expect integrity task when backfill incomplete and no realtime")
	}

	// Case 2: hourly backfill complete → integrity emitted.
	tasks = scheduler.BuildRealtimeTasks(pairs, map[string]SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true},
	}, now)
	if findTaskByType(tasks, "integrity_check") == nil {
		t.Fatal("expected integrity task when hourly backfill complete")
	}

	// Case 3: realtime started and preceding hour is covered → integrity emitted.
	tasks = scheduler.BuildRealtimeTasks(pairs, map[string]SyncState{
		"BTCEUR": {
			HourlyBackfillCompleted: false,
			HourlyRealtimeStartedAt: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
		},
	}, now)
	if findTaskByType(tasks, "integrity_check") == nil {
		t.Fatal("expected integrity task when realtime covers preceding hour")
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
