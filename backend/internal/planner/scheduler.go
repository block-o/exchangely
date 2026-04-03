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

// Scheduler converts per-pair sync state into independent backfill and realtime tasks.
// The realtimePollInterval controls how frequently fresh realtime polling tasks are
// emitted for caught-up pairs. A shorter interval yields fresher ticker prices.
type Scheduler struct {
	backfillWindow1H     time.Duration
	backfillWindow1D     time.Duration
	realtimePollInterval time.Duration
}

// NewScheduler returns the planner scheduler tuned for coarse-grained backfill windows
// and the given realtime poll cadence. The pollInterval determines how often the planner
// generates a fresh realtime task per pair — e.g. 2m means prices update every ~2 minutes.
func NewScheduler(pollInterval time.Duration) *Scheduler {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Minute
	}
	return &Scheduler{
		backfillWindow1H:     24 * time.Hour,
		backfillWindow1D:     7 * 24 * time.Hour,
		realtimePollInterval: pollInterval,
	}
}

// BuildInitialBackfillTasks emits hourly work first and only advances to daily backfill after hourly catch-up.
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

// BuildRealtimeTasks emits one polling task per fully caught-up pair for the current
// poll window. The task ID includes the poll-window start timestamp (truncated to
// realtimePollInterval), so the planner's idempotent Enqueue recognizes each window
// as a distinct task. This ensures prices refresh every pollInterval instead of once per hour.
//
// WindowStart/WindowEnd still span the full current hour so the exchange API returns
// enough context for consolidation, but the unique ID prevents de-duplication starvation.
func (s *Scheduler) BuildRealtimeTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	nextHour := currentHour.Add(time.Hour)
	pollWindow := now.UTC().Truncate(s.realtimePollInterval)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		// We only require hourly backfill to be caught up so we don't clobber historical writes.
		// Daily backfill can continue in the background while realtime dashboard data flows.
		if !ok || !pairState.HourlyBackfillCompleted {
			continue
		}

		// Use pollWindow in the ID so each poll interval generates a unique task.
		// The actual fetch window remains the full current hour for API compatibility.
		tasks = append(tasks, task.Task{
			ID:          taskID(task.TypeRealtime, trackedPair.Symbol, "1h", pollWindow, nextHour),
			Type:        task.TypeRealtime,
			Pair:        trackedPair.Symbol,
			Interval:    "1h",
			WindowStart: currentHour,
			WindowEnd:   nextHour,
		})
	}

	return tasks
}

// windowedTasks splits a sync gap into fixed-size task windows so workers can claim them independently.
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
