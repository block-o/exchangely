package worker

import (
	"context"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

type TaskStore interface {
	Claim(ctx context.Context, id string) (bool, error)
	Complete(ctx context.Context, id string) error
}

type UnlockFunc func() error

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

func (p *Processor) Process(ctx context.Context, item task.Task) error {
	claimed, err := p.store.Claim(ctx, item.ID)
	if err != nil || !claimed {
		return err
	}

	unlock, err := p.locker.Lock(ctx, item.Pair)
	if err != nil {
		return err
	}
	defer unlock()

	if err := p.executor.Execute(ctx, item); err != nil {
		return err
	}

	return p.store.Complete(ctx, item.ID)
}
