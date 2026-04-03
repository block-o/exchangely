package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// TaskPruner can delete old completed/failed tasks from the task log.
type TaskPruner interface {
	PruneOldTasks(ctx context.Context, olderThan time.Duration) (int64, error)
}

// CleanupExecutor removes old completed/failed task rows to prevent unbounded table growth.
// It is driven by the scheduler like any other task type — no separate goroutine needed.
type CleanupExecutor struct {
	pruner    TaskPruner
	retainFor time.Duration // how far back to keep completed tasks (default: 7 days)
}

func NewCleanupExecutor(pruner TaskPruner, retainFor time.Duration) *CleanupExecutor {
	if retainFor <= 0 {
		retainFor = 7 * 24 * time.Hour
	}
	return &CleanupExecutor{pruner: pruner, retainFor: retainFor}
}

func (c *CleanupExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeCleanup {
		return fmt.Errorf("cleanup executor received unexpected task type %q", item.Type)
	}

	slog.Info("task cleanup started", "retain_for", c.retainFor)
	deleted, err := c.pruner.PruneOldTasks(ctx, c.retainFor)
	if err != nil {
		slog.Warn("task cleanup failed", "error", err)
		return err
	}
	slog.Info("task cleanup completed", "deleted_rows", deleted)
	return nil
}
