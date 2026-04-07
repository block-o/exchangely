package worker

import (
	"context"
	"testing"

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
	runner := NewRunner(source, NewProcessor(&fakeStore{claimed: map[string]bool{}}, &fakeLocker{}, &fakeExecutor{}), 5, 12, 3)

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
}

func (s *fakePendingSource) Pending(_ context.Context, limit, backfillLimit int) ([]task.Task, error) {
	s.limit = limit
	s.backfillLimit = backfillLimit
	return nil, nil
}
