package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

func setupTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("EXCHANGELY_TEST_DATABASE_URL")
	if dsn == "" {
		// Fallback to a common local default if not set
		dsn = "postgres://postgres:postgres@localhost:5432/exchangely_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := Open(ctx, dsn)
	if err != nil {
		t.Skipf("Skipping postgres integration test: database not available at %s", dsn)
	}

	return db
}

func TestTaskRepository_RetriesAndCooldown(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewTaskRepository(db, "test-worker")
	ctx := context.Background()

	// 1. Cleanup
	_, _ = db.Exec("DELETE FROM tasks")

	testTask := task.Task{
		ID:          "test-retry-1h",
		Type:        task.TypeBackfill,
		Pair:        "BTCEUR",
		Interval:    "1h",
		WindowStart: time.Now().Add(-2 * time.Hour),
		WindowEnd:   time.Now().Add(-1 * time.Hour),
	}

	// 2. Enqueue
	_, err := repo.Enqueue(ctx, []task.Task{testTask})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// 3. Fail once
	err = repo.Fail(ctx, testTask.ID, "first failure")
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	// 4. Verify status and retry metadata
	var status string
	var retryCount int
	var retryAt *time.Time
	err = db.QueryRow("SELECT status, retry_count, retry_at FROM tasks WHERE id = $1", testTask.ID).Scan(&status, &retryCount, &retryAt)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if status != "pending" {
		t.Errorf("expected status pending for retry, got %q", status)
	}
	if retryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", retryCount)
	}
	if retryAt == nil {
		t.Fatal("expected retry_at to be set")
	}

	// Verify cooldown (1h +/- 5m Jitter)
	now := time.Now().UTC()
	minCooldown := now.Add(55 * time.Minute)
	maxCooldown := now.Add(65 * time.Minute)
	if retryAt.Before(minCooldown) || retryAt.After(maxCooldown) {
		t.Errorf("retry_at %v out of expected range [%v, %v]", retryAt, minCooldown, maxCooldown)
	}

	// 5. Verify Pending respects retry_at
	pending, err := repo.Pending(ctx, 10)
	if err != nil {
		t.Fatalf("Pending failed: %v", err)
	}
	for _, p := range pending {
		if p.ID == testTask.ID {
			t.Error("Pending should NOT return task in cooldown")
		}
	}

	// 6. Test Retry Limits (1h -> 24 retries)
	// Fast-forward retry_count to 23
	_, err = db.Exec("UPDATE tasks SET retry_count = 23 WHERE id = $1", testTask.ID)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Fail the 24th time -> still pending (retryCount becomes 24)
	err = repo.Fail(ctx, testTask.ID, "24th failure")
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	err = db.QueryRow("SELECT status, retry_count FROM tasks WHERE id = $1", testTask.ID).Scan(&status, &retryCount)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected status pending after 24th fail (24th retry), got %q", status)
	}

	// Fail the 25th time -> finally failed
	err = repo.Fail(ctx, testTask.ID, "last failure")
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	err = db.QueryRow("SELECT status, retry_count FROM tasks WHERE id = $1", testTask.ID).Scan(&status, &retryCount)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status failed after exceed retries, got %q", status)
	}
	if retryCount != 25 {
		t.Errorf("expected retry_count 25, got %d", retryCount)
	}
}

func TestCoverageRepository_Integration(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewCoverageRepository(db)
	ctx := context.Background()

	// 1. Cleanup
	_, _ = db.Exec("DELETE FROM data_coverage")

	day := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pair := "BTCEUR"

	// 3. Initial state
	coverage, err := repo.GetAllCompletedDays(ctx)
	if err != nil {
		t.Fatalf("GetAllCompletedDays failed: %v", err)
	}
	if coverage[pair][day.Format("2006-01-02")] {
		t.Fatal("expected no coverage initially")
	}

	// 4. Mark complete
	err = repo.MarkDayComplete(ctx, pair, day)
	if err != nil {
		t.Fatalf("MarkDayComplete failed: %v", err)
	}

	// 5. Verify
	coverage, err = repo.GetAllCompletedDays(ctx)
	if err != nil {
		t.Fatalf("GetAllCompletedDays failed: %v", err)
	}
	if !coverage[pair][day.Format("2006-01-02")] {
		t.Fatal("expected day to be marked complete")
	}
}
