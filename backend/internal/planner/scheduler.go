package planner

import (
	"fmt"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type SyncState struct {
	LastSynced        time.Time
	BackfillCompleted bool
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
		cursor := state[trackedPair.Symbol].LastSynced.UTC()
		if cursor.IsZero() {
			cursor = currentHour.AddDate(0, 0, -30)
		}

		tasks = append(tasks, s.windowedTasks(trackedPair.Symbol, "1h", cursor, currentHour, s.backfillWindow1H)...)
		tasks = append(tasks, s.windowedTasks(trackedPair.Symbol, "1d", cursor.Truncate(24*time.Hour), currentDay, s.backfillWindow1D)...)
	}

	return tasks
}

func (s *Scheduler) BuildRealtimeTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	nextHour := currentHour.Add(time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		if !ok || !pairState.BackfillCompleted {
			continue
		}

		tasks = append(tasks, task.Task{
			ID:          fmt.Sprintf("%s:%s:%d:%d", task.TypeRealtime, trackedPair.Symbol, currentHour.Unix(), nextHour.Unix()),
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
			ID:          fmt.Sprintf("%s:%s:%d:%d", task.TypeBackfill, pairSymbol, cursor.Unix(), windowEnd.Unix()),
			Type:        task.TypeBackfill,
			Pair:        pairSymbol,
			Interval:    interval,
			WindowStart: cursor,
			WindowEnd:   windowEnd,
		})
	}

	return tasks
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
