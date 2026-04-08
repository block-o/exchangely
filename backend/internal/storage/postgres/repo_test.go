package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
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
	pending, err := repo.Pending(ctx, 10, 10)
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

func TestTaskRepository_PendingCapsBackfillMix(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewTaskRepository(db, "test-worker")
	ctx := context.Background()

	_, _ = db.Exec("DELETE FROM tasks")

	now := time.Now().UTC().Truncate(time.Hour)
	items := []task.Task{
		{ID: "rt-1", Type: task.TypeRealtime, Pair: "BTCEUR", Interval: "realtime", WindowStart: now, WindowEnd: now.Add(time.Hour)},
		{ID: "integrity-1", Type: task.TypeDataSanity, Pair: "BTCEUR", Interval: "1h", WindowStart: now.Add(-time.Hour), WindowEnd: now},
		{ID: "cleanup-1", Type: task.TypeCleanup, Pair: "*", Interval: "1d", WindowStart: now.Truncate(24 * time.Hour), WindowEnd: now.Truncate(24 * time.Hour).Add(24 * time.Hour)},
		{ID: "backfill-1", Type: task.TypeBackfill, Pair: "BTCEUR", Interval: "1h", WindowStart: now.Add(-4 * time.Hour), WindowEnd: now.Add(-3 * time.Hour)},
		{ID: "backfill-2", Type: task.TypeBackfill, Pair: "ETHEUR", Interval: "1h", WindowStart: now.Add(-3 * time.Hour), WindowEnd: now.Add(-2 * time.Hour)},
		{ID: "backfill-3", Type: task.TypeBackfill, Pair: "XRPUSD", Interval: "1h", WindowStart: now.Add(-2 * time.Hour), WindowEnd: now.Add(-time.Hour)},
	}

	if _, err := repo.Enqueue(ctx, items); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	pending, err := repo.Pending(ctx, 5, 1)
	if err != nil {
		t.Fatalf("Pending failed: %v", err)
	}
	if len(pending) != 4 {
		t.Fatalf("expected 4 tasks (3 non-backfill + 1 backfill), got %d", len(pending))
	}

	backfillCount := 0
	for i, item := range pending {
		if item.Type == task.TypeBackfill {
			backfillCount++
			if i != len(pending)-1 {
				t.Fatalf("expected backfill task to be appended after non-backfill tasks, got %+v", pending)
			}
		}
	}
	if backfillCount != 1 {
		t.Fatalf("expected exactly 1 backfill task, got %d", backfillCount)
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

func TestMarketRepositoryTickerVolume24HFallsBackToQuoteTurnover(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewMarketRepository(db)
	ctx := context.Background()

	_, _ = db.Exec("DELETE FROM raw_candles WHERE pair_symbol = 'BTCEUR'")
	_, _ = db.Exec("DELETE FROM candles_1h WHERE pair_symbol = 'BTCEUR'")
	_, _ = db.Exec("DELETE FROM pairs WHERE symbol = 'BTCEUR'")
	_, _ = db.Exec("DELETE FROM assets WHERE symbol IN ('BTC', 'EUR')")

	_, err := db.Exec(`
		INSERT INTO assets (symbol, name, asset_type, circulating_supply)
		VALUES
			('BTC', 'Bitcoin', 'crypto', 19000000),
			('EUR', 'Euro', 'fiat', 0)
	`)
	if err != nil {
		t.Fatalf("seed assets failed: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO pairs (symbol, base_asset, quote_asset)
		VALUES ('BTCEUR', 'BTC', 'EUR')
	`)
	if err != nil {
		t.Fatalf("seed pair failed: %v", err)
	}

	windowStart := time.Date(2026, 4, 7, 20, 0, 0, 0, time.UTC)
	if err := repo.UpsertCandles(ctx, "1h", []candle.Candle{{
		Pair:      "BTCEUR",
		Interval:  "1h",
		Timestamp: windowStart.Unix(),
		Open:      59000,
		High:      60000,
		Low:       58000,
		Close:     59800,
		Volume:    5.5,
		Source:    "consolidated",
		Finalized: true,
	}}); err != nil {
		t.Fatalf("seed hourly candle failed: %v", err)
	}

	item, err := repo.Ticker(ctx, "BTCEUR")
	if err != nil {
		t.Fatalf("Ticker failed: %v", err)
	}

	want := 5.5 * 59800.0
	if item.Volume24H != want {
		t.Fatalf("expected fallback quote turnover %f, got %f", want, item.Volume24H)
	}
}

func TestMarketRepositoryTickerVolume24HPrefersNativeSnapshot(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewMarketRepository(db)
	ctx := context.Background()

	_, _ = db.Exec("DELETE FROM raw_candles WHERE pair_symbol = 'BTCEUR'")
	_, _ = db.Exec("DELETE FROM candles_1h WHERE pair_symbol = 'BTCEUR'")
	_, _ = db.Exec("DELETE FROM pairs WHERE symbol = 'BTCEUR'")
	_, _ = db.Exec("DELETE FROM assets WHERE symbol IN ('BTC', 'EUR')")

	_, err := db.Exec(`
		INSERT INTO assets (symbol, name, asset_type, circulating_supply)
		VALUES
			('BTC', 'Bitcoin', 'crypto', 19000000),
			('EUR', 'Euro', 'fiat', 0)
	`)
	if err != nil {
		t.Fatalf("seed assets failed: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO pairs (symbol, base_asset, quote_asset)
		VALUES ('BTCEUR', 'BTC', 'EUR')
	`)
	if err != nil {
		t.Fatalf("seed pair failed: %v", err)
	}

	windowStart := time.Date(2026, 4, 7, 20, 0, 0, 0, time.UTC)
	if err := repo.UpsertCandles(ctx, "1h", []candle.Candle{{
		Pair:      "BTCEUR",
		Interval:  "1h",
		Timestamp: windowStart.Unix(),
		Open:      59000,
		High:      60000,
		Low:       58000,
		Close:     59800,
		Volume:    5.5,
		Source:    "consolidated",
		Finalized: true,
	}}); err != nil {
		t.Fatalf("seed hourly candle failed: %v", err)
	}

	if err := repo.UpsertRawCandles(ctx, "1h", []candle.Candle{{
		Pair:      "BTCEUR",
		Interval:  "1h",
		Timestamp: windowStart.Unix(),
		Open:      59000,
		High:      60000,
		Low:       58000,
		Close:     59800,
		Volume:    5.5,
		Volume24H: 410000,
		Source:    "kraken",
		Finalized: false,
	}}); err != nil {
		t.Fatalf("seed raw candle failed: %v", err)
	}

	item, err := repo.Ticker(ctx, "BTCEUR")
	if err != nil {
		t.Fatalf("Ticker failed: %v", err)
	}

	if item.Volume24H != 410000 {
		t.Fatalf("expected native volume snapshot 410000, got %f", item.Volume24H)
	}
}
