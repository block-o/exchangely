package app

import (
	"testing"

	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/ingest"
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
	if got, want := sourceNames(sources.validatorSources), []string{"binancevision", "kraken"}; !equalStrings(got, want) {
		t.Fatalf("unexpected validator sources: got %v want %v", got, want)
	}
	if got, want := sources.enabledNames, []string{"binancevision", "kraken", "coingecko"}; !equalStrings(got, want) {
		t.Fatalf("unexpected enabled source names: got %v want %v", got, want)
	}
}

func sourceNames(sources []ingest.Source) []string {
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
