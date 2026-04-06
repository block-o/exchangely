package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaultsDevelopmentCORSOrigins(t *testing.T) {
	t.Setenv("BACKEND_ENV", "development")
	_ = os.Unsetenv("BACKEND_CORS_ALLOWED_ORIGINS")

	cfg := Load()

	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected 2 default dev origins, got %d", len(cfg.CORSAllowedOrigins))
	}
}

func TestLoadDisablesDefaultCORSOutsideDevelopment(t *testing.T) {
	t.Setenv("BACKEND_ENV", "production")
	_ = os.Unsetenv("BACKEND_CORS_ALLOWED_ORIGINS")

	cfg := Load()

	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Fatalf("expected no default origins outside development, got %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadUsesConfiguredCORSOrigins(t *testing.T) {
	t.Setenv("BACKEND_ENV", "production")
	t.Setenv("BACKEND_CORS_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")

	cfg := Load()

	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected configured origins, got %v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "https://app.example.com" || cfg.CORSAllowedOrigins[1] != "https://admin.example.com" {
		t.Fatalf("unexpected configured origins: %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadUsesConfiguredIntegrityValidatorSettings(t *testing.T) {
	t.Setenv("BACKEND_INTEGRITY_MIN_SOURCES", "3")
	t.Setenv("BACKEND_INTEGRITY_MAX_DIVERGENCE_PCT", "1.25")
	t.Setenv("BACKEND_COINGECKO_API_KEY", "demo-key")
	t.Setenv("BACKEND_ENABLE_BINANCE", "false")
	t.Setenv("BACKEND_ENABLE_KRAKEN", "false")
	t.Setenv("BACKEND_ENABLE_BINANCE_VISION", "true")
	t.Setenv("BACKEND_ENABLE_CRYPTODATADOWNLOAD", "false")
	t.Setenv("BACKEND_ENABLE_COINGECKO", "true")

	cfg := Load()

	if cfg.CoinGeckoAPIKey != "demo-key" {
		t.Fatalf("expected CoinGecko API key to load, got %q", cfg.CoinGeckoAPIKey)
	}
	if cfg.IntegrityMinSources != 3 {
		t.Fatalf("expected integrity min sources 3, got %d", cfg.IntegrityMinSources)
	}
	if cfg.IntegrityMaxDivergencePct != 1.25 {
		t.Fatalf("expected integrity divergence pct 1.25, got %v", cfg.IntegrityMaxDivergencePct)
	}
	if cfg.EnableBinance {
		t.Fatal("expected binance provider to be disabled")
	}
	if cfg.EnableKraken {
		t.Fatal("expected kraken provider to be disabled")
	}
	if !cfg.EnableBinanceVision {
		t.Fatal("expected binance vision provider to remain enabled")
	}
	if cfg.EnableCryptoDataDownload {
		t.Fatal("expected cryptodatadownload provider to be disabled")
	}
	if !cfg.EnableCoinGecko {
		t.Fatal("expected coingecko provider to be enabled")
	}
}

func TestLoadUsesConfiguredTaskRetention(t *testing.T) {
	t.Setenv("BACKEND_TASK_RETENTION_PERIOD", "48h")
	t.Setenv("BACKEND_TASK_MAX_LOG_COUNT", "5000")

	cfg := Load()

	if cfg.TaskRetentionPeriod != 48*time.Hour {
		t.Fatalf("expected task retention period 48h, got %v", cfg.TaskRetentionPeriod)
	}
	if cfg.TaskRetentionCount != 5000 {
		t.Fatalf("expected task retention count 5000, got %d", cfg.TaskRetentionCount)
	}
}
