package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	"github.com/block-o/exchangely/backend/internal/ingest"
	"github.com/block-o/exchangely/backend/internal/ingest/binance"
	"github.com/block-o/exchangely/backend/internal/ingest/binancevision"
	"github.com/block-o/exchangely/backend/internal/ingest/coingecko"
	"github.com/block-o/exchangely/backend/internal/ingest/cryptodatadownload"
	"github.com/block-o/exchangely/backend/internal/ingest/kraken"
	ingestregistry "github.com/block-o/exchangely/backend/internal/ingest/registry"
	kafka "github.com/block-o/exchangely/backend/internal/messaging/kafka"
	"github.com/block-o/exchangely/backend/internal/planner"
	"github.com/block-o/exchangely/backend/internal/service"
	postgresrepo "github.com/block-o/exchangely/backend/internal/storage/postgres"
	"github.com/block-o/exchangely/backend/internal/worker"
)

type App struct {
	server          *http.Server
	db              *sql.DB
	taskPublisher   *kafka.TaskPublisher
	taskConsumer    *kafka.TaskConsumer
	marketPublisher *kafka.MarketEventPublisher
	marketConsumer  *kafka.MarketEventConsumer
	planRunner      *planner.Runner
	workerRunner    *worker.Runner
	instanceID      string
	corsOriginCount int
	role            string
	enabledSources  []string
}

type sourceSet struct {
	registrySources  []ingest.Source
	validatorSources []ingest.Source
	enabledNames     []string
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
		_ = db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	catalogRepo := postgresrepo.NewCatalogRepository(db)
	catalogService := service.NewCatalogService(catalogRepo, cfg.DefaultQuoteAssets)
	if err := catalogService.Seed(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed catalog: %w", err)
	}

	marketRepo := postgresrepo.NewMarketRepository(db)
	syncRepo := postgresrepo.NewSyncRepository(db)
	leaseRepo := postgresrepo.NewLeaseRepository(db)
	taskRepo := postgresrepo.NewTaskRepository(db, instanceID)
	warningDismissalRepo := postgresrepo.NewWarningDismissalRepository(db)
	pairLocker := postgresrepo.NewAdvisoryPairLocker(db)

	systemService := service.NewSystemService(
		postgresrepo.NewHealthChecker(cfg.DatabaseURL),
		kafka.NewHealthChecker(cfg.KafkaBrokers),
		syncRepo,
		taskRepo,
		warningDismissalRepo,
		leaseRepo,
		cfg.PlannerLeaseName,
		cfg.RealtimePollInterval,
	)
	taskRepo.SetNotifier(systemService)
	marketService := service.NewMarketService(marketRepo)

	handler := router.New(router.Services{
		Catalog: catalogService,
		Market:  marketService,
		System:  systemService,
	}, router.Options{
		AllowedOrigins: cfg.CORSAllowedOrigins,
	})

	taskPublisher := kafka.NewTaskPublisher(cfg.KafkaBrokers, cfg.KafkaTasksTopic)
	marketPublisher := kafka.NewMarketEventPublisher(cfg.KafkaBrokers, cfg.KafkaMarketTopic)
	sources := buildSources(cfg)
	sourceRegistry := ingestregistry.New(sources.registrySources...)
	realtimeIngest := service.NewRealtimeIngestService(marketRepo, marketService)
	plannerRunner := planner.NewRunner(
		instanceID,
		cfg.PlannerLeaseName,
		cfg.PlannerLeaseTTL,
		cfg.PlannerTick,
		planner.NewScheduler(cfg.RealtimePollInterval),
		catalogRepo,
		syncRepo,
		leaseRepo,
		taskRepo,
		taskPublisher,
	)

	backfillExe := worker.NewBackfillExecutor(marketRepo, syncRepo, sourceRegistry, marketPublisher, marketService)
	validatorExe := worker.NewValidatorExecutor(sources.validatorSources, worker.ValidatorOptions{
		MinSources:       cfg.IntegrityMinSources,
		MaxDivergencePct: cfg.IntegrityMaxDivergencePct,
	})
	cleanupExe := worker.NewCleanupExecutor(taskRepo, 7*24*time.Hour) // Retain 7 days of logs

	routerExe := worker.NewRouterExecutor(map[string]worker.Executor{
		task.TypeBackfill:    backfillExe,
		task.TypeRealtime:    backfillExe,
		task.TypeConsolidate: backfillExe,
		task.TypeDataSanity:  validatorExe,
		task.TypeCleanup:     cleanupExe,
	})

	workerProcessor := worker.NewProcessor(
		taskRepo,
		pairLocker,
		routerExe,
	)
	taskConsumer := kafka.NewTaskConsumer(
		cfg.KafkaBrokers,
		cfg.KafkaTasksTopic,
		consumerGroup(cfg.KafkaConsumerGroup, "tasks"),
		workerProcessor,
	)
	workerRunner := worker.NewRunner(taskRepo, workerProcessor, cfg.WorkerPollInterval, cfg.WorkerBatchSize)
	marketConsumer := kafka.NewMarketEventConsumer(
		cfg.KafkaBrokers,
		cfg.KafkaMarketTopic,
		consumerGroup(cfg.KafkaConsumerGroup, "market"),
		realtimeIngest,
	)

	return &App{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		db:              db,
		taskPublisher:   taskPublisher,
		taskConsumer:    taskConsumer,
		marketPublisher: marketPublisher,
		marketConsumer:  marketConsumer,
		planRunner:      plannerRunner,
		workerRunner:    workerRunner,
		instanceID:      instanceID,
		corsOriginCount: len(cfg.CORSAllowedOrigins),
		role:            cfg.Role,
		enabledSources:  append([]string(nil), sources.enabledNames...),
	}, nil
}

