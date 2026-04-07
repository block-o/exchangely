package worker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/worker"
)

type mockNewsFetcher struct {
	err error
}

func (m *mockNewsFetcher) FetchLatest(ctx context.Context) error {
	return m.err
}

func TestNewsExecutor_Execute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fetcher := &mockNewsFetcher{}
		exec := worker.NewNewsExecutor(fetcher)

		err := exec.Execute(context.Background(), task.Task{Type: task.TypeNewsFetch})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		fetcher := &mockNewsFetcher{err: errors.New("failed")}
		exec := worker.NewNewsExecutor(fetcher)

		err := exec.Execute(context.Background(), task.Task{Type: task.TypeNewsFetch})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		fetcher := &mockNewsFetcher{}
		exec := worker.NewNewsExecutor(fetcher)

		err := exec.Execute(context.Background(), task.Task{Type: task.TypeBackfill})
		if err == nil {
			t.Error("expected error for wrong type, got nil")
		}
	})
}
