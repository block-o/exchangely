package planner

import (
	"fmt"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type SyncState struct {
	HourlyLastSynced        time.Time
	HourlyRealtimeStartedAt time.Time
	DailyLastSynced         time.Time
	HourlyBackfillCompleted bool
	DailyBackfillCompleted  bool
}

// Scheduler converts per-pair sync state into independent backfill and realtime tasks.
// The realtimePollInterval controls how frequently fresh realtime polling tasks are
// emitted per pair. A shorter interval yields fresher ticker prices.
type Scheduler struct {
	backfillWindow1H     time.Duration
	backfillWindow1D     time.Duration
	realtimePollInterval time.Duration
	newsFetchInterval    time.Duration
}

// NewScheduler returns the planner scheduler tuned for coarse-grained backfill windows
// and the given realtime poll cadence. The pollInterval determines how often the planner
// generates a fresh realtime task per pair — e.g. 2m means prices update every ~2 minutes.
func NewScheduler(pollInterval, newsInterval time.Duration) *Scheduler {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Minute
	}
	if newsInterval <= 0 {
		newsInterval = 5 * time.Minute
	}
	return &Scheduler{
		backfillWindow1H:     1 * time.Hour,
		backfillWindow1D:     24 * time.Hour,
		realtimePollInterval: pollInterval,
		newsFetchInterval:    newsInterval,
	}
}

// BuildInitialBackfillTasks emits hourly work first and only advances to daily backfill after hourly catch-up.
// Once live realtime starts for a pair, hourly historical work is capped at that cutover so backfill
// does not continue into the live-managed window.
func (s *Scheduler) BuildInitialBackfillTasks(pairs []pair.Pair, state map[string]SyncState, coverage map[string]map[string]bool, now time.Time) []task.Task {
	return s.BuildInitialBackfillTasksLimited(pairs, state, coverage, now, 0)
}

// BuildInitialBackfillTasksLimited behaves like BuildInitialBackfillTasks but stops
// once the requested task count limit is reached. A non-positive limit means unlimited.
func (s *Scheduler) BuildInitialBackfillTasksLimited(pairs []pair.Pair, state map[string]SyncState, coverage map[string]map[string]bool, now time.Time, limit int) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	currentDay := currentHour.Truncate(24 * time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState := state[trackedPair.Symbol]
		pairCoverage := coverage[trackedPair.Symbol]

		hourlyCursor := pairState.HourlyLastSynced.UTC()
		if hourlyCursor.IsZero() {
			hourlyCursor = trackedPair.BackfillStart.UTC()
			if hourlyCursor.IsZero() {
				hourlyCursor = currentHour.AddDate(0, 0, -30) // fallback if not set
			}
		}
		hourlyCutover := currentHour
		if !pairState.HourlyRealtimeStartedAt.IsZero() && pairState.HourlyRealtimeStartedAt.UTC().Before(hourlyCutover) {
			hourlyCutover = pairState.HourlyRealtimeStartedAt.UTC()
		}

		if !pairState.HourlyBackfillCompleted {
			for _, t := range s.windowedTasks(trackedPair.Symbol, "1h", hourlyCursor, hourlyCutover, s.backfillWindow1H) {
				dayKey := t.WindowStart.UTC().Format("2006-01-02")
				if pairCoverage[dayKey] {
					continue
				}
				tasks = append(tasks, t)
				if limit > 0 && len(tasks) >= limit {
					return tasks
				}
			}
			continue
		}

		dailyCursor := pairState.DailyLastSynced.UTC()
		if dailyCursor.IsZero() {
			dailyCursor = hourlyCursor.Truncate(24 * time.Hour)
		}
		if !pairState.DailyBackfillCompleted {
			for _, t := range s.windowedTasks(trackedPair.Symbol, "1d", dailyCursor, currentDay, s.backfillWindow1D) {
				dayKey := t.WindowStart.UTC().Format("2006-01-02")
				if pairCoverage[dayKey] {
					continue
				}
				tasks = append(tasks, t)
				if limit > 0 && len(tasks) >= limit {
					return tasks
				}
			}
		}
	}

	return tasks
}

