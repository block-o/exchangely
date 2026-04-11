package planner

import (
	"context"
	"log/slog"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
)

type PairProvider interface {
	ListPairs(ctx context.Context) ([]pair.Pair, error)
}

type SyncStateProvider interface {
	States(ctx context.Context) (map[string]postgresrepo.SyncState, error)
	MarkBackfillSeeded(ctx context.Context, pairSymbol string, lastSynced time.Time) error
}

type LeaseCoordinator interface {
	AcquireOrRenew(ctx context.Context, name, holder string, ttl time.Duration) (lease.Lease, bool, error)
}

type CoverageProvider interface {
	GetAllCompletedDays(ctx context.Context) (map[string]map[string]bool, error)
}

type IntegrityCoverageProvider interface {
	GetAllVerifiedDays(ctx context.Context) (map[string]map[string]bool, error)
}

type TaskSink interface {
	Enqueue(ctx context.Context, tasks []task.Task) ([]task.Task, error)
	ActiveBackfillPairs(ctx context.Context) (map[string]bool, error)
}

type TaskPublisher interface {
	Publish(ctx context.Context, tasks []task.Task) error
}

// Runner owns planner leadership and periodically turns catalog + sync state into executable tasks.
type Runner struct {
	instanceID              string
	leaseName               string
	leaseTTL                time.Duration
	interval                time.Duration
	scheduler               *Scheduler
	maxBackfillTasksPerTick int
	pairs                   PairProvider
	sync                    SyncStateProvider
	leases                  LeaseCoordinator
	tasks                   TaskSink
	coverage                CoverageProvider
	integrity               IntegrityCoverageProvider
	publisher               TaskPublisher
	isLeader                bool
}

// NewRunner wires the planner runtime with the scheduler, state stores, and optional Kafka publisher.
func NewRunner(
	instanceID, leaseName string,
	leaseTTL, interval time.Duration,
	scheduler *Scheduler,
	maxBackfillTasksPerTick int,
	pairs PairProvider,
	sync SyncStateProvider,
	leases LeaseCoordinator,
	tasks TaskSink,
	coverage CoverageProvider,
	integrity IntegrityCoverageProvider,
	publisher TaskPublisher,
) *Runner {
	return &Runner{
		instanceID:              instanceID,
		leaseName:               leaseName,
		leaseTTL:                leaseTTL,
		interval:                interval,
		scheduler:               scheduler,
		maxBackfillTasksPerTick: maxBackfillTasksPerTick,
		pairs:                   pairs,
		sync:                    sync,
		leases:                  leases,
		tasks:                   tasks,
		coverage:                coverage,
		integrity:               integrity,
		publisher:               publisher,
	}
}

