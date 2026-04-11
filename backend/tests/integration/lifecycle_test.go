package integration

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
	"github.com/block-o/exchangely/backend/internal/planner"
	"github.com/block-o/exchangely/backend/internal/worker"
)

func TestHourlyBackfillTaskExecutesAndUpdatesProgress(t *testing.T) {
	now := time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC)
	scheduler := planner.NewScheduler(5*time.Second, 5*time.Minute)

	// Backwards backfill: cursor starts at yesterday (Jan 2 00:00) and walks backwards.
	// HourlyLastSynced at Jan 2 00:00 means we've fetched down to that point.
	// We limit to 1 task to test a single execution.
	tasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{
		{Symbol: "BTCEUR"},
	}, map[string]planner.SyncState{
		"BTCEUR": {
			// HourlyLastSynced at Jan 2 00:00 means we've fetched down to that point.
			HourlyLastSynced:        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			HourlyBackfillCompleted: false,
			DailyBackfillCompleted:  false,
		},
	}, make(map[string]map[string]bool), nil, now, 1)

	if len(tasks) != 1 {
		t.Fatalf("expected exactly one hourly task, got %d", len(tasks))
	}
	if tasks[0].Interval != "1h" {
		t.Fatalf("expected hourly task, got %+v", tasks[0])
	}

	store := &integrationCandleStore{}
	sync := &integrationSyncWriter{}
	source := &integrationMarketSource{
		items: []candle.Candle{
			{
				Pair:      "BTCEUR",
				Interval:  "1h",
				Timestamp: tasks[0].WindowStart.Unix(),
				Open:      100,
				High:      101,
				Low:       99,
				Close:     100.5,
				Volume:    12,
				Source:    "binancevision",
				Finalized: true,
			},
		},
	}
	executor := worker.NewBackfillExecutor(store, sync, source, nil)

	if err := executor.Execute(context.Background(), tasks[0]); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(store.written) != 1 {
		t.Fatalf("expected 1 consolidated candle, got %d", len(store.written))
	}
	if len(store.rawWritten) != 1 {
		t.Fatalf("expected 1 raw candle, got %d", len(store.rawWritten))
	}
	if sync.lastInterval != "1h" || sync.lastPair != "BTCEUR" {
		t.Fatalf("unexpected sync update: %+v", sync)
	}
	if !sync.lastSynced.Equal(tasks[0].WindowEnd.UTC()) {
		t.Fatalf("expected sync to advance to %s, got %s", tasks[0].WindowEnd.UTC(), sync.lastSynced)
	}
}

