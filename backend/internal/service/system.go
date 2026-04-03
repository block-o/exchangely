package service

import (
	"context"
	"fmt"
	"log/slog"
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
	UpcomingTasks(ctx context.Context, limit, offset int) ([]task.Task, int, error)
	RecentTasks(ctx context.Context, limit, offset int, types, statuses []string) ([]task.Task, int, error)
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

type TaskStreamSnapshot struct {
	Upcoming      []task.Task `json:"upcoming"`
	UpcomingTotal int         `json:"upcomingTotal"`
	Recent        []task.Task `json:"recent"`
	RecentTotal   int         `json:"recentTotal"`
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

func (s *SystemService) UpcomingTasks(ctx context.Context, limit, offset int) ([]task.Task, int, error) {
	// 1. Fetch genuine pending tasks that have been emitted to the DB.
	dbTasks, total, err := s.taskReader.UpcomingTasks(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Build an index of pair+type combos that already have a real pending task.
	// Projections for those combos are suppressed — show the real thing, not the forecast.
	type pairType struct{ pair, taskType string }
	pendingIndex := make(map[pairType]bool, len(dbTasks))
	for _, t := range dbTasks {
		pendingIndex[pairType{t.Pair, t.Type}] = true
	}

	// Override interval for real tasks if they are live ticker.
	pollIntervalStr := formatDuration(s.pollInterval)
	for i := range dbTasks {
		if dbTasks[i].Type == task.TypeRealtime {
			dbTasks[i].Interval = pollIntervalStr
		}
	}

	// 2. Fetch the sync states so we can project future runs.
	rows, err := s.syncReader.SnapshotRows(ctx)
	if err != nil {
		return dbTasks, total, nil // gracefully fall back on DB tasks
	}

	// Only add projections if we are on the first page
	if offset == 0 {
		if !pendingIndex[pairType{"*", task.TypeCleanup}] {
			dayStart := time.Now().UTC().Truncate(24 * time.Hour)
			dbTasks = append(dbTasks, task.Task{
				ID:          "proj-cleanup",
				Type:        task.TypeCleanup,
				Pair:        "*",
				Interval:    "1d",
				Status:      "scheduled",
				WindowStart: dayStart,
				WindowEnd:   dayStart.Add(24 * time.Hour),
			})
		}

		for _, row := range rows {
			if !row.HourlyBackfillCompleted {
				// Skip projection if a real pending backfill already exists for this pair.
				if pendingIndex[pairType{row.Pair, task.TypeBackfill}] {
					continue
				}

				// Backfill projection.
				nextGapStart := row.HourlySyncedUnix
				if nextGapStart == 0 {
					nextGapStart = time.Now().UTC().AddDate(0, 0, -30).Truncate(time.Hour).Unix()
				}
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
				// Skip projection if a real pending realtime task already exists for this pair.
				if pendingIndex[pairType{row.Pair, task.TypeRealtime}] {
				} else {
					// Realtime projection: next poll boundary.
					pollBound := time.Now().UTC().Truncate(s.pollInterval).Add(s.pollInterval)
					windowEnd := pollBound.Add(time.Hour)

					dbTasks = append(dbTasks, task.Task{
						ID:          "proj-realtime-" + row.Pair,
						Type:        task.TypeRealtime,
						Pair:        row.Pair,
						Interval:    pollIntervalStr,
						Status:      "scheduled",
						WindowStart: pollBound,
						WindowEnd:   windowEnd,
					})
				}

				if !pendingIndex[pairType{row.Pair, task.TypeDataSanity}] {
					currentHour := time.Now().UTC().Truncate(time.Hour)
					prevHour := currentHour.Add(-time.Hour)
					dbTasks = append(dbTasks, task.Task{
						ID:          "proj-integrity-" + row.Pair,
						Type:        task.TypeDataSanity,
						Pair:        row.Pair,
						Interval:    "1h",
						Status:      "scheduled",
						WindowStart: prevHour,
						WindowEnd:   currentHour,
					})
				}

				if row.DailyBackfillCompleted && !pendingIndex[pairType{row.Pair, task.TypeConsolidate}] {
					currentDay := time.Now().UTC().Truncate(24 * time.Hour)
					prevDay := currentDay.Add(-24 * time.Hour)
					dbTasks = append(dbTasks, task.Task{
						ID:          "proj-consolidation-" + row.Pair,
						Type:        task.TypeConsolidate,
						Pair:        row.Pair,
						Interval:    "1d",
						Status:      "scheduled",
						WindowStart: prevDay,
						WindowEnd:   currentDay,
					})
				}
			}
		}
		// Note: total count doesn't include projections for simplicity, as they are synthetic
	}

	return dbTasks, total, nil
}

// formatDuration renders a time.Duration into a concise human-readable string
// like "2m", "30s", or "1h30m".
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0 && m == 0 && s == 0:
		return fmt.Sprintf("%dh", h)
	case h > 0 && s == 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	case m > 0 && s == 0:
		return fmt.Sprintf("%dm", m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func (s *SystemService) RecentTasks(ctx context.Context, limit, offset int, types, statuses []string) ([]task.Task, int, error) {
	tasks, total, err := s.taskReader.RecentTasks(ctx, limit, offset, types, statuses)
	if err != nil {
		return nil, 0, err
	}

	// Show poll interval for real ticker tasks
	pollIntervalStr := formatDuration(s.pollInterval)
	for i := range tasks {
		if tasks[i].Type == task.TypeRealtime {
			tasks[i].Interval = pollIntervalStr
		}
	}
	return tasks, total, nil
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

func (s *SystemService) StreamTasks(ctx context.Context, ch chan<- TaskStreamSnapshot, upcomingLimit, recentLimit int, recentTypes, recentStatuses []string) error {
	slog.Info("SSE client subscribed to task stream")

	// Initial push for page 1 using the caller's requested limits/filters.
	upcoming, upcomingTotal, _ := s.UpcomingTasks(ctx, upcomingLimit, 0)
	recent, recentTotal, _ := s.RecentTasks(ctx, recentLimit, 0, recentTypes, recentStatuses)
	select {
	case ch <- TaskStreamSnapshot{
		Upcoming:      upcoming,
		UpcomingTotal: upcomingTotal,
		Recent:        recent,
		RecentTotal:   recentTotal,
	}:
	default:
	}

	update := make(chan struct{}, 1)
	s.mu.Lock()
	s.updateChs = append(s.updateChs, update)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		for i, c := range s.updateChs {
			if c == update {
				s.updateChs = append(s.updateChs[:i], s.updateChs[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		slog.Info("SSE client disconnected")
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-update:
			upcoming, upcomingTotal, _ := s.UpcomingTasks(ctx, upcomingLimit, 0)
			recent, recentTotal, _ := s.RecentTasks(ctx, recentLimit, 0, recentTypes, recentStatuses)
			select {
			case ch <- TaskStreamSnapshot{
				Upcoming:      upcoming,
				UpcomingTotal: upcomingTotal,
				Recent:        recent,
				RecentTotal:   recentTotal,
			}:
			default:
			}
		case <-time.After(3 * time.Second): // Poll internal state if no updates
			upcoming, upcomingTotal, _ := s.UpcomingTasks(ctx, upcomingLimit, 0)
			recent, recentTotal, _ := s.RecentTasks(ctx, recentLimit, 0, recentTypes, recentStatuses)
			select {
			case ch <- TaskStreamSnapshot{
				Upcoming:      upcoming,
				UpcomingTotal: upcomingTotal,
				Recent:        recent,
				RecentTotal:   recentTotal,
			}:
			default:
			}
		}
	}
}

func (s *SystemService) Health(ctx context.Context) HealthStatus {
	checks := map[string]string{
		"api": "ok",
	}

	if err := s.dbChecker.Ping(ctx); err != nil {
		checks["db"] = "error"
	} else {
		checks["db"] = "ok"
	}

	if err := s.kafkaChecker.Ping(ctx); err != nil {
		checks["kafka"] = "error"
	} else {
		checks["kafka"] = "ok"
	}

	status := "ok"
	for _, v := range checks {
		if v != "ok" {
			status = "degraded"
			break
		}
	}

	return HealthStatus{
		Status:    status,
		Checks:    checks,
		Timestamp: time.Now().Unix(),
	}
}

func (s *SystemService) CurrentPlannerLeader(ctx context.Context) (lease.Lease, error) {
	return s.leaseReader.Current(ctx, s.leaseName)
}

func (s *SystemService) SyncSnapshot(ctx context.Context) ([]syncstatus.PairSyncStatus, error) {
	rows, err := s.syncReader.SnapshotRows(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]syncstatus.PairSyncStatus, 0, len(rows))
	for _, row := range rows {
		result = append(result, syncstatus.PairSyncStatus{
			Pair:                    row.Pair,
			HourlySyncedUnix:        row.HourlySyncedUnix,
			DailySyncedUnix:         row.DailySyncedUnix,
			HourlyBackfillCompleted: row.HourlyBackfillCompleted,
			DailyBackfillCompleted:  row.DailyBackfillCompleted,
		})
	}
	return result, nil
}
