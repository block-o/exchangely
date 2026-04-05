package service

import "testing"

func TestBootstrapAssetsIncludesCirculatingSupply(t *testing.T) {
	items := bootstrapAssets([]string{"EUR", "USD"})

	supplies := make(map[string]float64, len(items))
	for _, item := range items {
		supplies[item.Symbol] = item.CirculatingSupply
	}

	if supplies["BTC"] <= 0 {
		t.Fatalf("expected BTC circulating supply to be seeded, got %v", supplies["BTC"])
	}
	if supplies["ETH"] <= 0 {
		t.Fatalf("expected ETH circulating supply to be seeded, got %v", supplies["ETH"])
	}
	if supplies["EUR"] != 0 {
		t.Fatalf("expected quote assets to keep zero circulating supply, got EUR=%v", supplies["EUR"])
	}
	if supplies["USD"] != 0 {
		t.Fatalf("expected quote assets to keep zero circulating supply, got USD=%v", supplies["USD"])
	}
}