func TestDailyPromotionMakesPairRealtimeEligible(t *testing.T) {
	now := time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC)
	scheduler := planner.NewScheduler(5*time.Second, 5*time.Minute)

	// Backwards daily backfill: cursor starts at yesterday (Jan 2) and walks backwards.
	// DailyLastSynced at Jan 2 means we've fetched down to that point.
	// We limit to 1 task to test a single execution.
	state := map[string]planner.SyncState{
		"BTCEUR": {
			HourlyLastSynced:        time.Date(2024, 1, 2, 23, 0, 0, 0, time.UTC),
			HourlyBackfillCompleted: true,
			DailyLastSynced:         time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			DailyBackfillCompleted:  false,
		},
	}

	tasks := scheduler.BuildInitialBackfillTasksLimited([]pair.Pair{{Symbol: "BTCEUR"}}, state, make(map[string]map[string]bool), nil, now, 1)
	if len(tasks) != 1 {
		t.Fatalf("expected one daily task, got %d", len(tasks))
	}
	if tasks[0].Interval != "1d" {
		t.Fatalf("expected daily task, got %+v", tasks[0])
	}

	store := &integrationCandleStore{
		hourly: []candle.Candle{
			{
				Pair:      "BTCEUR",
				Interval:  "1h",
				Timestamp: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC).Unix(),
				Open:      100,
				High:      110,
				Low:       95,
				Close:     105,
				Volume:    5,
				Source:    "consolidated",
				Finalized: true,
			},
			{
				Pair:      "BTCEUR",
				Interval:  "1h",
				Timestamp: time.Date(2024, 1, 2, 1, 0, 0, 0, time.UTC).Unix(),
				Open:      105,
				High:      112,
				Low:       103,
				Close:     111,
				Volume:    7,
				Source:    "consolidated",
				Finalized: true,
			},
		},
	}
	sync := &integrationSyncWriter{}
	executor := worker.NewBackfillExecutor(store, sync, nil, nil)

	if err := executor.Execute(context.Background(), tasks[0]); err != nil {
		t.Fatalf("daily execute failed: %v", err)
	}

	if len(store.written) != 1 || store.written[0].Interval != "1d" {
		t.Fatalf("expected one daily candle, got %+v", store.written)
	}
	if sync.lastInterval != "1d" {
		t.Fatalf("expected daily sync update, got %+v", sync)
	}
	if !sync.lastSynced.Equal(tasks[0].WindowEnd.UTC()) {
		t.Fatalf("expected daily sync to advance to %s, got %s", tasks[0].WindowEnd.UTC(), sync.lastSynced)
	}

	state["BTCEUR"] = planner.SyncState{
		HourlyLastSynced:        state["BTCEUR"].HourlyLastSynced,
		DailyLastSynced:         sync.lastSynced,
		HourlyBackfillCompleted: true,
		DailyBackfillCompleted:  true,
	}

	afterPromotion := scheduler.BuildRealtimeTasks([]pair.Pair{{Symbol: "BTCEUR"}}, state, now)
	if len(afterPromotion) != 1 {
		t.Fatalf("expected 1 realtime task after daily promotion, got %d", len(afterPromotion))
	}
	if afterPromotion[0].Type != task.TypeRealtime || afterPromotion[0].Interval != "realtime" {
		t.Fatalf("unexpected realtime task: %+v", afterPromotion[0])
	}
}

type integrationCandleStore struct {
	rawWritten []candle.Candle
	written    []candle.Candle
	hourly     []candle.Candle
}

func (s *integrationCandleStore) UpsertCandles(_ context.Context, _ string, candles []candle.Candle) error {
	s.written = append(s.written, candles...)
	return nil
}

func (s *integrationCandleStore) UpsertRawCandles(_ context.Context, _ string, candles []candle.Candle) error {
	s.rawWritten = append(s.rawWritten, candles...)
	return nil
}

func (s *integrationCandleStore) RawCandles(_ context.Context, _ string, _ string, _ time.Time, _ time.Time) ([]candle.Candle, error) {
	return append([]candle.Candle{}, s.rawWritten...), nil
}

func (s *integrationCandleStore) HourlyCandles(_ context.Context, _ string, _ time.Time, _ time.Time) ([]candle.Candle, error) {
	return append([]candle.Candle{}, s.hourly...), nil
}

type integrationSyncWriter struct {
	lastPair          string
	lastInterval      string
	lastSynced        time.Time
	backfillCompleted bool
	realtimeStartedAt time.Time
}

func (s *integrationSyncWriter) UpsertProgress(_ context.Context, pairSymbol, interval string, lastSynced time.Time, backfillCompleted bool) error {
	s.lastPair = pairSymbol
	s.lastInterval = interval
	s.lastSynced = lastSynced.UTC()
	s.backfillCompleted = backfillCompleted
	return nil
}

func (s *integrationSyncWriter) MarkRealtimeStarted(_ context.Context, _ string, startedAt time.Time) error {
	s.realtimeStartedAt = startedAt.UTC()
	return nil
}

type integrationMarketSource struct {
	items []candle.Candle
	err   error
}

func (s *integrationMarketSource) FetchCandles(_ context.Context, _ provider.Request) ([]candle.Candle, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]candle.Candle{}, s.items...), nil
}