// Run keeps renewing the planner lease and scheduling work until the context is canceled.
func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		if err := r.runTick(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) runTick(ctx context.Context) error {
	_, acquired, err := r.leases.AcquireOrRenew(ctx, r.leaseName, r.instanceID, r.leaseTTL)
	if err != nil {
		return err
	}
	if !acquired {
		if r.isLeader {
			slog.Info("planner leadership lost", "instance_id", r.instanceID, "lease_name", r.leaseName)
			r.isLeader = false
		}
		return nil
	}
	if !r.isLeader {
		slog.Info("planner leadership acquired", "instance_id", r.instanceID, "lease_name", r.leaseName)
		r.isLeader = true
	}

	pairs, err := r.pairs.ListPairs(ctx)
	if err != nil {
		return err
	}

	states, err := r.sync.States(ctx)
	if err != nil {
		return err
	}

	adapted := make(map[string]SyncState, len(states))
	for symbol, state := range states {
		adapted[symbol] = SyncState{
			HourlyLastSynced:        state.HourlyLastSynced,
			HourlyRealtimeStartedAt: state.HourlyRealtimeStartedAt,
			DailyLastSynced:         state.DailyLastSynced,
			HourlyBackfillCompleted: state.HourlyBackfillCompleted,
			DailyBackfillCompleted:  state.DailyBackfillCompleted,
		}
	}

	for _, trackedPair := range pairs {
		if _, ok := states[trackedPair.Symbol]; !ok {
			if err := r.sync.MarkBackfillSeeded(ctx, trackedPair.Symbol, time.Time{}); err != nil {
				return err
			}
			adapted[trackedPair.Symbol] = SyncState{}
		}
	}

	coverage, err := r.coverage.GetAllCompletedDays(ctx)
	if err != nil {
		return err
	}

	integrityCov, err := r.integrity.GetAllVerifiedDays(ctx)
	if err != nil {
		return err
	}

	activePairs, err := r.tasks.ActiveBackfillPairs(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	realtimeTasks := r.scheduler.BuildRealtimeTasks(pairs, adapted, now)
	if err := r.enqueueBatch(ctx, realtimeTasks); err != nil {
		return err
	}

	backfillTasks := r.scheduler.BuildInitialBackfillTasksLimited(pairs, adapted, coverage, activePairs, now, r.maxBackfillTasksPerTick)
	if r.maxBackfillTasksPerTick > 0 && len(backfillTasks) == r.maxBackfillTasksPerTick {
		slog.Info("planner capped backfill task batch",
			"instance_id", r.instanceID,
			"backfill_task_limit", r.maxBackfillTasksPerTick,
		)
	}

	followUpTasks := make([]task.Task, 0, len(backfillTasks)+8)
	followUpTasks = append(followUpTasks, backfillTasks...)
	followUpTasks = append(followUpTasks, r.scheduler.BuildBackfillProbeTasks(pairs, adapted, now)...)
	followUpTasks = append(followUpTasks, r.scheduler.BuildGapValidationTasks(pairs, adapted, coverage, now)...)
	followUpTasks = append(followUpTasks, r.scheduler.BuildIntegrityCheckTasks(pairs, adapted, integrityCov, now)...)
	followUpTasks = append(followUpTasks, r.scheduler.BuildConsolidationTasks(pairs, adapted, now)...)
	followUpTasks = append(followUpTasks, r.scheduler.BuildCleanupTask(now))
	followUpTasks = append(followUpTasks, r.scheduler.BuildNewsFetchTasks(now)...)

	if len(realtimeTasks) == 0 && len(followUpTasks) == 0 {
		slog.Debug("planner tick complete", "instance_id", r.instanceID, "pair_count", len(pairs), "task_count", 0)
		return nil
	}

	if err := r.enqueueBatch(ctx, followUpTasks); err != nil {
		return err
	}

	return nil
}

func (r *Runner) enqueueBatch(ctx context.Context, tasks []task.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	enqueuedTasks, err := r.tasks.Enqueue(ctx, tasks)
	if err != nil {
		return err
	}
	if len(enqueuedTasks) == 0 {
		return nil
	}

	if r.publisher != nil {
		if err := r.publisher.Publish(ctx, enqueuedTasks); err != nil {
			slog.Warn("planner task publish degraded", "error", err, "task_count", len(enqueuedTasks))
		}
	}

	for _, item := range enqueuedTasks {
		slog.Info("planner scheduled task",
			"instance_id", r.instanceID,
			"task_id", item.ID,
			"type", item.Type,
			"pair", item.Pair,
			"interval", item.Interval,
			"window_start", item.WindowStart.UTC().Format(time.RFC3339),
			"window_end", item.WindowEnd.UTC().Format(time.RFC3339),
		)
	}

	return nil
}

func ComputeBackfillTaskCap(workerBatchSize, percent int) int {
	if workerBatchSize <= 0 || percent <= 0 {
		return 0
	}
	if percent > 100 {
		percent = 100
	}

	cap := (workerBatchSize*percent + 99) / 100
	if cap > workerBatchSize {
		return workerBatchSize
	}
	if cap < 1 {
		return 1
	}
	return cap
}
