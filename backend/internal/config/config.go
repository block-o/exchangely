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
	CORSAllowedOrigins []string
	DatabaseURL        string
	KafkaBrokers       []string
	KafkaTasksTopic    string
	KafkaMarketTopic   string
	KafkaConsumerGroup string
	PlannerLeaseName   string
	PlannerLeaseTTL    time.Duration
	PlannerTick        time.Duration
	// RealtimePollInterval controls how frequently the planner emits fresh realtime
	// tasks for caught-up pairs. Shorter intervals mean fresher ticker prices.
	RealtimePollInterval time.Duration
	WorkerPollInterval   time.Duration
	WorkerBatchSize      int
	DefaultQuoteAssets   []string
}

func Load() Config {
	env := getenv("BACKEND_ENV", "development")

	return Config{
		Env:                  env,
		HTTPAddr:             getenv("BACKEND_HTTP_ADDR", ":8080"),
		Role:                 getenv("BACKEND_ROLE", "all"),
		LogLevel:             getenv("BACKEND_LOG_LEVEL", "info"),
		CORSAllowedOrigins:   corsOrigins(env),
		DatabaseURL:          getenv("BACKEND_DATABASE_URL", ""),
		KafkaBrokers:         splitCSV(getenv("BACKEND_KAFKA_BROKERS", "")),
		KafkaTasksTopic:      getenv("BACKEND_KAFKA_TOPIC_TASKS", "exchangely.tasks"),
		KafkaMarketTopic:     getenv("BACKEND_KAFKA_TOPIC_MARKET_TICKS", "exchangely.market.ticks"),
		KafkaConsumerGroup:   getenv("BACKEND_KAFKA_CONSUMER_GROUP", "exchangely-workers"),
		PlannerLeaseName:     getenv("BACKEND_PLANNER_LEASE_NAME", "planner_leader"),
		PlannerLeaseTTL:      parseDuration(getenv("BACKEND_PLANNER_LEASE_TTL", "15s")),
		PlannerTick:          parseDuration(getenv("BACKEND_PLANNER_TICK", "10s")),
		RealtimePollInterval: parseDuration(getenv("BACKEND_REALTIME_POLL_INTERVAL", "2m")),
		WorkerPollInterval:   parseDuration(getenv("BACKEND_WORKER_POLL_INTERVAL", "5s")),
		WorkerBatchSize:      parseInt(getenv("BACKEND_WORKER_BATCH_SIZE", "8"), 8),
		DefaultQuoteAssets:   splitCSV(getenv("BACKEND_DEFAULT_QUOTE_ASSETS", "EUR,USDT")),
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
