package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

func TestProcessorIsIdempotentAcrossRepeatedDeliveries(t *testing.T) {
	store := &fakeStore{claimed: map[string]bool{}}
	locker := &fakeLocker{}
	executor := &fakeExecutor{}
	processor := NewProcessor(store, locker, executor)

	item := task.Task{ID: "backfill:BTCEUR:1", Pair: "BTCEUR"}

	if err := processor.Process(context.Background(), item); err != nil {
		t.Fatalf("first execution failed: %v", err)
	}

	if err := processor.Process(context.Background(), item); err != nil {
		t.Fatalf("second execution failed: %v", err)
	}

	if executor.calls != 1 {
		t.Fatalf("expected executor to run once, ran %d times", executor.calls)
	}
}

func TestRunnerPassesBackfillCapToPendingSource(t *testing.T) {
	source := &fakePendingSource{}
	runner := NewRunner(source, NewProcessor(&fakeStore{claimed: map[string]bool{}}, &fakeLocker{}, &fakeExecutor{}), 5, 12, 3, 1)

	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("runBatch failed: %v", err)
	}

	if source.limit != 12 {
		t.Fatalf("expected runner to request limit 12, got %d", source.limit)
	}
	if source.backfillLimit != 3 {
		t.Fatalf("expected runner to request backfill limit 3, got %d", source.backfillLimit)
	}
}

type fakeStore struct {
	claimed map[string]bool
}

func (s *fakeStore) Claim(_ context.Context, id string) (bool, error) {
	if s.claimed[id] {
		return false, nil
	}
	s.claimed[id] = true
	return true, nil
}

func (s *fakeStore) Complete(_ context.Context, _ string) error {
	return nil
}

func (s *fakeStore) Fail(_ context.Context, _, _ string) error {
	return nil
}

type fakeLocker struct{}

func (l *fakeLocker) Lock(_ context.Context, _ string) (UnlockFunc, error) {
	return func() error { return nil }, nil
}

type fakeExecutor struct {
	calls int
}

func (e *fakeExecutor) Execute(_ context.Context, _ task.Task) error {
	e.calls++
	return nil
}

type fakePendingSource struct {
	limit         int
	backfillLimit int
	tasks         []task.Task
}

func (s *fakePendingSource) Pending(_ context.Context, limit, backfillLimit int) ([]task.Task, error) {
	s.limit = limit
	s.backfillLimit = backfillLimit
	return s.tasks, nil
}

// --- Concurrency tests ---

// trackingExecutor records which task IDs were executed and supports
// artificial delays to verify parallel execution.
type trackingExecutor struct {
	mu       sync.Mutex
	executed []string
	delay    time.Duration
}

func (e *trackingExecutor) Execute(_ context.Context, t task.Task) error {
	if e.delay > 0 {
		time.Sleep(e.delay)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executed = append(e.executed, t.ID)
	return nil
}

func (e *trackingExecutor) executedIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := make([]string, len(e.executed))
	copy(cp, e.executed)
	return cp
}

// concurrencyTracker records the peak number of goroutines executing
// simultaneously inside the executor.
type concurrencyTracker struct {
	mu      sync.Mutex
	current int
	peak    int
	delay   time.Duration
}

func (c *concurrencyTracker) Execute(_ context.Context, _ task.Task) error {
	c.mu.Lock()
	c.current++
	if c.current > c.peak {
		c.peak = c.current
	}
	c.mu.Unlock()

	time.Sleep(c.delay)

	c.mu.Lock()
	c.current--
	c.mu.Unlock()
	return nil
}

func (c *concurrencyTracker) peakConcurrency() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peak
}

// threadSafeStore wraps fakeStore with a mutex for concurrent access.
type threadSafeStore struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func (s *threadSafeStore) Claim(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed[id] {
		return false, nil
	}
	s.claimed[id] = true
	return true, nil
}

func (s *threadSafeStore) Complete(_ context.Context, _ string) error { return nil }
func (s *threadSafeStore) Fail(_ context.Context, _, _ string) error  { return nil }

func makeTasks(n int) []task.Task {
	tasks := make([]task.Task, n)
	for i := range tasks {
		tasks[i] = task.Task{
			ID:   fmt.Sprintf("task-%d", i),
			Pair: fmt.Sprintf("PAIR%d", i),
		}
	}
	return tasks
}

func TestConcurrentBatchProcessesAllTasks(t *testing.T) {
	exe := &trackingExecutor{}
	store := &threadSafeStore{claimed: map[string]bool{}}
	processor := NewProcessor(store, &fakeLocker{}, exe)
	tasks := makeTasks(20)
	source := &fakePendingSource{tasks: tasks}

	runner := NewRunner(source, processor, time.Second, 20, 5, 4)
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("runBatch failed: %v", err)
	}

	got := exe.executedIDs()
	if len(got) != 20 {
		t.Fatalf("expected 20 tasks executed, got %d", len(got))
	}

	seen := make(map[string]bool)
	for _, id := range got {
		if seen[id] {
			t.Fatalf("task %s executed more than once", id)
		}
		seen[id] = true
	}
}

