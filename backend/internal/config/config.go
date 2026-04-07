package config

import (
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
	PlannerLeaseName         string
	PlannerLeaseTTL          time.Duration
	PlannerTick              time.Duration
	// RealtimePollInterval controls how frequently the planner emits fresh realtime
	// tasks for caught-up pairs. Shorter intervals mean fresher ticker prices but
	// also increase external API load and internal task volume.
	RealtimePollInterval      time.Duration
	WorkerPollInterval        time.Duration
	WorkerBatchSize           int
	PlannerBackfillBatchPct   int
	WorkerBackfillBatchPct    int
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
	// DefaultBackfillStart defines the earliest date for which the system will attempt
	// to fetch historical data for newly discovered pairs.
	DefaultBackfillStart time.Time
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
		RealtimePollInterval:      parseDuration(getenv("BACKEND_REALTIME_POLL_INTERVAL", "2m")),
		WorkerPollInterval:        parseDuration(getenv("BACKEND_WORKER_POLL_INTERVAL", "5s")),
		WorkerBatchSize:           parseInt(getenv("BACKEND_WORKER_BATCH_SIZE", "100"), 100),
		PlannerBackfillBatchPct:   parsePercent(getenv("BACKEND_PLANNER_BACKFILL_BATCH_PERCENT", "20"), 50),
		WorkerBackfillBatchPct:    parsePercent(getenv("BACKEND_WORKER_BACKFILL_BATCH_PERCENT", "50"), 50),
		CoinGeckoAPIKey:           getenv("BACKEND_COINGECKO_API_KEY", ""),
		IntegrityMinSources:       parseInt(getenv("BACKEND_INTEGRITY_MIN_SOURCES", "2"), 2),
		IntegrityMaxDivergencePct: parseFloat(getenv("BACKEND_INTEGRITY_MAX_DIVERGENCE_PCT", "0.5"), 0.5),
		DefaultQuoteAssets:        splitCSV(getenv("BACKEND_DEFAULT_QUOTE_ASSETS", "EUR,USD")),
		TaskRetentionPeriod:       parseDuration(getenv("BACKEND_TASK_RETENTION_PERIOD", "24h")),
		TaskRetentionCount:        parseInt(getenv("BACKEND_TASK_MAX_LOG_COUNT", "1000"), 1000),
		TickerCacheSize:           parseInt(getenv("BACKEND_TICKER_CACHE_SIZE", "100"), 100),
		TickersCacheTTL:           parseDuration(getenv("BACKEND_TICKERS_CACHE_TTL", "30s")),
		NewsFetchInterval:         parseDuration(getenv("BACKEND_NEWS_FETCH_INTERVAL", "5m")),
		DefaultBackfillStart:      parseTime(getenv("BACKEND_DEFAULT_BACKFILL_START", "2017-01-01T00:00:00Z")),
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

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Default to 2017-01-01 if invalid
		return time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return t
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
