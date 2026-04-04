package config

import (
	"os"
	"testing"
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
}
