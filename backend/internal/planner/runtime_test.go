package planner

import (
	"context"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
)

func TestRunTickSkipsSchedulingWhenLeaseNotAcquired(t *testing.T) {
	runner := NewRunner(
		"instance-a",
		"planner_leader",
		15*time.Second,
		10*time.Second,
		NewScheduler(2*time.Minute),
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		&fakeSyncProvider{states: map[string]postgresrepo.SyncState{}},
		fakeLeaseCoordinator{acquired: false},
		&fakeTaskSink{},
		&fakeTaskPublisher{},
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
		NewScheduler(2*time.Minute),
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		syncProvider,
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		publisher,
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	if len(syncProvider.seededPairs) != 1 || syncProvider.seededPairs[0] != "BTCEUR" {
		t.Fatalf("expected BTCEUR to be seeded, got %+v", syncProvider.seededPairs)
	}
	if taskSink.enqueueCalls != 1 {
		t.Fatalf("expected a single enqueue call, got %d", taskSink.enqueueCalls)
	}
	if len(taskSink.tasks) == 0 {
		t.Fatal("expected tasks to be enqueued")
	}
	if publisher.publishCalls != 1 {
		t.Fatalf("expected a single publish call, got %d", publisher.publishCalls)
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
		NewScheduler(2*time.Minute),
		fakePairProvider{pairs: []pair.Pair{{Symbol: "BTCEUR"}}},
		&fakeSyncProvider{
			states: map[string]postgresrepo.SyncState{
				"BTCEUR": nowState,
			},
		},
		fakeLeaseCoordinator{acquired: true},
		taskSink,
		publisher,
	)

	if err := runner.runTick(context.Background()); err != nil {
		t.Fatalf("runTick failed: %v", err)
	}

	foundRealtime := false
	for _, item := range taskSink.tasks {
		if item.Type == task.TypeRealtime && item.Pair == "BTCEUR" && item.Interval == "1h" {
			foundRealtime = true
			break
		}
	}
	if !foundRealtime {
		t.Fatalf("expected realtime task in %+v", taskSink.tasks)
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
	err          error
}

func (f *fakeTaskSink) Enqueue(_ context.Context, tasks []task.Task) ([]task.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.enqueueCalls++
	f.tasks = append([]task.Task{}, tasks...)
	return append([]task.Task{}, tasks...), nil
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
	f.tasks = append([]task.Task{}, tasks...)
	return nil
}