// TestIntegrityCheckSweepLifecycle verifies the end-to-end flow: planner
// emits a stable per-pair integrity sweep task, and the scheduler skips
// pairs that are fully verified.
func TestIntegrityCheckSweepLifecycle(t *testing.T) {
	scheduler := planner.NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2024, 1, 5, 12, 0, 0, 0, time.UTC)
	synced := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	state := map[string]planner.SyncState{
		"BTCEUR": {HourlyBackfillCompleted: true, HourlyLastSynced: synced},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	// Tick 1: no verified days — should emit a sweep task.
	tasks1 := scheduler.BuildIntegrityCheckTasks(pairs, state, make(map[string]map[string]bool), now)
	if len(tasks1) != 1 {
		t.Fatalf("tick 1: expected 1 integrity task, got %d", len(tasks1))
	}
	if tasks1[0].ID != "integrity_check:BTCEUR:sweep" {
		t.Fatalf("expected sweep ID, got %q", tasks1[0].ID)
	}

	// Tick 2: all days verified — should emit nothing.
	verified := map[string]map[string]bool{
		"BTCEUR": {
			"2024-01-02": true,
			"2024-01-03": true,
			"2024-01-04": true,
		},
	}
	tasks2 := scheduler.BuildIntegrityCheckTasks(pairs, state, verified, now)
	if len(tasks2) != 0 {
		t.Fatalf("tick 2: expected 0 tasks after full verification, got %d", len(tasks2))
	}
}

// TestGapValidationSweepLifecycle verifies the end-to-end flow: planner
// emits a stable per-pair gap sweep task, and the scheduler skips pairs
// that are fully covered.
func TestGapValidationSweepLifecycle(t *testing.T) {
	scheduler := planner.NewScheduler(5*time.Second, 5*time.Minute)
	now := time.Date(2024, 1, 5, 12, 0, 0, 0, time.UTC)
	synced := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	state := map[string]planner.SyncState{
		"BTCEUR": {HourlyLastSynced: synced},
	}
	pairs := []pair.Pair{{Symbol: "BTCEUR"}}

	// Tick 1: no coverage — should emit a sweep task.
	tasks1 := scheduler.BuildGapValidationTasks(pairs, state, make(map[string]map[string]bool), now)
	if len(tasks1) != 1 {
		t.Fatalf("tick 1: expected 1 gap validation task, got %d", len(tasks1))
	}
	if tasks1[0].ID != "gap_validation:BTCEUR:sweep" {
		t.Fatalf("expected sweep ID, got %q", tasks1[0].ID)
	}

	// Tick 2: all days covered — should emit nothing.
	coverage := map[string]map[string]bool{
		"BTCEUR": {
			"2024-01-02": true,
			"2024-01-03": true,
			"2024-01-04": true,
		},
	}
	tasks2 := scheduler.BuildGapValidationTasks(pairs, state, coverage, now)
	if len(tasks2) != 0 {
		t.Fatalf("tick 2: expected 0 tasks after full coverage, got %d", len(tasks2))
	}
}

// TestNewsFetchPerSourceLifecycle verifies that the scheduler emits one
// stable task per RSS source and that the IDs don't change across ticks.
func TestNewsFetchPerSourceLifecycle(t *testing.T) {
	scheduler := planner.NewScheduler(5*time.Second, 5*time.Minute)

	tick1 := scheduler.BuildNewsFetchTasks(time.Date(2024, 1, 5, 12, 0, 0, 0, time.UTC))
	tick2 := scheduler.BuildNewsFetchTasks(time.Date(2024, 1, 5, 12, 5, 0, 0, time.UTC))

	if len(tick1) != 3 {
		t.Fatalf("expected 3 news tasks, got %d", len(tick1))
	}

	for i := range tick1 {
		if tick1[i].ID != tick2[i].ID {
			t.Fatalf("expected stable IDs across ticks, got %q vs %q", tick1[i].ID, tick2[i].ID)
		}
		if tick1[i].Type != task.TypeNewsFetch {
			t.Fatalf("expected news_fetch type, got %q", tick1[i].Type)
		}
	}
}
