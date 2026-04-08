package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Ping(context.Context) error {
	return f.err
}

type fakeSyncReader struct {
	rows []postgresrepo.SyncRow
	err  error
}

func (f fakeSyncReader) SnapshotRows(context.Context) ([]postgresrepo.SyncRow, error) {
	return f.rows, f.err
}

type fakeLeaseReader struct{}

func (fakeLeaseReader) Current(context.Context, string) (lease.Lease, error) {
	return lease.Lease{}, nil
}

type fakeTaskReader struct {
	recent []task.Task
	err    error
}

func (f fakeTaskReader) UpcomingTasks(context.Context, int, int) ([]task.Task, int, error) {
	return nil, 0, nil
}

func (f fakeTaskReader) RecentTasks(context.Context, int, int, []string, []string) ([]task.Task, int, error) {
	return f.recent, len(f.recent), f.err
}

type fakeWarningStore struct {
	dismissed map[string]string
}

func (f fakeWarningStore) DismissWarning(context.Context, string, string) error {
	return nil
}

func (f fakeWarningStore) DismissedWarnings(context.Context) (map[string]string, error) {
	return f.dismissed, nil
}

func TestActiveWarningsFiltersDismissedFingerprint(t *testing.T) {
	ctx := context.Background()
	taskReader := fakeTaskReader{
		recent: []task.Task{
			{
				ID:        "failed-integrity",
				Type:      task.TypeDataSanity,
				Pair:      "BTCEUR",
				Status:    "failed",
				LastError: "validator mismatch",
			},
		},
	}
	syncReader := fakeSyncReader{
		rows: []postgresrepo.SyncRow{
			{Pair: "BTCEUR", HourlyBackfillCompleted: false},
		},
	}

	serviceWithoutDismissal := NewSystemService(
		fakePinger{},
		fakePinger{err: errors.New("kafka down")},
		syncReader,
		taskReader,
		fakeWarningStore{},
		fakeLeaseReader{},
		"planner",
		time.Minute,
	)

	warnings, err := serviceWithoutDismissal.ActiveWarnings(ctx)
	if err != nil {
		t.Fatalf("ActiveWarnings returned error: %v", err)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected multiple warnings, got %d", len(warnings))
	}

	var systemHealthFingerprint string
	for _, warning := range warnings {
		if warning.ID == "system-health" {
			systemHealthFingerprint = warning.Fingerprint
			break
		}
	}
	if systemHealthFingerprint == "" {
		t.Fatal("expected system-health warning to be present")
	}

	// Verify individual task failure warnings are present
	foundIntegrityWarning := false
	for _, warning := range warnings {
		if warning.ID == "integrity-failure-failed-integrity" {
			foundIntegrityWarning = true
			break
		}
	}
	if !foundIntegrityWarning {
		t.Fatal("expected individual integrity failure warning to be present")
	}

	serviceWithDismissal := NewSystemService(
		fakePinger{},
		fakePinger{err: errors.New("kafka down")},
		syncReader,
		taskReader,
		fakeWarningStore{dismissed: map[string]string{"system-health": systemHealthFingerprint}},
		fakeLeaseReader{},
		"planner",
		time.Minute,
	)

	filtered, err := serviceWithDismissal.ActiveWarnings(ctx)
	if err != nil {
		t.Fatalf("ActiveWarnings returned error: %v", err)
	}

	for _, warning := range filtered {
		if warning.ID == "system-health" {
			t.Fatal("expected dismissed system-health warning to be filtered")
		}
	}
}

func TestUpcomingTasksProjectionsHaveDescriptions(t *testing.T) {
	ctx := context.Background()

	syncReader := fakeSyncReader{
		rows: []postgresrepo.SyncRow{
			{
				Pair:                    "BTCEUR",
				HourlySyncedUnix:        time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Unix(),
				HourlyBackfillCompleted: false,
			},
			{
				Pair:                    "ETHUSD",
				HourlySyncedUnix:        time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Unix(),
				HourlyBackfillCompleted: true,
				DailyBackfillCompleted:  true,
			},
		},
	}

	svc := NewSystemService(
		fakePinger{},
		fakePinger{},
		syncReader,
		fakeTaskReader{},
		fakeWarningStore{},
		fakeLeaseReader{},
		"planner",
		5*time.Second,
	)

	tasks, _, err := svc.UpcomingTasks(ctx, 50, 0)
	if err != nil {
		t.Fatalf("UpcomingTasks returned error: %v", err)
	}

	if len(tasks) == 0 {
		t.Fatal("expected projected tasks, got none")
	}

	// Projected cleanup, backfill, integrity, and consolidation tasks should have descriptions.
	// Realtime tasks intentionally have empty descriptions.
	for _, item := range tasks {
		if item.Type == task.TypeRealtime {
			continue // intentionally empty
		}
		if item.Description == "" {
			t.Errorf("projected task %s (type=%s, pair=%s) has empty description", item.ID, item.Type, item.Pair)
		}
	}
}

