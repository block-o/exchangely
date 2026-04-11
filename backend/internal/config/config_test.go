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

func TestLoadClampsPlannerBackfillBatchPercent(t *testing.T) {
	t.Setenv("BACKEND_PLANNER_BACKFILL_BATCH_PERCENT", "150")

	cfg := Load()

	if cfg.PlannerBackfillBatchPct != 100 {
		t.Fatalf("expected planner backfill percent to clamp to 100, got %d", cfg.PlannerBackfillBatchPct)
	}
}

func TestLoadAllowsPlannerBackfillBatchPercentToDisableBackfill(t *testing.T) {
	t.Setenv("BACKEND_PLANNER_BACKFILL_BATCH_PERCENT", "-20")

	cfg := Load()

	if cfg.PlannerBackfillBatchPct != 0 {
		t.Fatalf("expected planner backfill percent to clamp to 0, got %d", cfg.PlannerBackfillBatchPct)
	}
}

func TestLoadClampsWorkerBackfillBatchPercent(t *testing.T) {
	t.Setenv("BACKEND_WORKER_BACKFILL_BATCH_PERCENT", "150")

	cfg := Load()

	if cfg.WorkerBackfillBatchPct != 100 {
		t.Fatalf("expected worker backfill percent to clamp to 100, got %d", cfg.WorkerBackfillBatchPct)
	}
}

func TestLoadAuthModeNormalization(t *testing.T) {
	t.Setenv("BACKEND_AUTH_MODE", "  Local,SSO  ")

	cfg := Load()

	if cfg.AuthMode != "local,sso" {
		t.Fatalf("expected normalized auth mode %q, got %q", "local,sso", cfg.AuthMode)
	}
}

func TestLoadAuthModeEmptyByDefault(t *testing.T) {
	_ = os.Unsetenv("BACKEND_AUTH_MODE")

	cfg := Load()

	if cfg.AuthMode != "" {
		t.Fatalf("expected empty auth mode by default, got %q", cfg.AuthMode)
	}
}

func TestAuthModeHelpers(t *testing.T) {
	tests := []struct {
		mode     string
		hasLocal bool
		hasSSO   bool
		enabled  bool
	}{
		{"", false, false, false},
		{"local", true, false, true},
		{"sso", false, true, true},
		{"local,sso", true, true, true},
		{"sso,local", true, true, true},
	}

	for _, tc := range tests {
		cfg := Config{AuthMode: tc.mode}
		if cfg.AuthModeHasLocal() != tc.hasLocal {
			t.Errorf("mode %q: AuthModeHasLocal() = %v, want %v", tc.mode, cfg.AuthModeHasLocal(), tc.hasLocal)
		}
		if cfg.AuthModeHasSSO() != tc.hasSSO {
			t.Errorf("mode %q: AuthModeHasSSO() = %v, want %v", tc.mode, cfg.AuthModeHasSSO(), tc.hasSSO)
		}
		if cfg.AuthEnabled() != tc.enabled {
			t.Errorf("mode %q: AuthEnabled() = %v, want %v", tc.mode, cfg.AuthEnabled(), tc.enabled)
		}
	}
}

func TestValidateAuthConfigDisabledMode(t *testing.T) {
	cfg := Config{AuthMode: ""}
	if errs := cfg.ValidateAuthConfig(); len(errs) != 0 {
		t.Fatalf("expected no errors for disabled auth, got %v", errs)
	}
}

func TestValidateAuthConfigInvalidMode(t *testing.T) {
	cfg := Config{AuthMode: "magic"}
	errs := cfg.ValidateAuthConfig()
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid auth mode")
	}
}

func TestValidateAuthConfigLocalMissingJWTSecret(t *testing.T) {
	cfg := Config{AuthMode: "local", AdminEmail: "admin@example.com"}
	errs := cfg.ValidateAuthConfig()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error (missing JWT secret), got %v", errs)
	}
}

func TestValidateAuthConfigLocalMissingAdminEmail(t *testing.T) {
	cfg := Config{AuthMode: "local", JWTSecret: "secret"}
	errs := cfg.ValidateAuthConfig()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error (missing admin email), got %v", errs)
	}
}

func TestValidateAuthConfigSSOValid(t *testing.T) {
	cfg := Config{
		AuthMode:           "sso",
		JWTSecret:          "secret",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
	}
	if errs := cfg.ValidateAuthConfig(); len(errs) != 0 {
		t.Fatalf("expected no errors for valid SSO config, got %v", errs)
	}
}

func TestValidateAuthConfigSSOMissingGoogleCreds(t *testing.T) {
	cfg := Config{AuthMode: "sso", JWTSecret: "secret"}
	errs := cfg.ValidateAuthConfig()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (missing Google client ID and secret), got %v", errs)
	}
}

func TestValidateAuthConfigBothModesValid(t *testing.T) {
	cfg := Config{
		AuthMode:           "local,sso",
		JWTSecret:          "secret",
		AdminEmail:         "admin@example.com",
		GoogleClientID:     "id",
		GoogleClientSecret: "secret",
	}
	if errs := cfg.ValidateAuthConfig(); len(errs) != 0 {
		t.Fatalf("expected no errors for valid local,sso config, got %v", errs)
	}
}
