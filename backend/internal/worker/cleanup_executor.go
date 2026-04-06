package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// TaskPruner can delete old completed/failed tasks from the task repository.
type TaskPruner interface {
	// PruneOldTasks removes tasks either older than olderThan or exceeding keepMax count.
	PruneOldTasks(ctx context.Context, olderThan time.Duration, keepMax int) (int64, error)
}

// CleanupExecutor removes old completed/failed task rows to prevent unbounded table growth.
// It is driven by the scheduler like any other task type — no separate goroutine needed.
type CleanupExecutor struct {
	pruner      TaskPruner
	retainFor   time.Duration // how far back to keep completed tasks
	retainCount int           // maximum number of tasks to keep
}

func NewCleanupExecutor(pruner TaskPruner, retainFor time.Duration, retainCount int) *CleanupExecutor {
	if retainFor <= 0 {
		retainFor = 24 * time.Hour
	}
	if retainCount <= 0 {
		retainCount = 1000
	}
	return &CleanupExecutor{
		pruner:      pruner,
		retainFor:   retainFor,
		retainCount: retainCount,
	}
}

func (c *CleanupExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeCleanup {
		return fmt.Errorf("cleanup executor received unexpected task type %q", item.Type)
	}

	slog.Info("task cleanup started", "retain_for", c.retainFor, "retain_count", c.retainCount)
	deleted, err := c.pruner.PruneOldTasks(ctx, c.retainFor, c.retainCount)
	if err != nil {
		slog.Warn("task cleanup failed", "error", err)
		return err
	}
	slog.Info("task cleanup completed", "deleted_rows", deleted)
	return nil
}
