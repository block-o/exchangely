package planner

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
	"github.com/google/uuid"
)

func TestRunTickSkipsSchedulingWhenLeaseNotAcquired(t *testing.T) {
	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour),
		4,
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		&fakeSyncProvider{states: map[string]postgresrepo.SyncState{}},
		fakeLeaseCoordinator{acquired: false},
		&fakeTaskSink{},
		&fakeCoverageProvider{},
		&fakeIntegrityCoverageProvider{},
		&fakeTaskPublisher{},
		nil,
	)

	tasks := runner.tasks.(*fakeTaskSink)
	publisher := runner.publisher.(*fakeTaskPublisher)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	if tasks.enqueueCalls != 0 {
		t.Fatalf("expected no enqueue when lease not acquired, got %d", tasks.enqueueCalls)
	}
	if publisher.publishCalls != 0 {
		t.Fatalf("expected no publish when lease not acquired, got %d", publisher.publishCalls)
	}
}

func TestRunTickSeedsMissingPairsAndEnqueuesTasks(t *testing.T) {
	syncProvider := &fakeSyncProvider{
		states: map[string]postgresrepo.SyncState{},
	}
	taskSink := &fakeTaskSink{}
	publisher := &fakeTaskPublisher{}

	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour),
		4,
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		syncProvider,
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		&fakeCoverageProvider{},
		&fakeIntegrityCoverageProvider{},
		publisher,
		nil,
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	if len(syncProvider.seededPairs) != 1 || syncProvider.seededPairs[0] != "BTCEUR" {
		t.Fatalf("expected BTCEUR to be seeded, got %+v", syncProvider.seededPairs)
	}
	if taskSink.enqueueCalls != 2 {
		t.Fatalf("expected realtime and follow-up enqueue calls, got %d", taskSink.enqueueCalls)
	}
	if len(taskSink.tasks) == 0 {
		t.Fatal("expected tasks to be enqueued")
	}
	if publisher.publishCalls != 2 {
		t.Fatalf("expected realtime and follow-up publish calls, got %d", publisher.publishCalls)
	}
	if len(publisher.tasks) != len(taskSink.tasks) {
		t.Fatalf("expected publisher to receive same tasks, got %d vs %d", len(publisher.tasks), len(taskSink.tasks))
	}
}

func TestRunTickPublishesRealtimeForCaughtUpPairs(t *testing.T) {
	nowState := postgresrepo.SyncState{
		HourlyLastSynced:        time.Now().UTC(),
		DailyLastSynced:         time.Now().UTC().Truncate(24 * time.Hour),
		HourlyBackfillCompleted: true,
		DailyBackfillCompleted:  true,
	}

	taskSink := &fakeTaskSink{}
	publisher := &fakeTaskPublisher{}
	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour),
		4,
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		&fakeSyncProvider{
			states: map[string]postgresrepo.SyncState{
				"BTCEUR": nowState,
			},
		},
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		&fakeCoverageProvider{},
		&fakeIntegrityCoverageProvider{},
		publisher,
		nil,
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	foundRealtime := false
	for _, item := range taskSink.tasks {
		if item.Type == task.TypeRealtime && item.Pair == "BTCEUR" && item.Interval == "realtime" {
			foundRealtime = true
			break
		}
	}
	if !foundRealtime {
		t.Fatalf("expected realtime task in %+v", taskSink.tasks)
	}
}

func TestRunTickEnqueuesRealtimeBeforeCappedBackfill(t *testing.T) {
	taskSink := &fakeTaskSink{}
	publisher := &fakeTaskPublisher{}

	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour),
		2,
		fakePairProvider{pairs: []pair.Pair{
			{Symbol: "BTCEUR"},
			{Symbol: "ETHUSD"},
		}},
		&fakeSyncProvider{
			states: map[string]postgresrepo.SyncState{
				"BTCEUR": {},
				"ETHUSD": {},
			},
		},
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		&fakeCoverageProvider{},
		&fakeIntegrityCoverageProvider{},
		publisher,
		nil,
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	if len(taskSink.batches) != 2 {
		t.Fatalf("expected 2 enqueue batches, got %d", len(taskSink.batches))
	}
	if len(taskSink.batches[0]) != 2 {
		t.Fatalf("expected 2 realtime tasks in first batch, got %d", len(taskSink.batches[0]))
	}
	for _, item := range taskSink.batches[0] {
		if item.Type != task.TypeRealtime {
			t.Fatalf("expected only realtime tasks in first batch, got %+v", taskSink.batches[0])
		}
	}

	backfillCount := 0
	for _, item := range taskSink.batches[1] {
		if item.Type == task.TypeBackfill {
			backfillCount++
		}
	}
	if backfillCount != 2 {
		t.Fatalf("expected capped backfill count 2, got %d", backfillCount)
	}
	if len(taskSink.batches[1]) <= backfillCount {
		t.Fatalf("expected follow-up batch to also include non-backfill tasks, got %+v", taskSink.batches[1])
	}
}

