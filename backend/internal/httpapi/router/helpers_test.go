package router_test

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/asset"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/news"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/storage/postgres"
)

// noopRepo implements all repository interfaces used by the backend services
// with minimal, non-panicking implementations for testing.
type noopRepo struct{}

func (r *noopRepo) ReplaceCatalog(_ context.Context, _ []asset.Asset, _ []pair.Pair) error {
	return nil
}
func (r *noopRepo) ListAssets(_ context.Context) ([]asset.Asset, error) { return nil, nil }
func (r *noopRepo) ListPairs(_ context.Context) ([]pair.Pair, error)    { return nil, nil }
func (r *noopRepo) Historical(_ context.Context, _, _ string, _, _ time.Time) ([]candle.Candle, error) {
	return nil, nil
}
func (r *noopRepo) Ticker(_ context.Context, _ string) (ticker.Ticker, error) {
	return ticker.Ticker{}, nil
}
func (r *noopRepo) Tickers(_ context.Context) ([]ticker.Ticker, error)         { return nil, nil }
func (r *noopRepo) Ping(_ context.Context) error                               { return nil }
func (r *noopRepo) SnapshotRows(_ context.Context) ([]postgres.SyncRow, error) { return nil, nil }
func (r *noopRepo) UpcomingTasks(_ context.Context, _, _ int) ([]task.Task, int, error) {
	return nil, 0, nil
}
func (r *noopRepo) RecentTasks(_ context.Context, _, _ int, _, _ []string) ([]task.Task, int, error) {
	return nil, 0, nil
}
func (r *noopRepo) DismissWarning(_ context.Context, _, _ string) error            { return nil }
func (r *noopRepo) DismissedWarnings(_ context.Context) (map[string]string, error) { return nil, nil }
func (r *noopRepo) Current(_ context.Context, _ string) (lease.Lease, error) {
	return lease.Lease{}, nil
}
func (r *noopRepo) UpsertNews(_ context.Context, _ []news.News) error      { return nil }
func (r *noopRepo) ListNews(_ context.Context, _ int) ([]news.News, error) { return nil, nil }