// BuildGapValidationTasks emits tasks to verify coverage for days between backfill_start and yesterday
// that are not yet marked as complete.
func (s *Scheduler) BuildGapValidationTasks(pairs []pair.Pair, state map[string]SyncState, coverage map[string]map[string]bool, now time.Time) []task.Task {
	yesterday := now.UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairCoverage := coverage[trackedPair.Symbol]
		start := trackedPair.BackfillStart.UTC()
		if start.IsZero() {
			continue
		}

		// Only check up to yesterday
		for cursor := start; !cursor.After(yesterday); cursor = cursor.Add(24 * time.Hour) {
			dayKey := cursor.Format("2006-01-02")
			if pairCoverage[dayKey] {
				continue
			}

			// Add gap validation task for this day
			tasks = append(tasks, task.Task{
				ID:          taskID(task.TypeGapValidation, trackedPair.Symbol, "1d", cursor, cursor.Add(24*time.Hour)),
				Type:        task.TypeGapValidation,
				Pair:        trackedPair.Symbol,
				Interval:    "1d",
				WindowStart: cursor,
				WindowEnd:   cursor.Add(24 * time.Hour),
			})

			// Limit the number of validation tasks per tick to avoid flooding (e.g. max 5 per pair)
			if len(tasks) > 50 { // total across all pairs
				return tasks
			}
		}
	}

	return tasks
}

// BuildCleanupTask emits a single task_cleanup task per calendar day (keyed by midnight UTC).
// The planner's idempotent Enqueue ensures it only executes once per day regardless of restart count.
func (s *Scheduler) BuildCleanupTask(now time.Time) task.Task {
	dayStart := now.UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	return task.Task{
		ID:          fmt.Sprintf("%s:daily:%d", task.TypeCleanup, dayStart.Unix()),
		Type:        task.TypeCleanup,
		Pair:        "*", // global
		Interval:    "1d",
		WindowStart: dayStart,
		WindowEnd:   dayEnd,
	}
}

// BuildNewsFetchTask emits a task_news_fetch task based on the configured interval.
func (s *Scheduler) BuildNewsFetchTask(now time.Time) task.Task {
	windowStart := now.UTC().Truncate(s.newsFetchInterval)
	windowEnd := windowStart.Add(s.newsFetchInterval)
	return task.Task{
		ID:          fmt.Sprintf("%s:periodic:%d", task.TypeNewsFetch, windowStart.Unix()),
		Type:        task.TypeNewsFetch,
		Pair:        "*", // global
		Interval:    s.newsFetchInterval.String(),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}
}

// BuildConsolidationTasks emits one daily consolidation task per pair for the last fully-closed UTC day.
// These tasks rebuild the previous daily candle from canonical hourly candles after backfill catch-up.
func (s *Scheduler) BuildConsolidationTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentDay := now.UTC().Truncate(24 * time.Hour)
	prevDay := currentDay.Add(-24 * time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		if !ok || !pairState.HourlyBackfillCompleted || !pairState.DailyBackfillCompleted {
			continue
		}

		tasks = append(tasks, task.Task{
			ID:          taskID(task.TypeConsolidate, trackedPair.Symbol, "1d", prevDay, currentDay),
			Type:        task.TypeConsolidate,
			Pair:        trackedPair.Symbol,
			Interval:    "1d",
			WindowStart: prevDay,
			WindowEnd:   currentDay,
		})
	}

	return tasks
}

// BuildRealtimeTasks emits one polling task per pair for the current poll window, even if
// historical backfill is still in progress. The task ID includes the poll-window start timestamp (truncated to
// realtimePollInterval), so the planner's idempotent Enqueue recognizes each window
// as a distinct task. This ensures prices refresh every pollInterval instead of once per hour.
//
// WindowStart/WindowEnd still span the full current hour so the exchange API returns
// enough context for consolidation, but the unique ID prevents de-duplication starvation.
// Integrity checks only start once the preceding hour is expected to be covered either
// by completed backfill or by the live cutover.
func (s *Scheduler) BuildRealtimeTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	nextHour := currentHour.Add(time.Hour)
	pollWindow := now.UTC().Truncate(s.realtimePollInterval)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		if !ok {
			continue
		}

		// Realtime extraction
		tasks = append(tasks, task.Task{
			ID:          taskID(task.TypeRealtime, trackedPair.Symbol, "realtime", pollWindow, nextHour),
			Type:        task.TypeRealtime,
			Pair:        trackedPair.Symbol,
			Interval:    "realtime",
			WindowStart: currentHour,
			WindowEnd:   nextHour,
		})

		prevHour := currentHour.Add(-time.Hour)
		if pairState.HourlyBackfillCompleted || (!pairState.HourlyRealtimeStartedAt.IsZero() && !prevHour.Before(pairState.HourlyRealtimeStartedAt.UTC())) {
			// Integrity checks only run once the preceding hour is expected to be covered either
			// by completed backfill or by the live cutover.
			tasks = append(tasks, task.Task{
				ID:          taskID(task.TypeDataSanity, trackedPair.Symbol, "1h", prevHour, currentHour),
				Type:        task.TypeDataSanity,
				Pair:        trackedPair.Symbol,
				Interval:    "1h",
				WindowStart: prevHour,
				WindowEnd:   currentHour,
			})
		}
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
