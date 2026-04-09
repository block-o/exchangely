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
// generates a fresh realtime task per pair — e.g. 5s means prices update every ~5 seconds.
func NewScheduler(pollInterval, newsInterval time.Duration) *Scheduler {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
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

// backfillCursor tracks per-pair iteration state for round-robin backfill scheduling.
type backfillCursor struct {
	pair     pair.Pair
	cursor   time.Time
	interval string
	window   time.Duration
	coverage map[string]bool
	done     bool
}

// BuildInitialBackfillTasksLimited walks backwards from yesterday toward the past
// and stops once the requested task count limit is reached. A non-positive limit
// means unlimited. Without a fixed backfill start date, the system walks back
// indefinitely until providers stop returning data. The per-tick limit prevents
// task flooding.
//
// Tasks are distributed round-robin across pairs so that a single failing pair
// cannot monopolise the entire backfill budget. Pairs that already have pending
// or running backfill tasks (listed in activePairs) are skipped so the budget
// goes to pairs that actually need new work.
func (s *Scheduler) BuildInitialBackfillTasksLimited(pairs []pair.Pair, state map[string]SyncState, coverage map[string]map[string]bool, activePairs map[string]bool, now time.Time, limit int) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	currentDay := currentHour.Truncate(24 * time.Hour)
	yesterday := currentDay.Add(-24 * time.Hour)

	// Build a cursor per pair that still needs backfill work.
	// Skip pairs that already have pending/running backfill tasks so the
	// budget goes to pairs that actually need new work.
	cursors := make([]backfillCursor, 0, len(pairs))
	for _, trackedPair := range pairs {
		if activePairs[trackedPair.Symbol] {
			continue
		}

		pairState := state[trackedPair.Symbol]
		pairCoverage := coverage[trackedPair.Symbol]

		if !pairState.HourlyBackfillCompleted {
			hourlyCeiling := yesterday
			if !pairState.HourlyRealtimeStartedAt.IsZero() && pairState.HourlyRealtimeStartedAt.UTC().Before(hourlyCeiling) {
				hourlyCeiling = pairState.HourlyRealtimeStartedAt.UTC().Truncate(time.Hour)
			}
			cursor := pairState.HourlyLastSynced.UTC()
			if cursor.IsZero() || cursor.After(hourlyCeiling) {
				cursor = hourlyCeiling
			}
			cursors = append(cursors, backfillCursor{
				pair:     trackedPair,
				cursor:   cursor,
				interval: "1h",
				window:   s.backfillWindow1H,
				coverage: pairCoverage,
			})
			continue
		}

		if !pairState.DailyBackfillCompleted {
			dailyCursor := pairState.DailyLastSynced.UTC()
			if dailyCursor.IsZero() || dailyCursor.After(yesterday) {
				dailyCursor = yesterday
			}
			cursors = append(cursors, backfillCursor{
				pair:     trackedPair,
				cursor:   dailyCursor,
				interval: "1d",
				window:   s.backfillWindow1D,
				coverage: pairCoverage,
			})
		}
	}

	tasks := make([]task.Task, 0)
	active := len(cursors)

	// Round-robin: emit one task per pair per round until the budget is full
	// or every cursor is exhausted.
	for active > 0 {
		for i := range cursors {
			c := &cursors[i]
			if c.done {
				continue
			}

			emitted := false
			for !emitted {
				windowStart := c.cursor.Add(-c.window)
				if !c.cursor.After(windowStart) {
					c.done = true
					active--
					break
				}

				dayKey := windowStart.UTC().Format("2006-01-02")
				prev := c.cursor
				c.cursor = windowStart

				if c.coverage[dayKey] {
					continue
				}

				t := task.Task{
					ID:          taskID(task.TypeBackfill, c.pair.Symbol, c.interval, windowStart, prev),
					Type:        task.TypeBackfill,
					Pair:        c.pair.Symbol,
					Interval:    c.interval,
					WindowStart: windowStart,
					WindowEnd:   prev,
				}
				t.Description = task.BuildDescription(t)
				tasks = append(tasks, t)
				emitted = true

				if limit > 0 && len(tasks) >= limit {
					return tasks
				}
			}
		}
	}

	return tasks
}

// BuildGapValidationTasks emits tasks to verify coverage for days between the oldest
// synced point and yesterday that are not yet marked as complete.
func (s *Scheduler) BuildGapValidationTasks(pairs []pair.Pair, state map[string]SyncState, coverage map[string]map[string]bool, now time.Time) []task.Task {
	yesterday := now.UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		if !ok {
			continue
		}
		pairCoverage := coverage[trackedPair.Symbol]

		// Use the oldest synced timestamp as the start of the gap scan.
		start := pairState.HourlyLastSynced.UTC().Truncate(24 * time.Hour)
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
			t := task.Task{
				ID:          taskID(task.TypeGapValidation, trackedPair.Symbol, "1d", cursor, cursor.Add(24*time.Hour)),
				Type:        task.TypeGapValidation,
				Pair:        trackedPair.Symbol,
				Interval:    "1d",
				WindowStart: cursor,
				WindowEnd:   cursor.Add(24 * time.Hour),
			}
			t.Description = task.BuildDescription(t)
			tasks = append(tasks, t)

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
	t := task.Task{
		ID:          fmt.Sprintf("%s:daily:%d", task.TypeCleanup, dayStart.Unix()),
		Type:        task.TypeCleanup,
		Pair:        "*", // global
		Interval:    "1d",
		WindowStart: dayStart,
		WindowEnd:   dayEnd,
	}
	t.Description = task.BuildDescription(t)
	return t
}

