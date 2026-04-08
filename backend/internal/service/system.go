package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
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

type WarningDismissalStore interface {
	DismissWarning(ctx context.Context, warningID, fingerprint string) error
	DismissedWarnings(ctx context.Context) (map[string]string, error)
}

type SystemService struct {
	dbChecker    Pinger
	kafkaChecker Pinger
	syncReader   SyncReader
	taskReader   SystemTaskReader
	warningStore WarningDismissalStore
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

type ActiveWarning struct {
	ID          string `json:"id"`
	Level       string `json:"level"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Fingerprint string `json:"fingerprint"`
	Timestamp   int64  `json:"timestamp,omitempty"`
}

func NewSystemService(dbChecker, kafkaChecker Pinger, syncReader SyncReader, taskReader SystemTaskReader, warningStore WarningDismissalStore, leaseReader LeaseReader, leaseName string, pollInterval time.Duration) *SystemService {
	return &SystemService{
		dbChecker:    dbChecker,
		kafkaChecker: kafkaChecker,
		syncReader:   syncReader,
		taskReader:   taskReader,
		warningStore: warningStore,
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
		projectedCount := 0
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
			projectedCount++
		}

		for _, row := range rows {
			nextGapStart := row.HourlySyncedUnix
			if nextGapStart == 0 {
				nextGapStart = time.Now().UTC().AddDate(0, 0, -30).Truncate(time.Hour).Unix()
			}
			hourlyCutover := time.Now().UTC().Truncate(time.Hour).Unix()
			if row.HourlyRealtimeStartedUnix > 0 && row.HourlyRealtimeStartedUnix < hourlyCutover {
				hourlyCutover = row.HourlyRealtimeStartedUnix
			}

			if !row.HourlyBackfillCompleted {
				if !pendingIndex[pairType{row.Pair, task.TypeBackfill}] && nextGapStart < hourlyCutover {
					nextGapEnd := minInt64(nextGapStart+3600, hourlyCutover)
					dbTasks = append(dbTasks, task.Task{
						ID:          "proj-backfill-" + row.Pair,
						Type:        task.TypeBackfill,
						Pair:        row.Pair,
						Interval:    "1h",
						Status:      "scheduled",
						WindowStart: time.Unix(nextGapStart, 0).UTC(),
						WindowEnd:   time.Unix(nextGapEnd, 0).UTC(),
					})
					projectedCount++
				}
			}

			// Realtime projection: next poll boundary.
			if !pendingIndex[pairType{row.Pair, task.TypeRealtime}] {
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
				projectedCount++
			}

			currentHour := time.Now().UTC().Truncate(time.Hour)
			prevHour := currentHour.Add(-time.Hour)
			if !pendingIndex[pairType{row.Pair, task.TypeDataSanity}] &&
				(row.HourlyBackfillCompleted || (row.HourlyRealtimeStartedUnix > 0 && prevHour.Unix() >= row.HourlyRealtimeStartedUnix)) {
				dbTasks = append(dbTasks, task.Task{
					ID:          "proj-integrity-" + row.Pair,
					Type:        task.TypeDataSanity,
					Pair:        row.Pair,
					Interval:    "1h",
					Status:      "scheduled",
					WindowStart: prevHour,
					WindowEnd:   currentHour,
				})
				projectedCount++
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
				projectedCount++
			}
		}

		total += projectedCount
		if limit > 0 && len(dbTasks) > limit {
			dbTasks = dbTasks[:limit]
		}
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

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
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

func (s *SystemService) ActiveWarnings(ctx context.Context) ([]ActiveWarning, error) {
	health := s.Health(ctx)
	syncPairs, err := s.SyncSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	failedTasks, _, err := s.RecentTasks(ctx, 10, 0, nil, []string{"failed"})
	if err != nil {
		return nil, err
	}

	warnings := buildActiveWarnings(health, syncPairs, failedTasks)
	if s.warningStore == nil {
		return warnings, nil
	}

	dismissed, err := s.warningStore.DismissedWarnings(ctx)
	if err != nil {
		return nil, err
	}

	filtered := warnings[:0]
	for _, warning := range warnings {
		if dismissed[warning.ID] == warning.Fingerprint {
			continue
		}
		filtered = append(filtered, warning)
	}

	return filtered, nil
}

func (s *SystemService) DismissWarning(ctx context.Context, warningID, fingerprint string) error {
	if warningID == "" || fingerprint == "" || s.warningStore == nil {
		return nil
	}

	return s.warningStore.DismissWarning(ctx, warningID, fingerprint)
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

func buildActiveWarnings(health HealthStatus, syncPairs []syncstatus.PairSyncStatus, failedTasks []task.Task) []ActiveWarning {
	warnings := make([]ActiveWarning, 0, 5)

	if health.Status != "ok" {
		failingChecks := make([]string, 0, len(health.Checks))
		for name, status := range health.Checks {
			if status != "ok" {
				failingChecks = append(failingChecks, name)
			}
		}
		sort.Strings(failingChecks)

		detail := "One or more system health checks are failing."
		if len(failingChecks) > 0 {
			detail = fmt.Sprintf("Failing checks: %s.", joinStrings(failingChecks))
		}

		warnings = append(warnings, newWarning("system-health", "error", "System health degraded", detail))
	}

	for _, pair := range syncPairs {
		if !pair.HourlyBackfillCompleted {
			warnings = append(warnings, newWarning(
				fmt.Sprintf("hourly-backfill-%s", pair.Pair),
				"warning",
				"Hourly backfill pending",
				fmt.Sprintf("%s is still filling hourly history.", pair.Pair),
			))
		}
	}

	for _, pair := range syncPairs {
		if pair.HourlyBackfillCompleted && !pair.DailyBackfillCompleted {
			warnings = append(warnings, newWarning(
				fmt.Sprintf("daily-backfill-%s", pair.Pair),
				"warning",
				"Daily backfill pending",
				fmt.Sprintf("%s is not ready for consolidation yet.", pair.Pair),
			))
		}
	}

	for _, failedTask := range failedTasks {
		var ts int64
		if failedTask.CompletedAt != nil {
			ts = failedTask.CompletedAt.Unix()
		}

		if failedTask.Type == task.TypeDataSanity {
			detail := fmt.Sprintf("Integrity check failed for %s", failedTask.Pair)
			if failedTask.LastError != "" {
				detail = fmt.Sprintf("%s (%s)", detail, truncateText(failedTask.LastError, 110))
			}
			warnings = append(warnings, newWarning(
				fmt.Sprintf("integrity-failure-%s", failedTask.ID),
				"error",
				"Integrity check failure",
				detail,
				ts,
			))
		} else {
			detail := fmt.Sprintf("%s failed", taskTypeLabel(failedTask.Type))
			if failedTask.Pair != "" && failedTask.Pair != "*" {
				detail = fmt.Sprintf("%s for %s", detail, failedTask.Pair)
			}
			if failedTask.LastError != "" {
				detail = fmt.Sprintf("%s (%s)", detail, truncateText(failedTask.LastError, 110))
			}
			warnings = append(warnings, newWarning(
				fmt.Sprintf("task-failure-%s", failedTask.ID),
				"warning",
				"Task failure",
				detail,
				ts,
			))
		}
	}

	// Sort all warnings by timestamp descending (newest first), then by level (errors before warnings).
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Timestamp != warnings[j].Timestamp {
			return warnings[i].Timestamp > warnings[j].Timestamp
		}
		if warnings[i].Level != warnings[j].Level {
			return warnings[i].Level == "error"
		}
		return warnings[i].ID < warnings[j].ID
	})

	return warnings
}

func newWarning(id, level, title, detail string, timestamp ...int64) ActiveWarning {
	sum := sha256.Sum256([]byte(id + "|" + level + "|" + title + "|" + detail))
	var ts int64
	if len(timestamp) > 0 {
		ts = timestamp[0]
	}
	return ActiveWarning{
		ID:          id,
		Level:       level,
		Title:       title,
		Detail:      detail,
		Fingerprint: hex.EncodeToString(sum[:]),
		Timestamp:   ts,
	}
}

func truncateText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit-1] + "…"
}

func joinStrings(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	default:
		result := values[0]
		for _, value := range values[1:] {
			result += ", " + value
		}
		return result
	}
}

func taskTypeLabel(taskType string) string {
	switch taskType {
	case task.TypeBackfill:
		return "Historical Backfill"
	case task.TypeRealtime:
		return "Live Ticker"
	case task.TypeDataSanity:
		return "Integrity Check"
	case task.TypeConsolidate:
		return "Consolidation"
	case task.TypeCleanup:
		return "Task Log Cleanup"
	default:
		return taskType
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
