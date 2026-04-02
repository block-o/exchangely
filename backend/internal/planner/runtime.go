package planner

import (
	"context"
	"log"
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

type TaskSink interface {
	Enqueue(ctx context.Context, tasks []task.Task) error
}

type TaskPublisher interface {
	Publish(ctx context.Context, tasks []task.Task) error
}

type Runner struct {
	instanceID string
	leaseName  string
	leaseTTL   time.Duration
	interval   time.Duration
	scheduler  *Scheduler
	pairs      PairProvider
	sync       SyncStateProvider
	leases     LeaseCoordinator
	tasks      TaskSink
	publisher  TaskPublisher
}

func NewRunner(
	instanceID, leaseName string,
	leaseTTL, interval time.Duration,
	scheduler *Scheduler,
	pairs PairProvider,
	sync SyncStateProvider,
	leases LeaseCoordinator,
	tasks TaskSink,
	publisher TaskPublisher,
) *Runner {
	return &Runner{
		instanceID: instanceID,
		leaseName:  leaseName,
		leaseTTL:   leaseTTL,
		interval:   interval,
		scheduler:  scheduler,
		pairs:      pairs,
		sync:       sync,
		leases:     leases,
		tasks:      tasks,
		publisher:  publisher,
	}
}

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
		return nil
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
		adapted[symbol] = SyncState{LastSynced: state.LastSynced}
	}

	for _, trackedPair := range pairs {
		if _, ok := states[trackedPair.Symbol]; !ok {
			if err := r.sync.MarkBackfillSeeded(ctx, trackedPair.Symbol, time.Time{}); err != nil {
				return err
			}
		}
	}

	tasks := r.scheduler.BuildInitialBackfillTasks(pairs, adapted, time.Now().UTC())
	if len(tasks) == 0 {
		return nil
	}

	if err := r.tasks.Enqueue(ctx, tasks); err != nil {
		return err
	}

	if r.publisher != nil {
		if err := r.publisher.Publish(ctx, tasks); err != nil {
			log.Printf("planner task publish degraded: %v", err)
		}
	}

	return nil
}