func TestRunTickEnqueuesPnLRefreshWhenPortfolioUsersExist(t *testing.T) {
	taskSink := &fakeTaskSink{}
	publisher := &fakeTaskPublisher{}
	userID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour),
		4,
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		&fakeSyncProvider{states: map[string]postgresrepo.SyncState{"BTCEUR": {}}},
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		&fakeCoverageProvider{},
		&fakeIntegrityCoverageProvider{},
		publisher,
		&fakePortfolioUserProvider{userIDs: []uuid.UUID{userID}},
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	foundPnLRefresh := false
	expectedID := "pnl_refresh:" + userID.String() + ":periodic"
	for _, item := range taskSink.tasks {
		if item.Type == task.TypePnLRefresh && item.ID == expectedID {
			foundPnLRefresh = true
			break
		}
	}
	if !foundPnLRefresh {
		t.Fatalf("expected pnl_refresh task with ID %s in %+v", expectedID, taskSink.tasks)
	}
}

func TestRunTickSkipsPnLRefreshWhenPortfolioDisabled(t *testing.T) {
	taskSink := &fakeTaskSink{}
	publisher := &fakeTaskPublisher{}

	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(5*time.Second, 5*time.Minute, 24*time.Hour, 24*time.Hour, 1*time.Hour),
		4,
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		&fakeSyncProvider{states: map[string]postgresrepo.SyncState{"BTCEUR": {}}},
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		&fakeCoverageProvider{},
		&fakeIntegrityCoverageProvider{},
		publisher,
		nil,
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	for _, item := range taskSink.tasks {
		if item.Type == task.TypePnLRefresh {
			t.Fatalf("expected no pnl_refresh tasks when portfolio is disabled, got %+v", item)
		}
	}
}

func TestComputeBackfillTaskCapClampsToWorkerBatchSize(t *testing.T) {
	if got := ComputeBackfillTaskCap(8, 150); got != 8 {
		t.Fatalf("expected cap to clamp to worker batch size 8, got %d", got)
	}
	if got := ComputeBackfillTaskCap(8, 25); got != 2 {
		t.Fatalf("expected 25%% of batch size 8 to round up to 2, got %d", got)
	}
	if got := ComputeBackfillTaskCap(8, 0); got != 0 {
		t.Fatalf("expected 0 percent to disable capped backfill, got %d", got)
	}
}

type fakePairProvider struct {
	pairs []pair.Pair
	err   error
}

func (f fakePairProvider) ListPairs(_ context.Context) ([]pair.Pair, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]pair.Pair{}, f.pairs...), nil
}

type fakeSyncProvider struct {
	states      map[string]postgresrepo.SyncState
	seededPairs []string
	err         error
}

func (f *fakeSyncProvider) States(_ context.Context) (map[string]postgresrepo.SyncState, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := make(map[string]postgresrepo.SyncState, len(f.states))
	for key, value := range f.states {
		result[key] = value
	}
	return result, nil
}

func (f *fakeSyncProvider) MarkBackfillSeeded(_ context.Context, pairSymbol string, _ time.Time) error {
	f.seededPairs = append(f.seededPairs, pairSymbol)
	return nil
}

type fakeLeaseCoordinator struct {
	acquired bool
	err      error
}

func (f fakeLeaseCoordinator) AcquireOrRenew(_ context.Context, name, holder string, ttl time.Duration) (lease.Lease, bool, error) {
	if f.err != nil {
		return lease.Lease{}, false, f.err
	}
	return lease.Lease{
		Name:      name,
		HolderID:  holder,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}, f.acquired, nil
}

type fakeTaskSink struct {
	enqueueCalls int
	tasks        []task.Task
	batches      [][]task.Task
	err          error
}

func (f *fakeTaskSink) Enqueue(_ context.Context, tasks []task.Task) ([]task.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.enqueueCalls++
	batch := append([]task.Task{}, tasks...)
	f.batches = append(f.batches, batch)
	f.tasks = append(f.tasks, batch...)
	return append([]task.Task{}, tasks...), nil
}

func (f *fakeTaskSink) ActiveBackfillPairs(_ context.Context) (map[string]bool, error) {
	return make(map[string]bool), nil
}

type fakeTaskPublisher struct {
	publishCalls int
	tasks        []task.Task
	err          error
}

func (f *fakeTaskPublisher) Publish(_ context.Context, tasks []task.Task) error {
	if f.err != nil {
		return f.err
	}
	f.publishCalls++
	f.tasks = append(f.tasks, tasks...)
	return nil
}

type fakeCoverageProvider struct {
	coverage map[string]map[string]bool
	err      error
}

func (f *fakeCoverageProvider) GetAllCompletedDays(_ context.Context) (map[string]map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.coverage == nil {
		return make(map[string]map[string]bool), nil
	}
	return f.coverage, nil
}

type fakeIntegrityCoverageProvider struct {
	coverage map[string]map[string]bool
	err      error
}

func (f *fakeIntegrityCoverageProvider) GetAllVerifiedDays(_ context.Context) (map[string]map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.coverage == nil {
		return make(map[string]map[string]bool), nil
	}
	return f.coverage, nil
}

type fakePortfolioUserProvider struct {
	userIDs []uuid.UUID
	err     error
}

func (f *fakePortfolioUserProvider) ListDistinctUserIDs(_ context.Context) ([]uuid.UUID, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]uuid.UUID{}, f.userIDs...), nil
}
