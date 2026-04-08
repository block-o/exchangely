package app

import (
	"testing"

	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/ingest/provider"
)

func TestBuildSourcesHonorsProviderFlags(t *testing.T) {
	cfg := config.Config{
		EnableBinance:            false,
		EnableKraken:             true,
		EnableBinanceVision:      true,
		EnableCryptoDataDownload: false,
		EnableCoinGecko:          true,
		CoinGeckoAPIKey:          "demo-key",
	}

	sources := buildSources(cfg)

	if got, want := sourceNames(sources.registrySources), []string{"binancevision", "kraken", "coingecko"}; !equalStrings(got, want) {
		t.Fatalf("unexpected registry sources: got %v want %v", got, want)
	}
	if got, want := sourceNames(sources.validatorSources), []string{"binancevision", "kraken", "coingecko"}; !equalStrings(got, want) {
		t.Fatalf("unexpected validator sources: got %v want %v", got, want)
	}
	if got, want := sources.enabledNames, []string{"binancevision", "kraken", "coingecko"}; !equalStrings(got, want) {
		t.Fatalf("unexpected enabled source names: got %v want %v", got, want)
	}
}

func TestBuildSourcesKeepsBackfillOnlyProvidersOutOfValidatorSet(t *testing.T) {
	cfg := config.Config{
		EnableBinance:            false,
		EnableKraken:             false,
		EnableBinanceVision:      false,
		EnableCryptoDataDownload: true,
		EnableCoinGecko:          false,
	}

	sources := buildSources(cfg)

	if got, want := sourceNames(sources.registrySources), []string{"cryptodatadownload"}; !equalStrings(got, want) {
		t.Fatalf("unexpected registry sources: got %v want %v", got, want)
	}
	if len(sources.validatorSources) != 0 {
		t.Fatalf("expected no validator sources for backfill-only provider, got %v", sourceNames(sources.validatorSources))
	}
	if got, want := sources.enabledNames, []string{"cryptodatadownload"}; !equalStrings(got, want) {
		t.Fatalf("unexpected enabled source names: got %v want %v", got, want)
	}
}

func TestBuildSourcesReturnsEmptySetsWhenAllProvidersDisabled(t *testing.T) {
	sources := buildSources(config.Config{})

	if len(sources.registrySources) != 0 {
		t.Fatalf("expected no registry sources, got %v", sourceNames(sources.registrySources))
	}
	if len(sources.validatorSources) != 0 {
		t.Fatalf("expected no validator sources, got %v", sourceNames(sources.validatorSources))
	}
	if len(sources.enabledNames) != 0 {
		t.Fatalf("expected no enabled sources, got %v", sources.enabledNames)
	}
}

func sourceNames(sources []provider.Source) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name())
	}
	return names
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
