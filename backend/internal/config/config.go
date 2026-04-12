package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                      string
	HTTPAddr                 string
	Role                     string
	LogLevel                 string
	CORSAllowedOrigins       []string
	DatabaseURL              string
	KafkaBrokers             []string
	KafkaTasksTopic          string
	KafkaMarketTopic         string
	KafkaConsumerGroup       string
	EnableBinance            bool
	EnableKraken             bool
	EnableBinanceVision      bool
	EnableCryptoDataDownload bool
	EnableCoinGecko          bool
	CDDAvailabilityBaseURL   string
	PlannerLeaseName         string
	PlannerLeaseTTL          time.Duration
	PlannerTick              time.Duration
	// RealtimePollInterval controls how frequently the planner emits fresh realtime
	// tasks for caught-up pairs. Shorter intervals mean fresher ticker prices but
	// also increase external API load and internal task volume.
	RealtimePollInterval    time.Duration
	WorkerPollInterval      time.Duration
	WorkerBatchSize         int
	PlannerBackfillBatchPct int
	WorkerBackfillBatchPct  int
	// WorkerConcurrency controls how many tasks within a single batch are
	// processed in parallel. Pair-level advisory locks still prevent concurrent
	// mutations on the same trading pair. A value of 1 preserves the original
	// sequential behavior.
	WorkerConcurrency         int
	CoinGeckoAPIKey           string
	IntegrityMinSources       int
	IntegrityMaxDivergencePct float64
	DefaultQuoteAssets        []string
	// TaskRetentionPeriod defines how long completed and failed tasks are kept
	// in the task repository before being pruned.
	TaskRetentionPeriod time.Duration
	// TaskRetentionCount defines the maximum number of completed and failed
	// tasks kept per cleanup cycle, further pruning the log if it exceeds this count.
	TaskRetentionCount int
	// TickerCacheSize defines the maximum number of individual tickers to keep in the memory cache.
	TickerCacheSize int
	// TickersCacheTTL defines how long the global tickers snapshot is kept in memory.
	TickersCacheTTL time.Duration
	// NewsFetchInterval defines how often the worker should fetch news from RSS feeds.
	NewsFetchInterval time.Duration
	// IntegrityCheckInterval defines how often integrity check sweep tasks are scheduled per pair.
	IntegrityCheckInterval time.Duration
	// GapValidationInterval defines how often gap validation sweep tasks are scheduled per pair.
	GapValidationInterval time.Duration

	// Auth configuration.
	// AuthMode controls which authentication methods are active.
	// Valid values: "" (disabled), "local", "sso", "local,sso" (both).
	// When empty, auth is completely disabled and all endpoints are public.
	AuthMode           string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	JWTSecret          string
	JWTExpiry          time.Duration
	RefreshTokenExpiry time.Duration
	AdminEmail         string
	BcryptCost         int

	// API rate limiting configuration.
	RateLimitUser    int           // BACKEND_RATELIMIT_USER, default: 100
	RateLimitPremium int           // BACKEND_RATELIMIT_PREMIUM, default: 500
	RateLimitAdmin   int           // BACKEND_RATELIMIT_ADMIN, default: 1000
	RateLimitIP      int           // BACKEND_RATELIMIT_IP, default: 200
	RateLimitWindow  time.Duration // BACKEND_RATELIMIT_WINDOW, default: 1m

	// TrustedProxies is a comma-separated list of CIDR ranges or IP addresses
	// whose X-Forwarded-For / X-Real-IP headers are trusted for extracting the
	// original client IP. When empty, r.RemoteAddr is always used.
	TrustedProxies []string

	// APIBaseURL is the public base URL for the API (e.g. http://localhost:8080/api/v1).
	// Shared with the frontend via the same API_BASE_URL env var. Used in the
	// OpenAPI spec server URL and Swagger UI configuration.
	APIBaseURL string

	// Portfolio configuration.
	PortfolioEnabled       bool   // BACKEND_PORTFOLIO_ENABLED, default: false
	PortfolioEncryptionKey string // BACKEND_PORTFOLIO_ENCRYPTION_KEY, hex-encoded 32-byte AES-256 master key
	EtherscanAPIKey        string // BACKEND_ETHERSCAN_API_KEY
	SolanaRPCURL           string // BACKEND_SOLANA_RPC_URL, default: https://api.mainnet-beta.solana.com
	BitcoinAPIURL          string // BACKEND_BITCOIN_API_URL, default: https://blockstream.info/api
}

