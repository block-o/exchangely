package service

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/syncstatus"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type SystemService struct {
	dbChecker     Pinger
	kafkaChecker  Pinger
	catalog       *CatalogService
	plannerLeader string
}

type HealthStatus struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks"`
	Timestamp int64             `json:"timestamp"`
}

func NewSystemService(dbChecker, kafkaChecker Pinger, catalog *CatalogService) *SystemService {
	return &SystemService{
		dbChecker:     dbChecker,
		kafkaChecker:  kafkaChecker,
		catalog:       catalog,
		plannerLeader: "unknown",
	}
}

func (s *SystemService) SetPlannerLeader(holder string) {
	s.plannerLeader = holder
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

func (s *SystemService) SyncStatus() syncstatus.Snapshot {
	pairs := s.catalog.Pairs()
	items := make([]syncstatus.PairSyncStatus, 0, min(6, len(pairs)))
	now := time.Now().UTC()

	for index, trackedPair := range pairs {
		if index >= 6 {
			break
		}
		items = append(items, syncstatus.PairSyncStatus{
			Pair:              trackedPair.Symbol,
			BackfillCompleted: index%2 == 0,
			LastSyncedUnix:    now.Add(-time.Duration(index) * time.Hour).Unix(),
			NextTargetUnix:    now.Add(time.Hour).Unix(),
		})
	}

	return syncstatus.Snapshot{
		PlannerLeader: s.plannerLeader,
		Pairs:         items,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Symbols(pairs []pair.Pair) []string {
	out := make([]string, 0, len(pairs))
	for _, item := range pairs {
		out = append(out, item.Symbol)
	}
	return out
}
