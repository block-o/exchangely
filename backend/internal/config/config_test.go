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

func TestLoadRateLimitDefaults(t *testing.T) {
	_ = os.Unsetenv("BACKEND_RATELIMIT_USER")
	_ = os.Unsetenv("BACKEND_RATELIMIT_PREMIUM")
	_ = os.Unsetenv("BACKEND_RATELIMIT_ADMIN")
	_ = os.Unsetenv("BACKEND_RATELIMIT_IP")
	_ = os.Unsetenv("BACKEND_RATELIMIT_WINDOW")

	cfg := Load()

	if cfg.RateLimitUser != 100 {
		t.Fatalf("expected RateLimitUser default 100, got %d", cfg.RateLimitUser)
	}
	if cfg.RateLimitPremium != 500 {
		t.Fatalf("expected RateLimitPremium default 500, got %d", cfg.RateLimitPremium)
	}
	if cfg.RateLimitAdmin != 1000 {
		t.Fatalf("expected RateLimitAdmin default 1000, got %d", cfg.RateLimitAdmin)
	}
	if cfg.RateLimitIP != 200 {
		t.Fatalf("expected RateLimitIP default 200, got %d", cfg.RateLimitIP)
	}
	if cfg.RateLimitWindow != 1*time.Minute {
		t.Fatalf("expected RateLimitWindow default 1m, got %v", cfg.RateLimitWindow)
	}
}

func TestLoadRateLimitCustomValues(t *testing.T) {
	t.Setenv("BACKEND_RATELIMIT_USER", "50")
	t.Setenv("BACKEND_RATELIMIT_PREMIUM", "250")
	t.Setenv("BACKEND_RATELIMIT_ADMIN", "2000")
	t.Setenv("BACKEND_RATELIMIT_IP", "400")
	t.Setenv("BACKEND_RATELIMIT_WINDOW", "2m")

	cfg := Load()

	if cfg.RateLimitUser != 50 {
		t.Fatalf("expected RateLimitUser 50, got %d", cfg.RateLimitUser)
	}
	if cfg.RateLimitPremium != 250 {
		t.Fatalf("expected RateLimitPremium 250, got %d", cfg.RateLimitPremium)
	}
	if cfg.RateLimitAdmin != 2000 {
		t.Fatalf("expected RateLimitAdmin 2000, got %d", cfg.RateLimitAdmin)
	}
	if cfg.RateLimitIP != 400 {
		t.Fatalf("expected RateLimitIP 400, got %d", cfg.RateLimitIP)
	}
	if cfg.RateLimitWindow != 2*time.Minute {
		t.Fatalf("expected RateLimitWindow 2m, got %v", cfg.RateLimitWindow)
	}
}

func TestLoadRateLimitFallbackOnInvalidValues(t *testing.T) {
	t.Setenv("BACKEND_RATELIMIT_USER", "not-a-number")
	t.Setenv("BACKEND_RATELIMIT_PREMIUM", "")
	t.Setenv("BACKEND_RATELIMIT_ADMIN", "abc")
	t.Setenv("BACKEND_RATELIMIT_IP", "12.5")
	t.Setenv("BACKEND_RATELIMIT_WINDOW", "invalid")

	cfg := Load()

	if cfg.RateLimitUser != 100 {
		t.Fatalf("expected RateLimitUser fallback 100, got %d", cfg.RateLimitUser)
	}
	if cfg.RateLimitPremium != 500 {
		t.Fatalf("expected RateLimitPremium fallback 500, got %d", cfg.RateLimitPremium)
	}
	if cfg.RateLimitAdmin != 1000 {
		t.Fatalf("expected RateLimitAdmin fallback 1000, got %d", cfg.RateLimitAdmin)
	}
	if cfg.RateLimitIP != 200 {
		t.Fatalf("expected RateLimitIP fallback 200, got %d", cfg.RateLimitIP)
	}
	if cfg.RateLimitWindow != 15*time.Second {
		t.Fatalf("expected RateLimitWindow fallback 15s (parseDuration default), got %v", cfg.RateLimitWindow)
	}
}

