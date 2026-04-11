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

// ---------------------------------------------------------------------------
// Enqueue dedup — realtime task at-most-once guarantee
// ---------------------------------------------------------------------------

// TestEnqueueRealtimeDedup verifies the at-most-one-per-pair guarantee:
//  1. First enqueue inserts the task (pending).
//  2. Second enqueue while pending is a no-op (not re-enqueued).
//  3. After the task is claimed (running), enqueue is still a no-op.
//  4. After the task is completed, enqueue re-activates it as pending.
//  5. After the task fails permanently, enqueue re-activates it as pending.
func TestEnqueueRealtimeDedup(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewTaskRepository(db, "test-worker")
	ctx := context.Background()

	_, _ = db.Exec("DELETE FROM tasks")

	now := time.Now().UTC().Truncate(time.Hour)
	rt := task.Task{
		ID:          "live_ticker:BTCEUR:realtime",
		Type:        task.TypeRealtime,
		Pair:        "BTCEUR",
		Interval:    "realtime",
		WindowStart: now,
		WindowEnd:   now.Add(time.Hour),
	}

	// 1. First enqueue — should insert.
	enqueued, err := repo.Enqueue(ctx, []task.Task{rt})
	if err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueued))
	}

	assertTaskStatus(t, db, rt.ID, "pending")

	// 2. Second enqueue while pending — should be a no-op.
	enqueued, err = repo.Enqueue(ctx, []task.Task{rt})
	if err != nil {
		t.Fatalf("second enqueue failed: %v", err)
	}
	if len(enqueued) != 0 {
		t.Fatalf("expected 0 enqueued (pending dedup), got %d", len(enqueued))
	}

	// 3. Claim the task (simulate worker picking it up).
	claimed, err := repo.Claim(ctx, rt.ID)
	if err != nil || !claimed {
		t.Fatalf("claim failed: claimed=%v err=%v", claimed, err)
	}
	assertTaskStatus(t, db, rt.ID, "running")

	// Enqueue while running — should be a no-op.
	enqueued, err = repo.Enqueue(ctx, []task.Task{rt})
	if err != nil {
		t.Fatalf("enqueue while running failed: %v", err)
	}
	if len(enqueued) != 0 {
		t.Fatalf("expected 0 enqueued (running dedup), got %d", len(enqueued))
	}

	// 4. Complete the task.
	if err := repo.Complete(ctx, rt.ID); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	assertTaskStatus(t, db, rt.ID, "completed")

	// Enqueue after completion — should re-activate as pending.
	nextHour := now.Add(time.Hour)
	rtRefreshed := rt
	rtRefreshed.WindowStart = nextHour
	rtRefreshed.WindowEnd = nextHour.Add(time.Hour)

	enqueued, err = repo.Enqueue(ctx, []task.Task{rtRefreshed})
	if err != nil {
		t.Fatalf("enqueue after completion failed: %v", err)
	}
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 re-enqueued task after completion, got %d", len(enqueued))
	}
	assertTaskStatus(t, db, rt.ID, "pending")

	// Verify the window was updated.
	var ws, we time.Time
	err = db.QueryRow("SELECT window_start, window_end FROM tasks WHERE id = $1", rt.ID).Scan(&ws, &we)
	if err != nil {
		t.Fatalf("query window failed: %v", err)
	}
	if !ws.UTC().Equal(nextHour.UTC()) {
		t.Fatalf("expected updated window_start %s, got %s", nextHour.UTC(), ws.UTC())
	}

	// 5. Claim + fail permanently, then re-enqueue.
	_, _ = repo.Claim(ctx, rt.ID)
	// Set retry_count high so Fail marks it as 'failed' permanently.
	_, _ = db.Exec("UPDATE tasks SET retry_count = 100 WHERE id = $1", rt.ID)
	if err := repo.Fail(ctx, rt.ID, "permanent failure"); err != nil {
		t.Fatalf("fail failed: %v", err)
	}
	assertTaskStatus(t, db, rt.ID, "failed")

	enqueued, err = repo.Enqueue(ctx, []task.Task{rt})
	if err != nil {
		t.Fatalf("enqueue after failure failed: %v", err)
	}
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 re-enqueued task after failure, got %d", len(enqueued))
	}
	assertTaskStatus(t, db, rt.ID, "pending")

	// Verify retry state was reset.
	var retryCount int
	err = db.QueryRow("SELECT retry_count FROM tasks WHERE id = $1", rt.ID).Scan(&retryCount)
	if err != nil {
		t.Fatalf("query retry_count failed: %v", err)
	}
	if retryCount != 0 {
		t.Fatalf("expected retry_count reset to 0, got %d", retryCount)
	}
}