func buildSources(cfg config.Config) sourceSet {
	sources := sourceSet{
		registrySources:  make([]ingest.Source, 0, 5),
		validatorSources: make([]ingest.Source, 0, 3),
		enabledNames:     make([]string, 0, 5),
	}

	if cfg.EnableBinanceVision {
		source := binancevision.NewClient("", nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.validatorSources = append(sources.validatorSources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}
	if cfg.EnableCryptoDataDownload {
		source := cryptodatadownload.NewClient("", nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}
	if cfg.EnableKraken {
		source := kraken.NewClient("", nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.validatorSources = append(sources.validatorSources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}
	if cfg.EnableCoinGecko {
		source := coingecko.NewClient("", cfg.CoinGeckoAPIKey, nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}
	if cfg.EnableBinance {
		source := binance.NewClient("", nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.validatorSources = append(sources.validatorSources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}

	return sources
}

func (a *App) Run(ctx context.Context) error {
	slog.Info("backend runtime starting",
		"instance_id", a.instanceID,
		"http_addr", a.server.Addr,
		"role", effectiveRole(a.role),
		"planner_enabled", hasRole(a.role, "planner"),
		"worker_enabled", hasRole(a.role, "worker"),
		"cors_allowed_origins", a.corsOriginCount,
		"enabled_sources", a.enabledSources,
	)

	errCh := make(chan error, 5)

	go func() {
		slog.Info("http server listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if hasRole(a.role, "planner") {
		slog.Info("planner loop enabled")
		go func() {
			if err := a.planRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("planner loop: %w", err)
			}
		}()
	}

	if hasRole(a.role, "worker") {
		slog.Info("worker loops enabled")
		go func() {
			if err := a.taskConsumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("task consumer loop: %w", err)
			}
		}()
		go func() {
			if err := a.workerRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("worker loop: %w", err)
			}
		}()
		go func() {
			if err := a.marketConsumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("market consumer loop: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		slog.Info("shutdown requested", "reason", ctx.Err())
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
	if a.taskConsumer != nil {
		if err := a.taskConsumer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.marketPublisher != nil {
		if err := a.marketPublisher.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.marketConsumer != nil {
		if err := a.marketConsumer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		slog.Error("shutdown completed with errors", "count", len(errs), "error", errs[0])
		return errs[0]
	}

	slog.Info("shutdown complete", "instance_id", a.instanceID)
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

func consumerGroup(base, suffix string) string {
	if strings.TrimSpace(base) == "" {
		base = "exchangely-workers"
	}
	return base + "-" + suffix
}

func effectiveRole(role string) string {
	if strings.TrimSpace(role) == "" {
		return "all"
	}
	return role
}