func Load() Config {
	env := getenv("BACKEND_ENV", "development")

	return Config{
		Env:                       env,
		HTTPAddr:                  getenv("BACKEND_HTTP_ADDR", ":8080"),
		Role:                      getenv("BACKEND_ROLE", "all"),
		LogLevel:                  getenv("BACKEND_LOG_LEVEL", "info"),
		CORSAllowedOrigins:        corsOrigins(env),
		DatabaseURL:               getenv("BACKEND_DATABASE_URL", ""),
		KafkaBrokers:              splitCSV(getenv("BACKEND_KAFKA_BROKERS", "")),
		KafkaTasksTopic:           getenv("BACKEND_KAFKA_TOPIC_TASKS", "exchangely.tasks"),
		KafkaMarketTopic:          getenv("BACKEND_KAFKA_TOPIC_MARKET_TICKS", "exchangely.market.ticks"),
		KafkaConsumerGroup:        getenv("BACKEND_KAFKA_CONSUMER_GROUP", "exchangely-workers"),
		EnableBinance:             parseBool(getenv("BACKEND_ENABLE_BINANCE", "true"), true),
		EnableKraken:              parseBool(getenv("BACKEND_ENABLE_KRAKEN", "true"), true),
		EnableBinanceVision:       parseBool(getenv("BACKEND_ENABLE_BINANCE_VISION", "true"), true),
		EnableCryptoDataDownload:  parseBool(getenv("BACKEND_ENABLE_CRYPTODATADOWNLOAD", "true"), true),
		EnableCoinGecko:           parseBool(getenv("BACKEND_ENABLE_COINGECKO", "true"), true),
		PlannerLeaseName:          getenv("BACKEND_PLANNER_LEASE_NAME", "planner_leader"),
		PlannerLeaseTTL:           parseDuration(getenv("BACKEND_PLANNER_LEASE_TTL", "15s")),
		PlannerTick:               parseDuration(getenv("BACKEND_PLANNER_TICK", "10s")),
		RealtimePollInterval:      parseDuration(getenv("BACKEND_REALTIME_POLL_INTERVAL", "5s")),
		WorkerPollInterval:        parseDuration(getenv("BACKEND_WORKER_POLL_INTERVAL", "5s")),
		WorkerBatchSize:           parseInt(getenv("BACKEND_WORKER_BATCH_SIZE", "100"), 100),
		PlannerBackfillBatchPct:   parsePercent(getenv("BACKEND_PLANNER_BACKFILL_BATCH_PERCENT", "50"), 50),
		WorkerBackfillBatchPct:    parsePercent(getenv("BACKEND_WORKER_BACKFILL_BATCH_PERCENT", "50"), 50),
		WorkerConcurrency:         parseInt(getenv("BACKEND_WORKER_CONCURRENCY", "4"), 4),
		CoinGeckoAPIKey:           getenv("BACKEND_COINGECKO_API_KEY", ""),
		CDDAvailabilityBaseURL:    getenv("BACKEND_CDD_AVAILABILITY_BASE_URL", ""),
		IntegrityMinSources:       parseInt(getenv("BACKEND_INTEGRITY_MIN_SOURCES", "2"), 2),
		IntegrityMaxDivergencePct: parseFloat(getenv("BACKEND_INTEGRITY_MAX_DIVERGENCE_PCT", "0.5"), 0.5),
		DefaultQuoteAssets:        splitCSV(getenv("BACKEND_DEFAULT_QUOTE_ASSETS", "EUR,USD")),
		TaskRetentionPeriod:       parseDuration(getenv("BACKEND_TASK_RETENTION_PERIOD", "24h")),
		TaskRetentionCount:        parseInt(getenv("BACKEND_TASK_MAX_LOG_COUNT", "1000"), 1000),
		TickerCacheSize:           parseInt(getenv("BACKEND_TICKER_CACHE_SIZE", "100"), 100),
		TickersCacheTTL:           parseDuration(getenv("BACKEND_TICKERS_CACHE_TTL", "30s")),
		NewsFetchInterval:         parseDuration(getenv("BACKEND_NEWS_FETCH_INTERVAL", "15m")),
		IntegrityCheckInterval:    parseDuration(getenv("BACKEND_INTEGRITY_CHECK_INTERVAL", "24h")),
		GapValidationInterval:     parseDuration(getenv("BACKEND_GAP_VALIDATION_INTERVAL", "24h")),
		AuthMode:                  normalizeAuthMode(getenv("BACKEND_AUTH_MODE", "")),
		GoogleClientID:            getenv("BACKEND_GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:        getenv("BACKEND_GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:         getenv("BACKEND_GOOGLE_REDIRECT_URI", "http://localhost:8080/api/v1/auth/google/callback"),
		JWTSecret:                 getenv("BACKEND_JWT_SECRET", ""),
		JWTExpiry:                 parseDuration(getenv("BACKEND_JWT_EXPIRY", "15m")),
		RefreshTokenExpiry:        parseDuration(getenv("BACKEND_REFRESH_TOKEN_EXPIRY", "168h")),
		AdminEmail:                getenv("BACKEND_ADMIN_EMAIL", ""),
		BcryptCost:                parseInt(getenv("BACKEND_BCRYPT_COST", "12"), 12),
		RateLimitUser:             parseInt(getenv("BACKEND_RATELIMIT_USER", "100"), 100),
		RateLimitPremium:          parseInt(getenv("BACKEND_RATELIMIT_PREMIUM", "500"), 500),
		RateLimitAdmin:            parseInt(getenv("BACKEND_RATELIMIT_ADMIN", "1000"), 1000),
		RateLimitIP:               parseInt(getenv("BACKEND_RATELIMIT_IP", "200"), 200),
		RateLimitWindow:           parseDuration(getenv("BACKEND_RATELIMIT_WINDOW", "1m")),
		TrustedProxies:            splitCSV(getenv("BACKEND_TRUSTED_PROXIES", "")),
		APIBaseURL:                getenv("API_BASE_URL", "http://localhost:8080/api/v1"),
		PortfolioEnabled:          parseBool(getenv("BACKEND_PORTFOLIO_ENABLED", "false"), false),
		PortfolioEncryptionKey:    getenv("BACKEND_PORTFOLIO_ENCRYPTION_KEY", ""),
		EtherscanAPIKey:           getenv("BACKEND_ETHERSCAN_API_KEY", ""),
		SolanaRPCURL:              getenv("BACKEND_SOLANA_RPC_URL", "https://api.mainnet-beta.solana.com"),
		BitcoinAPIURL:             getenv("BACKEND_BITCOIN_API_URL", "https://blockstream.info/api"),
	}
}

func corsOrigins(env string) []string {
	value, ok := os.LookupEnv("BACKEND_CORS_ALLOWED_ORIGINS")
	if ok {
		return splitCSV(value)
	}
	if strings.EqualFold(strings.TrimSpace(env), "development") {
		return []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	}
	return nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}

	return fallback
}

