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
	updateChs    []chan struct{}
	mu           sync.RWMutex
}

type HealthStatus struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks"`
	Timestamp int64             `json:"timestamp"`
}

func NewSystemService(dbChecker, kafkaChecker Pinger, syncReader SyncReader, taskReader SystemTaskReader, leaseReader LeaseReader, leaseName string) *SystemService {
	return &SystemService{
		dbChecker:    dbChecker,
		kafkaChecker: kafkaChecker,
		syncReader:   syncReader,
		taskReader:   taskReader,
		leaseName:    leaseName,
		leaseReader:  leaseReader,
		updateChs:    make([]chan struct{}, 0),
	}
}

func (s *SystemService) UpcomingTasks(ctx context.Context, limit int) ([]task.Task, error) {
	return s.taskReader.UpcomingTasks(ctx, limit)
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
	} else if err != nil && err != sql.ErrNoRows {
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