func TestLoadPortfolioDefaults(t *testing.T) {
	_ = os.Unsetenv("BACKEND_PORTFOLIO_ENABLED")
	_ = os.Unsetenv("BACKEND_PORTFOLIO_ENCRYPTION_KEY")
	_ = os.Unsetenv("BACKEND_ETHERSCAN_API_KEY")
	_ = os.Unsetenv("BACKEND_SOLANA_RPC_URL")
	_ = os.Unsetenv("BACKEND_BITCOIN_API_URL")

	cfg := Load()

	if cfg.PortfolioEnabled {
		t.Fatal("expected PortfolioEnabled to default to false")
	}
	if cfg.PortfolioEncryptionKey != "" {
		t.Fatalf("expected empty PortfolioEncryptionKey, got %q", cfg.PortfolioEncryptionKey)
	}
	if cfg.EtherscanAPIKey != "" {
		t.Fatalf("expected empty EtherscanAPIKey, got %q", cfg.EtherscanAPIKey)
	}
	if cfg.SolanaRPCURL != "https://api.mainnet-beta.solana.com" {
		t.Fatalf("expected default SolanaRPCURL, got %q", cfg.SolanaRPCURL)
	}
	if cfg.BitcoinAPIURL != "https://blockstream.info/api" {
		t.Fatalf("expected default BitcoinAPIURL, got %q", cfg.BitcoinAPIURL)
	}
}

func TestLoadPortfolioCustomValues(t *testing.T) {
	t.Setenv("BACKEND_PORTFOLIO_ENABLED", "true")
	t.Setenv("BACKEND_PORTFOLIO_ENCRYPTION_KEY", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
	t.Setenv("BACKEND_ETHERSCAN_API_KEY", "my-etherscan-key")
	t.Setenv("BACKEND_SOLANA_RPC_URL", "https://custom-solana.example.com")
	t.Setenv("BACKEND_BITCOIN_API_URL", "https://custom-btc.example.com/api")

	cfg := Load()

	if !cfg.PortfolioEnabled {
		t.Fatal("expected PortfolioEnabled to be true")
	}
	if cfg.PortfolioEncryptionKey != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Fatalf("expected custom PortfolioEncryptionKey, got %q", cfg.PortfolioEncryptionKey)
	}
	if cfg.EtherscanAPIKey != "my-etherscan-key" {
		t.Fatalf("expected custom EtherscanAPIKey, got %q", cfg.EtherscanAPIKey)
	}
	if cfg.SolanaRPCURL != "https://custom-solana.example.com" {
		t.Fatalf("expected custom SolanaRPCURL, got %q", cfg.SolanaRPCURL)
	}
	if cfg.BitcoinAPIURL != "https://custom-btc.example.com/api" {
		t.Fatalf("expected custom BitcoinAPIURL, got %q", cfg.BitcoinAPIURL)
	}
}

func TestValidatePortfolioConfigDisabled(t *testing.T) {
	cfg := Config{PortfolioEnabled: false}
	if errs := cfg.ValidatePortfolioConfig(); len(errs) != 0 {
		t.Fatalf("expected no errors when portfolio is disabled, got %v", errs)
	}
}

func TestValidatePortfolioConfigEnabledMissingKey(t *testing.T) {
	cfg := Config{PortfolioEnabled: true, PortfolioEncryptionKey: ""}
	errs := cfg.ValidatePortfolioConfig()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for missing encryption key, got %v", errs)
	}
}

func TestValidatePortfolioConfigEnabledWithKey(t *testing.T) {
	cfg := Config{
		PortfolioEnabled:       true,
		PortfolioEncryptionKey: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
	}
	if errs := cfg.ValidatePortfolioConfig(); len(errs) != 0 {
		t.Fatalf("expected no errors for valid portfolio config, got %v", errs)
	}
}

func TestLoadRecomputeDebounceWindowDefault(t *testing.T) {
	_ = os.Unsetenv("BACKEND_RECOMPUTE_DEBOUNCE_WINDOW")

	cfg := Load()

	if cfg.RecomputeDebounceWindow != 30*time.Second {
		t.Fatalf("expected RecomputeDebounceWindow default 30s, got %v", cfg.RecomputeDebounceWindow)
	}
}

func TestLoadRecomputeDebounceWindowCustom(t *testing.T) {
	t.Setenv("BACKEND_RECOMPUTE_DEBOUNCE_WINDOW", "45s")

	cfg := Load()

	if cfg.RecomputeDebounceWindow != 45*time.Second {
		t.Fatalf("expected RecomputeDebounceWindow 45s, got %v", cfg.RecomputeDebounceWindow)
	}
}

func TestLoadPnLRefreshIntervalDefault(t *testing.T) {
	_ = os.Unsetenv("BACKEND_PNL_REFRESH_INTERVAL")

	cfg := Load()

	if cfg.PnLRefreshInterval != time.Hour {
		t.Fatalf("expected PnLRefreshInterval default 1h, got %v", cfg.PnLRefreshInterval)
	}
}

func TestLoadPnLRefreshIntervalCustom(t *testing.T) {
	t.Setenv("BACKEND_PNL_REFRESH_INTERVAL", "30m")

	cfg := Load()

	if cfg.PnLRefreshInterval != 30*time.Minute {
		t.Fatalf("expected PnLRefreshInterval 30m, got %v", cfg.PnLRefreshInterval)
	}
}