func splitCSV(value string) []string {
	raw := strings.Split(value, ",")
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}

	return items
}

func parseDuration(value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 15 * time.Second
	}

	return duration
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func parseFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func parseBool(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func parsePercent(value string, fallback int) int {
	parsed := parseInt(value, fallback)
	if parsed < 0 {
		return 0
	}
	if parsed > 100 {
		return 100
	}
	return parsed
}

// normalizeAuthMode lowercases and trims the auth mode value.
// Returns "" for empty/unset, or the cleaned value.
func normalizeAuthMode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// AuthModeHasLocal returns true if the auth mode includes local authentication.
func (c Config) AuthModeHasLocal() bool {
	return strings.Contains(c.AuthMode, "local")
}

// AuthModeHasSSO returns true if the auth mode includes SSO (Google OAuth).
func (c Config) AuthModeHasSSO() bool {
	return strings.Contains(c.AuthMode, "sso")
}

// AuthEnabled returns true if any auth mode is configured.
func (c Config) AuthEnabled() bool {
	return c.AuthMode != ""
}

// ValidateAuthConfig checks that the required environment variables are present
// for the configured auth mode. Returns a list of validation errors (empty = valid).
func (c Config) ValidateAuthConfig() []string {
	if !c.AuthEnabled() {
		return nil
	}

	var errs []string

	// Validate the mode value itself.
	switch c.AuthMode {
	case "local", "sso", "local,sso", "sso,local":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("BACKEND_AUTH_MODE %q is not valid; use \"local\", \"sso\", or \"local,sso\"", c.AuthMode))
		return errs // no point checking further
	}

	// JWT secret is always required when auth is enabled.
	if c.JWTSecret == "" {
		errs = append(errs, "BACKEND_JWT_SECRET is required when BACKEND_AUTH_MODE is set")
	}

	if c.AuthModeHasLocal() && c.AdminEmail == "" {
		errs = append(errs, "BACKEND_ADMIN_EMAIL is required when BACKEND_AUTH_MODE includes \"local\"")
	}

	if c.AuthModeHasSSO() {
		if c.GoogleClientID == "" {
			errs = append(errs, "BACKEND_GOOGLE_CLIENT_ID is required when BACKEND_AUTH_MODE includes \"sso\"")
		}
		if c.GoogleClientSecret == "" {
			errs = append(errs, "BACKEND_GOOGLE_CLIENT_SECRET is required when BACKEND_AUTH_MODE includes \"sso\"")
		}
	}

	return errs
}

// ValidatePortfolioConfig checks that required portfolio environment variables
// are present when portfolio features are enabled. Returns a list of validation
// errors (empty = valid).
func (c Config) ValidatePortfolioConfig() []string {
	if !c.PortfolioEnabled {
		return nil
	}

	var errs []string

	if c.PortfolioEncryptionKey == "" {
		errs = append(errs, "BACKEND_PORTFOLIO_ENCRYPTION_KEY is required when BACKEND_PORTFOLIO_ENABLED is true")
	}

	return errs
}
