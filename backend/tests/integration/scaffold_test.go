package integration

import (
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/planner"
)

func TestSchedulerProducesTasksForKnownPairs(t *testing.T) {
	scheduler := planner.NewScheduler()
	tasks := scheduler.BuildInitialBackfillTasks([]pair.Pair{{Symbol: "BTCEUR"}}, nil, time.Now().UTC())
	if len(tasks) == 0 {
		t.Fatal("expected tasks to be scheduled")
	}
}
