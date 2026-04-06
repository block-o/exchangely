package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type mockPruner struct {
	lastRetention time.Duration
	lastCount     int
	err           error
}

func (m *mockPruner) PruneOldTasks(ctx context.Context, olderThan time.Duration, keepMax int) (int64, error) {
	m.lastRetention = olderThan
	m.lastCount = keepMax
	return 10, m.err
}

func TestCleanupExecutorExecutesPrunerWithCorrectParams(t *testing.T) {
	pruner := &mockPruner{}
	retainFor := 48 * time.Hour
	retainCount := 500
	executor := NewCleanupExecutor(pruner, retainFor, retainCount)

	item := task.Task{Type: task.TypeCleanup}
	err := executor.Execute(context.Background(), item)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pruner.lastRetention != retainFor {
		t.Errorf("expected retention %v, got %v", retainFor, pruner.lastRetention)
	}
	if pruner.lastCount != retainCount {
		t.Errorf("expected count %d, got %d", retainCount, pruner.lastCount)
	}
}

func TestCleanupExecutorHandlesPrunerError(t *testing.T) {
	expectedErr := errors.New("prune failed")
	pruner := &mockPruner{err: expectedErr}
	executor := NewCleanupExecutor(pruner, 0, 0)

	item := task.Task{Type: task.TypeCleanup}
	err := executor.Execute(context.Background(), item)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestCleanupExecutorRejectsWrongTaskType(t *testing.T) {
	executor := NewCleanupExecutor(&mockPruner{}, 0, 0)
	item := task.Task{Type: task.TypeBackfill}
	err := executor.Execute(context.Background(), item)

	if err == nil {
		t.Fatal("expected error for wrong task type, got nil")
	}
}
