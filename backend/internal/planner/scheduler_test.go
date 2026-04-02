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
		"BTCEUR": {LastSynced: now.Add(-48 * time.Hour)},
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
	}

	if !seenPairIntervals["BTCEUR:1h"] || !seenPairIntervals["BTCEUR:1d"] {
		t.Fatal("expected BTCEUR hourly and daily tasks")
	}
	if !seenPairIntervals["ETHUSDT:1h"] || !seenPairIntervals["ETHUSDT:1d"] {
		t.Fatal("expected ETHUSDT hourly and daily tasks")
	}
}

func TestBuildRealtimeTasksIncludesOnlyCaughtUpPairs(t *testing.T) {
	scheduler := NewScheduler()
	now := time.Date(2026, 4, 2, 12, 34, 0, 0, time.UTC)

	tasks := scheduler.BuildRealtimeTasks([]pair.Pair{
		{Symbol: "BTCEUR"},
		{Symbol: "ETHUSDT"},
	}, map[string]SyncState{
		"BTCEUR":  {LastSynced: now, BackfillCompleted: true},
		"ETHUSDT": {LastSynced: now, BackfillCompleted: false},
	}, now)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 realtime task, got %d", len(tasks))
	}
	if tasks[0].Type != "realtime_poll" || tasks[0].Pair != "BTCEUR" || tasks[0].Interval != "1h" {
		t.Fatalf("unexpected realtime task: %+v", tasks[0])
	}
}
