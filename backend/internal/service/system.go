package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/syncstatus"
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

type SystemService struct {
	dbChecker    Pinger
	kafkaChecker Pinger
	syncReader   SyncReader
	leaseName    string
	leaseReader  LeaseReader
}

type HealthStatus struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks"`
	Timestamp int64             `json:"timestamp"`
}

func NewSystemService(dbChecker, kafkaChecker Pinger, syncReader SyncReader, leaseReader LeaseReader, leaseName string) *SystemService {
	return &SystemService{
		dbChecker:    dbChecker,
		kafkaChecker: kafkaChecker,
		syncReader:   syncReader,
		leaseName:    leaseName,
		leaseReader:  leaseReader,
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
			Pair:              row.Pair,
			BackfillCompleted: row.BackfillCompleted,
			LastSyncedUnix:    row.LastSyncedUnix,
			NextTargetUnix:    row.NextTargetUnix,
		})
	}

	return syncstatus.Snapshot{
		PlannerLeader: holder,
		Pairs:         items,
	}, nil
}
