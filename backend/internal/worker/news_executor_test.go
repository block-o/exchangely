package worker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/worker"
)

type mockNewsFetcher struct {
	err         error
	lastSource  string
	fetchCalled bool
}

func (m *mockNewsFetcher) FetchLatest(ctx context.Context) error {
	m.fetchCalled = true
	return m.err
}

func (m *mockNewsFetcher) FetchSource(ctx context.Context, source string) error {
	m.lastSource = source
	return m.err
}

func TestNewsExecutor_Execute(t *testing.T) {
	t.Run("per-source success", func(t *testing.T) {
		fetcher := &mockNewsFetcher{}
		exec := worker.NewNewsExecutor(fetcher)

		err := exec.Execute(context.Background(), task.Task{Type: task.TypeNewsFetch, Pair: "coindesk"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if fetcher.lastSource != "coindesk" {
			t.Errorf("expected source coindesk, got %q", fetcher.lastSource)
		}
	})

	t.Run("legacy global fallback", func(t *testing.T) {
		fetcher := &mockNewsFetcher{}
		exec := worker.NewNewsExecutor(fetcher)

		err := exec.Execute(context.Background(), task.Task{Type: task.TypeNewsFetch, Pair: "*"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if !fetcher.fetchCalled {
			t.Error("expected FetchLatest to be called for global task")
		}
	})

	t.Run("failure", func(t *testing.T) {
		fetcher := &mockNewsFetcher{err: errors.New("failed")}
		exec := worker.NewNewsExecutor(fetcher)

		err := exec.Execute(context.Background(), task.Task{Type: task.TypeNewsFetch, Pair: "coindesk"})
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