type fakeTickerReader struct {
	tickers []ticker.Ticker
}

func (f fakeTickerReader) Tickers(context.Context) ([]ticker.Ticker, error) {
	return f.tickers, nil
}

func TestStaleTickerFeedWarning(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Unix()
	staleUnix := now - 600 // 10 minutes ago
	freshUnix := now - 60  // 1 minute ago

	svc := NewSystemService(
		fakePinger{},
		fakePinger{},
		fakeSyncReader{},
		fakeTaskReader{},
		fakeWarningStore{},
		fakeLeaseReader{},
		"planner",
		time.Minute,
		fakeTickerReader{
			tickers: []ticker.Ticker{
				{Pair: "BTCEUR", LastUpdateUnix: freshUnix},
				{Pair: "SOLEUR", LastUpdateUnix: staleUnix},
				{Pair: "SOLUSD", LastUpdateUnix: staleUnix},
			},
		},
	)

	warnings, err := svc.ActiveWarnings(ctx)
	if err != nil {
		t.Fatalf("ActiveWarnings returned error: %v", err)
	}

	var staleWarning *ActiveWarning
	for i, w := range warnings {
		if w.ID == "stale-ticker-feeds" {
			staleWarning = &warnings[i]
			break
		}
	}

	if staleWarning == nil {
		t.Fatal("expected stale-ticker-feeds warning to be present")
	}
	if staleWarning.Level != "warning" {
		t.Errorf("expected warning level, got %s", staleWarning.Level)
	}
	if !contains(staleWarning.Detail, "SOLEUR") || !contains(staleWarning.Detail, "SOLUSD") {
		t.Errorf("expected stale pairs in detail, got: %s", staleWarning.Detail)
	}
	if contains(staleWarning.Detail, "BTCEUR") {
		t.Errorf("fresh pair BTCEUR should not appear in stale warning detail: %s", staleWarning.Detail)
	}
}

func TestNoStaleTickerWarningWhenAllFresh(t *testing.T) {
	ctx := context.Background()
	freshUnix := time.Now().Unix() - 60

	svc := NewSystemService(
		fakePinger{},
		fakePinger{},
		fakeSyncReader{},
		fakeTaskReader{},
		fakeWarningStore{},
		fakeLeaseReader{},
		"planner",
		time.Minute,
		fakeTickerReader{
			tickers: []ticker.Ticker{
				{Pair: "BTCEUR", LastUpdateUnix: freshUnix},
				{Pair: "ETHEUR", LastUpdateUnix: freshUnix},
			},
		},
	)

	warnings, err := svc.ActiveWarnings(ctx)
	if err != nil {
		t.Fatalf("ActiveWarnings returned error: %v", err)
	}

	for _, w := range warnings {
		if w.ID == "stale-ticker-feeds" {
			t.Fatal("expected no stale-ticker-feeds warning when all tickers are fresh")
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestStreamSnapshotIncludesSyncStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	syncReader := fakeSyncReader{
		rows: []postgresrepo.SyncRow{
			{
				Pair:                    "BTCEUR",
				HourlyBackfillCompleted: true,
				DailyBackfillCompleted:  false,
				HourlySyncedUnix:        1700000000,
				DailySyncedUnix:         1700000000,
				BackfillCompleted:       false,
			},
		},
	}

	svc := NewSystemService(
		fakePinger{},
		fakePinger{},
		syncReader,
		fakeTaskReader{},
		fakeWarningStore{},
		fakeLeaseReader{},
		"planner",
		time.Minute,
	)

	ch := make(chan TaskStreamSnapshot, 1)
	go func() {
		_ = svc.StreamTasks(ctx, ch, 10, 10, nil, nil)
	}()

	select {
	case snap := <-ch:
		if len(snap.SyncStatus) != 1 {
			t.Fatalf("expected 1 sync status entry, got %d", len(snap.SyncStatus))
		}
		if snap.SyncStatus[0].Pair != "BTCEUR" {
			t.Fatalf("expected pair BTCEUR, got %s", snap.SyncStatus[0].Pair)
		}
		if !snap.SyncStatus[0].HourlyBackfillCompleted {
			t.Fatal("expected hourly backfill completed to be true")
		}
		if snap.SyncStatus[0].DailyBackfillCompleted {
			t.Fatal("expected daily backfill completed to be false")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for stream snapshot")
	}
}
