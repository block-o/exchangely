package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/block-o/exchangely/backend/internal/domain/task"
)

// NewsFetcher defines the interface for fetching news in the background.
type NewsFetcher interface {
	FetchLatest(ctx context.Context) error
}

// NewsExecutor handles the background execution of news_fetch tasks.
type NewsExecutor struct {
	fetcher NewsFetcher
}

// NewNewsExecutor initializes a NewsExecutor with a fetcher.
func NewNewsExecutor(fetcher NewsFetcher) *NewsExecutor {
	return &NewsExecutor{fetcher: fetcher}
}

// Execute performs the news fetching task.
func (e *NewsExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeNewsFetch {
		return fmt.Errorf("news executor received unexpected task type %q", item.Type)
	}

	slog.Info("news fetch task started")
	if err := e.fetcher.FetchLatest(ctx); err != nil {
		slog.Warn("news fetch task failed", "error", err)
		return err
	}
	slog.Info("news fetch task completed")
	return nil
}
