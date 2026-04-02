package planner

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

func TestBuildInitialBackfillTasksPartitionsByPairAndInterval(t *testing.T) {
	scheduler := NewScheduler()
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
	scheduler := NewScheduler()
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

	if len(tasks) != 1 {
		t.Fatalf("expected 1 realtime task, got %d", len(tasks))
	}
	if tasks[0].Type != "realtime_poll" || tasks[0].Pair != "BTCEUR" || tasks[0].Interval != "1h" {
		t.Fatalf("unexpected realtime task: %+v", tasks[0])
	}
}
