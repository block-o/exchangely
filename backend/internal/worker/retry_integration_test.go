package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/storage/postgres"
	"github.com/block-o/exchangely/backend/internal/testutil"
)

type errorExecutor struct{}

func (e *errorExecutor) Execute(ctx context.Context, item task.Task) error {
	return fmt.Errorf("persistent failure")
}

func TestWorker_RetryStateIntegration(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	repo := postgres.NewTaskRepository(db, "worker-a")

	// 1. Cleanup
	_, _ = db.Exec("DELETE FROM tasks")

	item := task.Task{
		ID:          "worker-retry-test",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Now().Add(-time.Hour),
		WindowEnd:   time.Now(),
	}

	// 2. Enqueue
	_, err := repo.Enqueue(ctx, []task.Task{item})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// 3. Process with failure
	processor := NewProcessor(repo, &fakeLocker{}, &errorExecutor{})
	_ = processor.Process(ctx, item)

	// 4. Verify DB state via Repository
	pending, err := repo.Pending(ctx, 10, 10)
	if err != nil {
		t.Fatalf("Pending failed: %v", err)
	}

	for _, p := range pending {
		if p.ID == item.ID {
			t.Error("Task should be in cooldown and NOT returned by Pending")
		}
	}

	// 5. Verify it's still 'pending' in DB but has retry_at
	var status string
	var retryCount int
	var retryAt *time.Time
	err = db.QueryRow("SELECT status, retry_count, retry_at FROM tasks WHERE id = $1", item.ID).Scan(&status, &retryCount, &retryAt)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if status != "pending" {
		t.Errorf("expected status pending for retry, got %q", status)
	}
	if retryCount != 1 {
		t.Errorf("expected retryCount 1, got %d", retryCount)
	}
	if retryAt == nil || retryAt.Before(time.Now()) {
		t.Errorf("expected future retryAt, got %v", retryAt)
	}
}
