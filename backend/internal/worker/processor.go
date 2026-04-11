package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type TaskStore interface {
	Claim(ctx context.Context, id string) (bool, error)
	Complete(ctx context.Context, id string) error
	Fail(ctx context.Context, id, reason string) error
}

type UnlockFunc = func() error

type PairLocker interface {
	Lock(ctx context.Context, pair string) (UnlockFunc, error)
}

type Executor interface {
	Execute(ctx context.Context, task task.Task) error
}

type Processor struct {
	store    TaskStore
	locker   PairLocker
	executor Executor
}

func NewProcessor(store TaskStore, locker PairLocker, executor Executor) *Processor {
	return &Processor{
		store:    store,
		locker:   locker,
		executor: executor,
	}
}

type RouterExecutor struct {
	routers map[string]Executor
}

func NewRouterExecutor(routers map[string]Executor) *RouterExecutor {
	return &RouterExecutor{routers: routers}
}

func (r *RouterExecutor) Execute(ctx context.Context, item task.Task) error {
	executor, ok := r.routers[item.Type]
	if !ok {
		return fmt.Errorf("no executor registered for task type %q", item.Type)
	}
	return executor.Execute(ctx, item)
}

func (p *Processor) Process(ctx context.Context, item task.Task) error {
	claimed, err := p.store.Claim(ctx, item.ID)
	if err != nil || !claimed {
		return err
	}
	startedAt := time.Now()

	unlock, err := p.locker.Lock(ctx, item.Pair)
	if err != nil {
		return err
	}
	defer func() {
		_ = unlock()
	}()

	slog.Info("worker task started",
		"task_id", item.ID,
		"type", item.Type,
		"pair", item.Pair,
		"interval", item.Interval,
		"window_start", item.WindowStart.UTC().Format(time.RFC3339),
		"window_end", item.WindowEnd.UTC().Format(time.RFC3339),
	)

	if err := p.executor.Execute(ctx, item); err != nil {
		_ = p.store.Fail(ctx, item.ID, err.Error())
		return err
	}

	if err := p.store.Complete(ctx, item.ID); err != nil {
		return err
	}

	slog.Info("worker task completed",
		"task_id", item.ID,
		"type", item.Type,
		"pair", item.Pair,
		"interval", item.Interval,
		"window_start", item.WindowStart.UTC().Format(time.RFC3339),
		"window_end", item.WindowEnd.UTC().Format(time.RFC3339),
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	return nil
}

type PendingSource interface {
	Pending(ctx context.Context, limit, backfillLimit int) ([]task.Task, error)
}

type Runner struct {
	source                   PendingSource
	processor                *Processor
	interval                 time.Duration
	batchSize                int
	maxBackfillTasksPerBatch int
	concurrency              int
}

func NewRunner(source PendingSource, processor *Processor, interval time.Duration, batchSize int, maxBackfillTasksPerBatch int, concurrency int) *Runner {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Runner{
		source:                   source,
		processor:                processor,
		interval:                 interval,
		batchSize:                batchSize,
		maxBackfillTasksPerBatch: maxBackfillTasksPerBatch,
		concurrency:              concurrency,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		if err := r.runBatch(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) runBatch(ctx context.Context) error {
	items, err := r.source.Pending(ctx, r.batchSize, r.maxBackfillTasksPerBatch)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	slog.Info("worker batch fetched", "task_count", len(items), "concurrency", r.concurrency)

	if r.concurrency <= 1 {
		for _, item := range items {
			if err := r.processor.Process(ctx, item); err != nil {
				slog.Warn("worker task failed", "task_id", item.ID, "pair", item.Pair, "interval", item.Interval, "error", err)
			}
		}
		return nil
	}

	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(t task.Task) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := r.processor.Process(ctx, t); err != nil {
				slog.Warn("worker task failed", "task_id", t.ID, "pair", t.Pair, "interval", t.Interval, "error", err)
			}
		}(item)
	}

	wg.Wait()
	return nil
}
