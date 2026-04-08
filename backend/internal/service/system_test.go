package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/task"
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
