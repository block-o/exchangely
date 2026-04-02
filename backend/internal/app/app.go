package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	"github.com/block-o/exchangely/backend/internal/ingest/binance"
	"github.com/block-o/exchangely/backend/internal/ingest/kraken"
	ingestregistry "github.com/block-o/exchangely/backend/internal/ingest/registry"
	kafka "github.com/block-o/exchangely/backend/internal/messaging/kafka"
	"github.com/block-o/exchangely/backend/internal/planner"
	"github.com/block-o/exchangely/backend/internal/service"
	healthpostgres "github.com/block-o/exchangely/backend/internal/storage/postgres"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
	"github.com/block-o/exchangely/backend/internal/worker"
)

type App struct {
	server        *http.Server
	db            *sql.DB
	taskPublisher *kafka.TaskPublisher
	planRunner    *planner.Runner
	workerRunner  *worker.Runner
	role          string
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	instanceID, _ := os.Hostname()
	if instanceID == "" {
		instanceID = "exchangely-local"
	}

	db, err := postgresrepo.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := postgresrepo.Migrate(ctx, db, migrationsDir()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	catalogRepo := postgresrepo.NewCatalogRepository(db)
	catalogService := service.NewCatalogService(catalogRepo, cfg.DefaultQuoteAssets)
	if err := catalogService.Seed(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed catalog: %w", err)
	}

	marketRepo := postgresrepo.NewMarketRepository(db)
	syncRepo := postgresrepo.NewSyncRepository(db)
	leaseRepo := postgresrepo.NewLeaseRepository(db)
	taskRepo := postgresrepo.NewTaskRepository(db, instanceID)
	pairLocker := postgresrepo.NewAdvisoryPairLocker(db)

	systemService := service.NewSystemService(
		healthpostgres.NewHealthChecker(cfg.DatabaseURL),
		kafka.NewHealthChecker(cfg.KafkaBrokers),
		syncRepo,
		leaseRepo,
		cfg.PlannerLeaseName,
	)
	marketService := service.NewMarketService(marketRepo)

	handler := router.New(router.Services{
		Catalog: catalogService,
		Market:  marketService,
		System:  systemService,
	})

	taskPublisher := kafka.NewTaskPublisher(cfg.KafkaBrokers, cfg.KafkaTasksTopic)
	sourceRegistry := ingestregistry.New(
		kraken.NewClient("", nil),
		binance.NewClient("", nil),
	)
	plannerRunner := planner.NewRunner(
		instanceID,
		cfg.PlannerLeaseName,
		cfg.PlannerLeaseTTL,
		cfg.PlannerTick,
		planner.NewScheduler(),
		catalogRepo,
		syncRepo,
		leaseRepo,
		taskRepo,
		taskPublisher,
	)

	workerProcessor := worker.NewProcessor(
		taskRepo,
		pairLocker,
		worker.NewBackfillExecutor(marketRepo, syncRepo, sourceRegistry),
	)
	workerRunner := worker.NewRunner(taskRepo, workerProcessor, cfg.WorkerPollInterval, cfg.WorkerBatchSize)

	return &App{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		db:            db,
		taskPublisher: taskPublisher,
		planRunner:    plannerRunner,
		workerRunner:  workerRunner,
		role:          cfg.Role,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 3)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if hasRole(a.role, "planner") {
		go func() {
			if err := a.planRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("planner loop: %w", err)
			}
		}()
	}

	if hasRole(a.role, "worker") {
		go func() {
			if err := a.workerRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("worker loop: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		return a.shutdown()
	case err := <-errCh:
		_ = a.shutdown()
		return err
	}
}

func (a *App) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var errs []error
	if err := a.server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs = append(errs, err)
	}
	if a.taskPublisher != nil {
		if err := a.taskPublisher.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		log.Printf("shutdown errors: %v", errs)
		return errs[0]
	}

	return nil
}

func hasRole(configured, desired string) bool {
	if configured == "" || configured == "all" {
		return true
	}

	for _, item := range strings.Split(configured, ",") {
		if strings.TrimSpace(item) == desired {
			return true
		}
	}

	return false
}

func migrationsDir() string {
	if _, err := os.Stat("migrations"); err == nil {
		return "migrations"
	}
	return "backend/migrations"
}