// BuildNewsFetchTask emits a task_news_fetch task based on the configured interval.
func (s *Scheduler) BuildNewsFetchTask(now time.Time) task.Task {
	windowStart := now.UTC().Truncate(s.newsFetchInterval)
	windowEnd := windowStart.Add(s.newsFetchInterval)
	t := task.Task{
		ID:          fmt.Sprintf("%s:periodic:%d", task.TypeNewsFetch, windowStart.Unix()),
		Type:        task.TypeNewsFetch,
		Pair:        "*", // global
		Interval:    s.newsFetchInterval.String(),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}
	t.Description = task.BuildDescription(t)
	return t
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

		t := task.Task{
			ID:          taskID(task.TypeConsolidate, trackedPair.Symbol, "1d", prevDay, currentDay),
			Type:        task.TypeConsolidate,
			Pair:        trackedPair.Symbol,
			Interval:    "1d",
			WindowStart: prevDay,
			WindowEnd:   currentDay,
		}
		t.Description = task.BuildDescription(t)
		tasks = append(tasks, t)
	}

	return tasks
}

// BuildRealtimeTasks emits one polling task per pair. The task ID is stable per
// pair (no poll-window timestamp) so the planner's idempotent Enqueue naturally
// prevents a second task from being queued while one is still pending or running.
// Once a worker completes the task, the next planner tick will enqueue a fresh one.
//
// WindowStart/WindowEnd span the full current hour so the exchange API returns
// enough context for consolidation.
// Integrity checks only start once the preceding hour is expected to be covered either
// by completed backfill or by the live cutover.
func (s *Scheduler) BuildRealtimeTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	currentHour := now.UTC().Truncate(time.Hour)
	nextHour := currentHour.Add(time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState, ok := state[trackedPair.Symbol]
		if !ok {
			continue
		}

		// Stable ID per pair — only one realtime task per pair can exist at a time.
		tasks = append(tasks, task.Task{
			ID:          fmt.Sprintf("%s:%s:realtime", task.TypeRealtime, trackedPair.Symbol),
			Type:        task.TypeRealtime,
			Pair:        trackedPair.Symbol,
			Interval:    "realtime",
			WindowStart: currentHour,
			WindowEnd:   nextHour,
		})

		prevHour := currentHour.Add(-time.Hour)
		if pairState.HourlyBackfillCompleted || (!pairState.HourlyRealtimeStartedAt.IsZero() && !prevHour.Before(pairState.HourlyRealtimeStartedAt.UTC())) {
			t := task.Task{
				ID:          taskID(task.TypeDataSanity, trackedPair.Symbol, "1h", prevHour, currentHour),
				Type:        task.TypeDataSanity,
				Pair:        trackedPair.Symbol,
				Interval:    "1h",
				WindowStart: prevHour,
				WindowEnd:   currentHour,
			}
			t.Description = task.BuildDescription(t)
			tasks = append(tasks, t)
		}
	}

	return tasks
}

// BuildBackfillProbeTasks emits one backfill task per pair per calendar day,
// targeting the hour just before the current oldest synced point. This ensures
// that even after regular backfill stops producing data, the system retries
// once a day in case new providers are added or upstream data becomes available.
// The task ID is keyed by day so it runs at most once per pair per day.
func (s *Scheduler) BuildBackfillProbeTasks(pairs []pair.Pair, state map[string]SyncState, now time.Time) []task.Task {
	dayStart := now.UTC().Truncate(24 * time.Hour)
	tasks := make([]task.Task, 0)

	for _, trackedPair := range pairs {
		pairState := state[trackedPair.Symbol]

		// Only probe if we have a cursor to extend from.
		cursor := pairState.HourlyLastSynced.UTC()
		if cursor.IsZero() {
			continue
		}

		// Probe the hour just before the oldest synced point.
		probeEnd := cursor
		probeStart := probeEnd.Add(-s.backfillWindow1H)

		tasks = append(tasks, task.Task{
			ID:          fmt.Sprintf("%s:%s:probe:%d", task.TypeBackfill, trackedPair.Symbol, dayStart.Unix()),
			Type:        task.TypeBackfill,
			Pair:        trackedPair.Symbol,
			Interval:    "1h",
			WindowStart: probeStart,
			WindowEnd:   probeEnd,
			Description: fmt.Sprintf("1h probe candle %s → %s", probeStart.UTC().Format("Jan 2 2006 15:04"), probeEnd.UTC().Format("Jan 2 2006 15:04")),
		})
	}

	return tasks
}

func taskID(taskType, pairSymbol, interval string, start, end time.Time) string {
	return fmt.Sprintf("%s:%s:%s:%d:%d", taskType, pairSymbol, interval, start.UTC().Unix(), end.UTC().Unix())
}
