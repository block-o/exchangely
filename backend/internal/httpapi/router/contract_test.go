package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/asset"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/lease"
	"github.com/block-o/exchangely/backend/internal/domain/news"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	"github.com/block-o/exchangely/backend/internal/service"
	"github.com/block-o/exchangely/backend/internal/storage/postgres"
	"gopkg.in/yaml.v3"
)

// noopRepo implements all repository interfaces used by the backend services
// with minimal, non-panicking implementations for contract testing.
type noopRepo struct{}

func (r *noopRepo) ReplaceCatalog(ctx context.Context, assets []asset.Asset, pairs []pair.Pair) error {
	return nil
}
func (r *noopRepo) ListAssets(ctx context.Context) ([]asset.Asset, error) { return nil, nil }
func (r *noopRepo) ListPairs(ctx context.Context) ([]pair.Pair, error)    { return nil, nil }
func (r *noopRepo) Historical(ctx context.Context, p, i string, s, e time.Time) ([]candle.Candle, error) {
	return nil, nil
}
func (r *noopRepo) Ticker(ctx context.Context, p string) (ticker.Ticker, error) {
	return ticker.Ticker{}, nil
}
func (r *noopRepo) Tickers(ctx context.Context) ([]ticker.Ticker, error) { return nil, nil }
func (r *noopRepo) Ping(ctx context.Context) error                       { return nil }
func (r *noopRepo) SnapshotRows(ctx context.Context) ([]postgres.SyncRow, error) {
	return nil, nil
}
func (r *noopRepo) UpcomingTasks(ctx context.Context, l, o int) ([]task.Task, int, error) {
	return nil, 0, nil
}
func (r *noopRepo) RecentTasks(ctx context.Context, l, o int, t, s []string) ([]task.Task, int, error) {
	return nil, 0, nil
}
func (r *noopRepo) DismissWarning(ctx context.Context, w, f string) error { return nil }
func (r *noopRepo) DismissedWarnings(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (r *noopRepo) Current(ctx context.Context, name string) (lease.Lease, error) {
	return lease.Lease{}, nil
}
func (r *noopRepo) UpsertNews(ctx context.Context, i []news.News) error      { return nil }
func (r *noopRepo) ListNews(ctx context.Context, l int) ([]news.News, error) { return nil, nil }

func TestRouterHasNoUndocumentedPaths(t *testing.T) {
	// 1. Load and parse openapi.yaml
	path := filepath.Join("..", "..", "..", "..", "docs", "openapi", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var spec struct {
		Paths map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal openapi.yaml: %v", err)
	}

	// 2. Exhaustive list of paths we KNOW are registered in the router.
	// This serves as the source of truth for the reverse check since http.ServeMux
	// is not easily introspectable.
	knownPaths := []string{
		"/api/v1/health",
		"/api/v1/assets",
		"/api/v1/pairs",
		"/api/v1/historical/{pair}",
		"/api/v1/ticker/{pair}",
		"/api/v1/tickers",
		"/api/v1/tickers/stream",
		"/api/v1/system/sync-status",
		"/api/v1/system/version",
		"/api/v1/system/tasks",
		"/api/v1/system/tasks/stream",
		"/api/v1/system/warnings",
		"/api/v1/news",
		"/api/v1/news/stream",
	}

	for _, p := range knownPaths {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("Path %q is registered in router but MISSING from openapi.yaml documentation", p)
		}
	}
}

func TestOpenAPIContractSync(t *testing.T) {
	// 1. Load and parse openapi.yaml
	path := filepath.Join("..", "..", "..", "..", "docs", "openapi", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var spec struct {
		Paths map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to unmarshal openapi.yaml: %v", err)
	}

	// 2. Initialize router with dummy services
	// We use the real New functions with nil/dummy dependencies to avoid panics.
	nr := &noopRepo{}
	svcs := router.Services{
		Catalog: service.NewCatalogService(nr, nil),
		Market:  service.NewMarketService(nr, 100, 30*time.Second),
		System:  service.NewSystemService(nr, nr, nr, nr, nr, nr, "leader", 0),
		News:    service.NewNewsService(nr),
	}
	h := router.New(svcs, router.Options{})

	// 3. Verify each path documented in the spec exists in the router
	for apiPath := range spec.Paths {
		t.Run(apiPath, func(t *testing.T) {
			// Precompute a safe URL for the path
			// Replace {pair} with a dummy value
			testURL := strings.ReplaceAll(apiPath, "{pair}", "BTCEUR")

			// Use a context with timeout to avoid blocking on SSE streams
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			req := httptest.NewRequest("GET", testURL, nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			// If the route is missing, it will return 404.
			// Note: Status 0 or some other error because of timeout is also fine,
			// as long as it's not 404.
			if rec.Code == http.StatusNotFound {
				t.Errorf("Path %q documented in openapi.yaml but not registered in router (returned 404)", apiPath)
			}
		})
	}
}