// TestEnqueueDedupDoesNotAffectDifferentIDs verifies that the dedup logic
// only applies to tasks with the same ID — different IDs are independent.
func TestEnqueueDedupDoesNotAffectDifferentIDs(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewTaskRepository(db, "test-worker")
	ctx := context.Background()

	_, _ = db.Exec("DELETE FROM tasks")

	now := time.Now().UTC().Truncate(time.Hour)
	btc := task.Task{
		ID:          "live_ticker:BTCEUR:realtime",
		Type:        task.TypeRealtime,
		Pair:        "BTCEUR",
		Interval:    "realtime",
		WindowStart: now,
		WindowEnd:   now.Add(time.Hour),
	}
	eth := task.Task{
		ID:          "live_ticker:ETHUSD:realtime",
		Type:        task.TypeRealtime,
		Pair:        "ETHUSD",
		Interval:    "realtime",
		WindowStart: now,
		WindowEnd:   now.Add(time.Hour),
	}

	enqueued, err := repo.Enqueue(ctx, []task.Task{btc, eth})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if len(enqueued) != 2 {
		t.Fatalf("expected 2 enqueued tasks, got %d", len(enqueued))
	}

	// Complete BTC, leave ETH pending.
	_, _ = repo.Claim(ctx, btc.ID)
	_ = repo.Complete(ctx, btc.ID)

	// Re-enqueue both — only BTC should be re-activated.
	enqueued, err = repo.Enqueue(ctx, []task.Task{btc, eth})
	if err != nil {
		t.Fatalf("re-enqueue failed: %v", err)
	}
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 re-enqueued (BTC only), got %d", len(enqueued))
	}
	if enqueued[0].ID != btc.ID {
		t.Fatalf("expected BTC to be re-enqueued, got %q", enqueued[0].ID)
	}
}

func assertTaskStatus(t *testing.T, db *sql.DB, taskID, expected string) {
	t.Helper()
	var status string
	err := db.QueryRow("SELECT status FROM tasks WHERE id = $1", taskID).Scan(&status)
	if err != nil {
		t.Fatalf("query status for %q failed: %v", taskID, err)
	}
	if status != expected {
		t.Fatalf("expected task %q status %q, got %q", taskID, expected, status)
	}
}

// ---------------------------------------------------------------------------
// Ticker query — stale data exclusion
// ---------------------------------------------------------------------------

// TestTickerExcludesStaleData verifies that the ticker snapshot query only
// considers candles within the last 30 days. A pair with only old data (>30d)
// should return sql.ErrNoRows for the single-ticker path and be absent from
// the all-tickers result, while a pair with recent data is returned normally.
func TestTickerExcludesStaleData(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewMarketRepository(db)
	ctx := context.Background()

	stalePair := "STALETESTUSD"
	freshPair := "FRESHTESTUSD"

	// Cleanup
	for _, p := range []string{stalePair, freshPair} {
		_, _ = db.Exec("DELETE FROM raw_candles WHERE pair_symbol = $1", p)
		_, _ = db.Exec("DELETE FROM candles_1h WHERE pair_symbol = $1", p)
	}
	t.Cleanup(func() {
		for _, p := range []string{stalePair, freshPair} {
			_, _ = db.Exec("DELETE FROM raw_candles WHERE pair_symbol = $1", p)
			_, _ = db.Exec("DELETE FROM candles_1h WHERE pair_symbol = $1", p)
		}
	})

	// Seed a stale candle (60 days ago — outside the 30-day window).
	staleTime := time.Now().UTC().Add(-60 * 24 * time.Hour).Truncate(time.Hour)
	if err := repo.UpsertCandles(ctx, "1h", []candle.Candle{{
		Pair:      stalePair,
		Interval:  "1h",
		Timestamp: staleTime.Unix(),
		Open:      100, High: 105, Low: 95, Close: 102,
		Volume: 10, Source: "test", Finalized: true,
	}}); err != nil {
		t.Fatalf("seed stale candle failed: %v", err)
	}

	// Seed a fresh candle (1 hour ago — inside the 30-day window).
	freshTime := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Hour)
	if err := repo.UpsertCandles(ctx, "1h", []candle.Candle{{
		Pair:      freshPair,
		Interval:  "1h",
		Timestamp: freshTime.Unix(),
		Open:      200, High: 210, Low: 190, Close: 205,
		Volume: 20, Source: "test", Finalized: true,
	}}); err != nil {
		t.Fatalf("seed fresh candle failed: %v", err)
	}

	// Single-ticker: stale pair should return no rows.
	_, err := repo.Ticker(ctx, stalePair)
	if err == nil {
		t.Fatal("expected error for stale pair ticker, got nil")
	}

	// Single-ticker: fresh pair should succeed.
	freshTicker, err := repo.Ticker(ctx, freshPair)
	if err != nil {
		t.Fatalf("Ticker for fresh pair failed: %v", err)
	}
	if freshTicker.Pair != freshPair {
		t.Fatalf("expected pair %q, got %q", freshPair, freshTicker.Pair)
	}
	if freshTicker.Price != 205 {
		t.Fatalf("expected price 205, got %v", freshTicker.Price)
	}

	// All-tickers: stale pair should be absent, fresh pair should be present.
	tickers, err := repo.Tickers(ctx)
	if err != nil {
		t.Fatalf("Tickers failed: %v", err)
	}

	foundStale := false
	foundFresh := false
	for _, tk := range tickers {
		if tk.Pair == stalePair {
			foundStale = true
		}
		if tk.Pair == freshPair {
			foundFresh = true
		}
	}
	if foundStale {
		t.Fatal("expected stale pair to be excluded from tickers result")
	}
	if !foundFresh {
		t.Fatal("expected fresh pair to be present in tickers result")
	}
}
