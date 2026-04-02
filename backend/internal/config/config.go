package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                string
	HTTPAddr           string
	Role               string
	LogLevel           string
	DatabaseURL        string
	KafkaBrokers       []string
	KafkaTasksTopic    string
	KafkaMarketTopic   string
	KafkaConsumerGroup string
	PlannerLeaseName   string
	PlannerLeaseTTL    time.Duration
	PlannerTick        time.Duration
	WorkerPollInterval time.Duration
	WorkerBatchSize    int
	DefaultQuoteAssets []string
}

func Load() Config {
	return Config{
		Env:                getenv("BACKEND_ENV", "development"),
		HTTPAddr:           getenv("BACKEND_HTTP_ADDR", ":8080"),
		Role:               getenv("BACKEND_ROLE", "all"),
		LogLevel:           getenv("BACKEND_LOG_LEVEL", "info"),
		DatabaseURL:        getenv("BACKEND_DATABASE_URL", ""),
		KafkaBrokers:       splitCSV(getenv("BACKEND_KAFKA_BROKERS", "")),
		KafkaTasksTopic:    getenv("BACKEND_KAFKA_TOPIC_TASKS", "exchangely.tasks"),
		KafkaMarketTopic:   getenv("BACKEND_KAFKA_TOPIC_MARKET_TICKS", "exchangely.market.ticks"),
		KafkaConsumerGroup: getenv("BACKEND_KAFKA_CONSUMER_GROUP", "exchangely-workers"),
		PlannerLeaseName:   getenv("BACKEND_PLANNER_LEASE_NAME", "planner_leader"),
		PlannerLeaseTTL:    parseDuration(getenv("BACKEND_PLANNER_LEASE_TTL", "15s")),
		PlannerTick:        parseDuration(getenv("BACKEND_PLANNER_TICK", "10s")),
		WorkerPollInterval: parseDuration(getenv("BACKEND_WORKER_POLL_INTERVAL", "5s")),
		WorkerBatchSize:    parseInt(getenv("BACKEND_WORKER_BATCH_SIZE", "8"), 8),
		DefaultQuoteAssets: splitCSV(getenv("BACKEND_DEFAULT_QUOTE_ASSETS", "EUR,USDT")),
	}
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
