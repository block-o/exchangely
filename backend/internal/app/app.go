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

	"github.com/block-o/exchangely/backend/internal/auth"
	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/domain/task"
	"github.com/block-o/exchangely/backend/internal/httpapi/router"
	"github.com/block-o/exchangely/backend/internal/ingest/binance"
	"github.com/block-o/exchangely/backend/internal/ingest/binancevision"
	"github.com/block-o/exchangely/backend/internal/ingest/coingecko"
	"github.com/block-o/exchangely/backend/internal/ingest/cryptodatadownload"
	"github.com/block-o/exchangely/backend/internal/ingest/kraken"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
	"github.com/block-o/exchangely/backend/internal/ingest/realtime"
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

// sourceSet keeps the runtime wiring explicit: the registry uses ordered fallback,
// while validation only receives providers trusted for cross-source comparisons.
type sourceSet struct {
	registrySources  []provider.Source
	validatorSources []provider.Source
	enabledNames     []string
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	// Validate auth configuration before proceeding.
	if errs := cfg.ValidateAuthConfig(); len(errs) > 0 {
		for _, e := range errs {
			slog.Error("auth config error", "error", e)
		}
		return nil, fmt.Errorf("invalid auth configuration: %s", strings.Join(errs, "; "))
	}

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
	newsRepo := postgresrepo.NewNewsRepository(db)
	coverageRepo := postgresrepo.NewCoverageRepository(db)
	integrityRepo := postgresrepo.NewIntegrityRepository(db)

	newsService := service.NewNewsService(newsRepo)

	// Auth service — conditionally enabled when BACKEND_AUTH_MODE is set.
	var authService *auth.Service
	var apiTokenService *auth.APITokenService
	var apiRateLimiter *auth.APIRateLimiter
	var adminUserService *auth.AdminUserService
	if cfg.AuthEnabled() {
		userRepo := postgresrepo.NewUserRepository(db)
		sessionRepo := postgresrepo.NewSessionRepository(db)
		authCfg := auth.Config{
			AuthMode:           cfg.AuthMode,
			GoogleClientID:     cfg.GoogleClientID,
			GoogleClientSecret: cfg.GoogleClientSecret,
			GoogleRedirectURI:  cfg.GoogleRedirectURI,
			JWTSecret:          []byte(cfg.JWTSecret),
			JWTExpiry:          cfg.JWTExpiry,
			RefreshTokenExpiry: cfg.RefreshTokenExpiry,
			BcryptCost:         cfg.BcryptCost,
			AdminEmail:         cfg.AdminEmail,
			Env:                cfg.Env,
		}
		authService = auth.NewService(userRepo, sessionRepo, authCfg)

		if cfg.AuthModeHasLocal() {
			if err := authService.BootstrapAdmin(ctx); err != nil {
				slog.Error("admin bootstrap failed", "error", err)
				// Non-fatal: the app can still run without the admin account.
			}
		}

		// API token service and rate limiter.
		apiTokenRepo := postgresrepo.NewAPITokenRepository(db)
		rateLimitRepo := postgresrepo.NewRateLimitRepository(db)

		apiTokenService = auth.NewAPITokenService(apiTokenRepo, userRepo, auth.DefaultAPITokenConfig())
		apiRateLimiter = auth.NewAPIRateLimiter(rateLimitRepo, auth.APIRateLimitConfig{
			UserLimit:    cfg.RateLimitUser,
			PremiumLimit: cfg.RateLimitPremium,
			AdminLimit:   cfg.RateLimitAdmin,
			IPLimit:      cfg.RateLimitIP,
			Window:       cfg.RateLimitWindow,
		})
		apiRateLimiter.StartPruner(ctx)

		// Admin user management service.
		adminUserService = auth.NewAdminUserService(userRepo, sessionRepo)

		slog.Info("auth enabled", "mode", cfg.AuthMode)
	} else {
		slog.Info("auth disabled — BACKEND_AUTH_MODE not set")
	}

	systemService := service.NewSystemService(
		postgresrepo.NewHealthChecker(cfg.DatabaseURL),
		kafka.NewHealthChecker(cfg.KafkaBrokers),
		syncRepo,
		taskRepo,
		warningDismissalRepo,
		leaseRepo,
		cfg.PlannerLeaseName,
		cfg.RealtimePollInterval,
		marketRepo,
	)
	taskRepo.SetNotifier(systemService)
	marketService := service.NewMarketService(marketRepo, cfg.TickerCacheSize, cfg.TickersCacheTTL)

	handler := router.New(router.Services{
		Catalog:          catalogService,
		Market:           marketService,
		System:           systemService,
		News:             newsService,
		Auth:             authService,
		APITokenService:  apiTokenService,
		APIRateLimiter:   apiRateLimiter,
		AdminUserService: adminUserService,
	}, router.Options{
		AllowedOrigins: cfg.CORSAllowedOrigins,
		Env:            cfg.Env,
		AuthMode:       cfg.AuthMode,
		TrustedProxies: cfg.TrustedProxies,
		APIBaseURL:     cfg.APIBaseURL,
	})

	taskPublisher := kafka.NewTaskPublisher(cfg.KafkaBrokers, cfg.KafkaTasksTopic)
	marketPublisher := kafka.NewMarketEventPublisher(cfg.KafkaBrokers, cfg.KafkaMarketTopic)
	sources := buildSources(cfg)
	sourceRegistry := provider.NewRegistry(sources.registrySources...)
	realtimeIngest := realtime.NewIngestService(marketRepo, marketService)
	plannerRunner := planner.NewRunner(
		instanceID,
		cfg.PlannerLeaseName,
		cfg.PlannerLeaseTTL,
		cfg.PlannerTick,
		planner.NewScheduler(cfg.RealtimePollInterval, cfg.NewsFetchInterval),
		planner.ComputeBackfillTaskCap(cfg.WorkerBatchSize, cfg.PlannerBackfillBatchPct),
		catalogRepo,
		syncRepo,
		leaseRepo,
		taskRepo,
		coverageRepo,
		integrityRepo,
		taskPublisher,
	)

	backfillExe := worker.NewBackfillExecutor(marketRepo, syncRepo, sourceRegistry.WithCapability(provider.CapHistorical), marketService)
	realtimeExe := worker.NewRealtimeExecutor(marketRepo, syncRepo, sourceRegistry.WithCapability(provider.CapRealtime), marketPublisher, marketService)
	validatorExe := worker.NewValidatorExecutor(sources.validatorSources, integrityRepo, integrityRepo, worker.ValidatorOptions{
		MinSources:       cfg.IntegrityMinSources,
		MaxDivergencePct: cfg.IntegrityMaxDivergencePct,
	})
	validatorExe.SetResultWriter(&integrityResultAdapter{repo: integrityRepo})
	cleanupExe := worker.NewCleanupExecutor(taskRepo, cfg.TaskRetentionPeriod, cfg.TaskRetentionCount)
	gapValidatorExe := worker.NewGapValidatorExecutor(marketRepo, coverageRepo, coverageRepo)

	routerExe := worker.NewRouterExecutor(map[string]worker.Executor{
		task.TypeBackfill:      backfillExe,
		task.TypeRealtime:      realtimeExe,
		task.TypeConsolidate:   backfillExe,
		task.TypeDataSanity:    validatorExe,
		task.TypeCleanup:       cleanupExe,
		task.TypeGapValidation: gapValidatorExe,
		task.TypeNewsFetch:     worker.NewNewsExecutor(newsService),
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
	workerRunner := worker.NewRunner(
		taskRepo,
		workerProcessor,
		cfg.WorkerPollInterval,
		cfg.WorkerBatchSize,
		planner.ComputeBackfillTaskCap(cfg.WorkerBatchSize, cfg.WorkerBackfillBatchPct),
		cfg.WorkerConcurrency,
	)
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

// buildSources translates provider flags into the two ingest paths:
// registry fallback and validator peer comparison.
func buildSources(cfg config.Config) sourceSet {
	sources := sourceSet{
		registrySources:  make([]provider.Source, 0, 5),
		validatorSources: make([]provider.Source, 0, 3),
		enabledNames:     make([]string, 0, 5),
	}

	if cfg.EnableBinanceVision {
		source := binancevision.NewClient("", nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.validatorSources = append(sources.validatorSources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}
	if cfg.EnableCryptoDataDownload {
		source := cryptodatadownload.NewClient("", cfg.CDDAvailabilityBaseURL, nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.enabledNames = append(sources.enabledNames, source.Name())
	}
	if cfg.EnableBinance {
		source := binance.NewClient("", nil)
		sources.registrySources = append(sources.registrySources, source)
		sources.validatorSources = append(sources.validatorSources, source)
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

// integrityResultAdapter bridges the worker.IntegrityResultWriter interface
// to the postgres IntegrityRepository, mapping between the two IntegrityResult types.
type integrityResultAdapter struct {
	repo *postgresrepo.IntegrityRepository
}

func (a *integrityResultAdapter) RecordResult(ctx context.Context, r worker.IntegrityResult) error {
	return a.repo.RecordResult(ctx, postgresrepo.IntegrityResult{
		PairSymbol:      r.PairSymbol,
		Day:             r.Day,
		Verified:        r.Verified,
		GapCount:        r.GapCount,
		DivergenceCount: r.DivergenceCount,
		SourcesChecked:  r.SourcesChecked,
		ErrorMessage:    r.ErrorMessage,
	})
}
