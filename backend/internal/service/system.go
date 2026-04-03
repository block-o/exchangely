package service

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/syncstatus"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type SyncReader interface {
	SnapshotRows(ctx context.Context) ([]postgresrepo.SyncRow, error)
}

type LeaseReader interface {
	Current(ctx context.Context, name string) (lease.Lease, error)
}

type SystemTaskReader interface {
	UpcomingTasks(ctx context.Context, limit int) ([]task.Task, error)
	RecentTasks(ctx context.Context, limit int) ([]task.Task, error)
}

type SystemService struct {
	dbChecker    Pinger
	kafkaChecker Pinger
	syncReader   SyncReader
	taskReader   SystemTaskReader
	leaseName    string
	leaseReader  LeaseReader
	pollInterval time.Duration
	updateChs    []chan struct{}
	mu           sync.RWMutex
}

type HealthStatus struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks"`
	Timestamp int64             `json:"timestamp"`
}

func NewSystemService(dbChecker, kafkaChecker Pinger, syncReader SyncReader, taskReader SystemTaskReader, leaseReader LeaseReader, leaseName string, pollInterval time.Duration) *SystemService {
	return &SystemService{
		dbChecker:    dbChecker,
		kafkaChecker: kafkaChecker,
		syncReader:   syncReader,
		taskReader:   taskReader,
		leaseName:    leaseName,
		leaseReader:  leaseReader,
		pollInterval: pollInterval,
		updateChs:    make([]chan struct{}, 0),
	}
}

func (s *SystemService) UpcomingTasks(ctx context.Context, limit int) ([]task.Task, error) {
	// 1. Fetch genuine pending tasks that have been emitted to the DB.
	dbTasks, err := s.taskReader.UpcomingTasks(ctx, limit)
	if err != nil {
		return nil, err
	}

	// 2. Fetch the sync states so we can project future runs.
	rows, err := s.syncReader.SnapshotRows(ctx)
	if err != nil {
		return dbTasks, nil // gracefully fall back on DB tasks
	}

	for _, row := range rows {
		if !row.HourlyBackfillCompleted {
			// Backfill projection.
			// The next execution is the start of the un-synced window.
			// It begins precisely after the LastSynced (or initial configured boundary).
			nextGapStart := row.HourlySyncedUnix
			if nextGapStart == 0 {
				nextGapStart = time.Now().UTC().AddDate(0, 0, -30).Truncate(time.Hour).Unix()
			}
			
			// Compute the next target segment (1 hour stride).
			nextGapEnd := nextGapStart + 3600
			
			dbTasks = append(dbTasks, task.Task{
				ID:          "proj-backfill-" + row.Pair,
				Type:        task.TypeBackfill,
				Pair:        row.Pair,
				Interval:    "1h",
				Status:      "scheduled",
				WindowStart: time.Unix(nextGapStart, 0).UTC(),
				WindowEnd:   time.Unix(nextGapEnd, 0).UTC(),
			})
		} else {
			// Realtime projection.
			// Scheduled exactly for the next poll interval bound.
			pollBound := time.Now().UTC().Truncate(s.pollInterval).Add(s.pollInterval)
			windowEnd := pollBound.Add(time.Hour)

			dbTasks = append(dbTasks, task.Task{
				ID:          "proj-realtime-" + row.Pair,
				Type:        task.TypeRealtime,
				Pair:        row.Pair,
				Interval:    "1h",
				Status:      "scheduled",
				WindowStart: pollBound,
				WindowEnd:   windowEnd,
			})
		}
	}

	return dbTasks, nil
}

func (s *SystemService) RecentTasks(ctx context.Context, limit int) ([]task.Task, error) {
	return s.taskReader.RecentTasks(ctx, limit)
}

func (s *SystemService) NotifyUpdate() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.updateChs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *SystemService) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateChs = append(s.updateChs, ch)
	return ch
}

func (s *SystemService) Unsubscribe(ch <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.updateChs {
		if c == ch {
			s.updateChs = append(s.updateChs[:i], s.updateChs[i+1:]...)
			return
		}
	}
}

func (s *SystemService) Health(ctx context.Context) HealthStatus {
	checks := map[string]string{
		"api": "ok",
	}

	status := "ok"
	if err := s.dbChecker.Ping(ctx); err != nil {
		checks["timescaledb"] = "degraded"
		status = "degraded"
	} else {
		checks["timescaledb"] = "ok"
	}

	if err := s.kafkaChecker.Ping(ctx); err != nil {
		checks["kafka"] = "degraded"
		status = "degraded"
	} else {
		checks["kafka"] = "ok"
	}

	return HealthStatus{
		Status:    status,
		Checks:    checks,
		Timestamp: time.Now().UTC().Unix(),
	}
}

func (s *SystemService) SyncStatus(ctx context.Context) (syncstatus.Snapshot, error) {
	rows, err := s.syncReader.SnapshotRows(ctx)
	if err != nil {
		return syncstatus.Snapshot{}, err
	}

	holder := "unknown"
	if current, err := s.leaseReader.Current(ctx, s.leaseName); err == nil {
		holder = current.HolderID
	} else if err != sql.ErrNoRows {
		return syncstatus.Snapshot{}, err
	}

	items := make([]syncstatus.PairSyncStatus, 0, len(rows))
	for _, row := range rows {
		items = append(items, syncstatus.PairSyncStatus{
			Pair:                    row.Pair,
			BackfillCompleted:       row.BackfillCompleted,
			LastSyncedUnix:          row.LastSyncedUnix,
			NextTargetUnix:          row.NextTargetUnix,
			HourlyBackfillCompleted: row.HourlyBackfillCompleted,
			DailyBackfillCompleted:  row.DailyBackfillCompleted,
			HourlySyncedUnix:        row.HourlySyncedUnix,
			DailySyncedUnix:         row.DailySyncedUnix,
			NextHourlyTargetUnix:    row.NextHourlyTargetUnix,
			NextDailyTargetUnix:     row.NextDailyTargetUnix,
		})
	}

	return syncstatus.Snapshot{
		PlannerLeader: holder,
		Pairs:         items,
	}, nil
}
