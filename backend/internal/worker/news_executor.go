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
	FetchSource(ctx context.Context, source string) error
}

// NewsExecutor handles the background execution of news_fetch tasks.
// Each task targets a specific RSS source identified by the task's Pair field.
type NewsExecutor struct {
	fetcher NewsFetcher
}

// NewNewsExecutor initializes a NewsExecutor with a fetcher.
func NewNewsExecutor(fetcher NewsFetcher) *NewsExecutor {
	return &NewsExecutor{fetcher: fetcher}
}

// Execute performs the news fetching task for a specific source.
func (e *NewsExecutor) Execute(ctx context.Context, item task.Task) error {
	if item.Type != task.TypeNewsFetch {
		return fmt.Errorf("news executor received unexpected task type %q", item.Type)
	}

	source := item.Pair
	if source == "" || source == "*" {
		// Legacy fallback: fetch all sources.
		slog.Info("news fetch task started (all sources)")
		if err := e.fetcher.FetchLatest(ctx); err != nil {
			slog.Warn("news fetch task failed", "error", err)
			return err
		}
		slog.Info("news fetch task completed (all sources)")
		return nil
	}

	slog.Info("news fetch task started", "source", source)
	if err := e.fetcher.FetchSource(ctx, source); err != nil {
		slog.Warn("news fetch task failed", "source", source, "error", err)
		return err
	}
	slog.Info("news fetch task completed", "source", source)
	return nil
}
