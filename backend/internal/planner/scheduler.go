package planner

import (
	"fmt"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type SyncState struct {
	HourlyLastSynced        time.Time
	DailyLastSynced         time.Time
	HourlyBackfillCompleted bool
	DailyBackfillCompleted  bool
}

type Scheduler struct {
	backfillWindow1H time.Duration
	backfillWindow1D time.Duration
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		backfillWindow1H: 24 * time.Hour,
		backfillWindow1D: 7 * 24 * time.Hour,
	}
}

func (s *Scheduler) BuildInitialBackfillTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	currentDay := currentHour.Truncate(24 * time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState := state[trackedPair.Symbol]
		hourlyCursor := pairState.HourlyLastSynced.UTC()
		if hourlyCursor.IsZero() {
			hourlyCursor = currentHour.AddDate(0, 0, -30)
		}

		if !pairState.HourlyBackfillCompleted {
			tasks = append(tasks, s.windowedTasks(trackedPair.Symbol, "1h", hourlyCursor, currentHour, s.backfillWindow1H)...)
			continue
		}

		dailyCursor := pairState.DailyLastSynced.UTC()
		if dailyCursor.IsZero() {
			dailyCursor = hourlyCursor.Truncate(24 * time.Hour)
		}
		if !pairState.DailyBackfillCompleted {
			tasks = append(tasks, s.windowedTasks(trackedPair.Symbol, "1d", dailyCursor, currentDay, s.backfillWindow1D)...)
		}
	}

	return tasks
}

func (s *Scheduler) BuildRealtimeTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	nextHour := currentHour.Add(time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		if !ok || !pairState.HourlyBackfillCompleted || !pairState.DailyBackfillCompleted {
			continue
		}

		tasks = append(tasks, task.Task{
			ID:          taskID(task.TypeRealtime, trackedPair.Symbol, "1h", currentHour, nextHour),
			Type:        task.TypeRealtime,
			Pair:        trackedPair.Symbol,
			Interval:    "1h",
			WindowStart: currentHour,
			WindowEnd:   nextHour,
		})
	}

	return tasks
}

func (s *Scheduler) windowedTasks(pairSymbol, interval string, start, end time.Time, size time.Duration) []task.Task {
	tasks := make([]task.Task, 0)
	for cursor := start; cursor.Before(end); cursor = minTime(cursor.Add(size), end) {
		windowEnd := minTime(cursor.Add(size), end)
		if !windowEnd.After(cursor) {
			continue
		}

		tasks = append(tasks, task.Task{
			ID:          taskID(task.TypeBackfill, pairSymbol, interval, cursor, windowEnd),
			Type:        task.TypeBackfill,
			Pair:        pairSymbol,
			Interval:    interval,
			WindowStart: cursor,
			WindowEnd:   windowEnd,
		})
	}

	return tasks
}

func taskID(taskType, pairSymbol, interval string, start, end time.Time) string {
	return fmt.Sprintf("%s:%s:%s:%d:%d", taskType, pairSymbol, interval, start.UTC().Unix(), end.UTC().Unix())
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