func TestConcurrentBatchRespectsMaxConcurrency(t *testing.T) {
	tracker := &concurrencyTracker{delay: 50 * time.Millisecond}
	store := &threadSafeStore{claimed: map[string]bool{}}
	processor := NewProcessor(store, &fakeLocker{}, tracker)
	tasks := makeTasks(10)
	source := &fakePendingSource{tasks: tasks}

	runner := NewRunner(source, processor, time.Second, 10, 5, 3)
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("runBatch failed: %v", err)
	}

	peak := tracker.peakConcurrency()
	if peak > 3 {
		t.Fatalf("expected peak concurrency <= 3, got %d", peak)
	}
	if peak < 2 {
		t.Logf("warning: peak concurrency was %d, expected at least 2 for 10 tasks with concurrency=3", peak)
	}
}

func TestSequentialBatchWhenConcurrencyIsOne(t *testing.T) {
	tracker := &concurrencyTracker{delay: 10 * time.Millisecond}
	store := &threadSafeStore{claimed: map[string]bool{}}
	processor := NewProcessor(store, &fakeLocker{}, tracker)
	tasks := makeTasks(5)
	source := &fakePendingSource{tasks: tasks}

	runner := NewRunner(source, processor, time.Second, 5, 2, 1)
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("runBatch failed: %v", err)
	}

	peak := tracker.peakConcurrency()
	if peak != 1 {
		t.Fatalf("expected peak concurrency of 1 (sequential), got %d", peak)
	}
}

func TestConcurrencyDefaultsToOneWhenZeroOrNegative(t *testing.T) {
	for _, c := range []int{0, -1, -100} {
		runner := NewRunner(&fakePendingSource{}, nil, time.Second, 10, 5, c)
		if runner.concurrency != 1 {
			t.Fatalf("NewRunner(concurrency=%d) should default to 1, got %d", c, runner.concurrency)
		}
	}
}

func TestConcurrentBatchIsFasterThanSequential(t *testing.T) {
	const taskCount = 8
	const taskDelay = 30 * time.Millisecond

	buildRunner := func(concurrency int) *Runner {
		exe := &trackingExecutor{delay: taskDelay}
		store := &threadSafeStore{claimed: map[string]bool{}}
		processor := NewProcessor(store, &fakeLocker{}, exe)
		source := &fakePendingSource{tasks: makeTasks(taskCount)}
		return NewRunner(source, processor, time.Second, taskCount, taskCount, concurrency)
	}

	seqRunner := buildRunner(1)
	start := time.Now()
	_ = seqRunner.runBatch(context.Background())
	seqDuration := time.Since(start)

	parRunner := buildRunner(4)
	start = time.Now()
	_ = parRunner.runBatch(context.Background())
	parDuration := time.Since(start)

	// Parallel should be meaningfully faster. With 8 tasks at 30ms each:
	// sequential ~240ms, parallel(4) ~60ms. We just check parallel < 75% of sequential.
	if parDuration >= seqDuration*3/4 {
		t.Fatalf("parallel batch (%v) was not meaningfully faster than sequential (%v)", parDuration, seqDuration)
	}
}

func TestConcurrentBatchTaskFailureDoesNotBlockOthers(t *testing.T) {
	failingExe := &selectiveFailExecutor{failIDs: map[string]bool{"task-2": true, "task-5": true}}
	store := &threadSafeStore{claimed: map[string]bool{}}
	processor := NewProcessor(store, &fakeLocker{}, failingExe)
	tasks := makeTasks(8)
	source := &fakePendingSource{tasks: tasks}

	runner := NewRunner(source, processor, time.Second, 8, 4, 4)
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("runBatch should not return error on task failures: %v", err)
	}

	// All 8 tasks should have been attempted (claimed).
	store.mu.Lock()
	claimedCount := len(store.claimed)
	store.mu.Unlock()
	if claimedCount != 8 {
		t.Fatalf("expected 8 tasks claimed, got %d", claimedCount)
	}
}

type selectiveFailExecutor struct {
	failIDs map[string]bool
}

func (e *selectiveFailExecutor) Execute(_ context.Context, t task.Task) error {
	if e.failIDs[t.ID] {
		return fmt.Errorf("intentional failure for %s", t.ID)
	}
	return nil
}

func TestConcurrentBatchEmptyBatchIsNoop(t *testing.T) {
	source := &fakePendingSource{tasks: nil}
	runner := NewRunner(source, nil, time.Second, 10, 5, 4)

	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("empty batch should succeed: %v", err)
	}
}

func TestConcurrentBatchSingleTask(t *testing.T) {
	exe := &trackingExecutor{}
	store := &threadSafeStore{claimed: map[string]bool{}}
	processor := NewProcessor(store, &fakeLocker{}, exe)
	source := &fakePendingSource{tasks: makeTasks(1)}

	runner := NewRunner(source, processor, time.Second, 10, 5, 4)
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatalf("runBatch failed: %v", err)
	}

	got := exe.executedIDs()
	if len(got) != 1 {
		t.Fatalf("expected 1 task executed, got %d", len(got))
	}
}
