package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/coordinator"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	healthkafka "github.com/block-o/exchangely/backend/internal/messaging/kafka"
	"github.com/block-o/exchangely/backend/internal/planner"
	"github.com/block-o/exchangely/backend/internal/service"
	healthpostgres "github.com/block-o/exchangely/backend/internal/storage/postgres"
	"github.com/block-o/exchangely/backend/internal/worker"
)

type App struct {
	server *http.Server
}

func New(cfg config.Config) *App {
	instanceID, _ := os.Hostname()
	if instanceID == "" {
		instanceID = "exchangely-local"
	}

	catalog := service.NewCatalogService(cfg.DefaultQuoteAssets)
	market := service.NewMarketService()
	system := service.NewSystemService(
		healthpostgres.NewHealthChecker(cfg.DatabaseURL),
		healthkafka.NewHealthChecker(cfg.KafkaBrokers),
		catalog,
	)

	leaseManager := coordinator.NewLeaseManager(instanceID, cfg.PlannerLeaseName, cfg.PlannerLeaseTTL)
	system.SetPlannerLeader(leaseManager.CurrentLease().HolderID)

	_ = planner.NewScheduler()
	_ = worker.NewProcessor(noopStore{}, noopLocker{}, noopExecutor{})

	handler := router.New(router.Services{
		Catalog: catalog,
		Market:  market,
		System:  system,
	})

	return &App{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("server failed: %w", err)
	}
}

type noopStore struct{}

func (noopStore) Claim(context.Context, string) (bool, error) { return true, nil }
func (noopStore) Complete(context.Context, string) error      { return nil }

type noopLocker struct{}

func (noopLocker) Lock(context.Context, string) (worker.UnlockFunc, error) {
	return func() error { return nil }, nil
}

type noopExecutor struct{}

func (noopExecutor) Execute(_ context.Context, task task.Task) error { return nil }
