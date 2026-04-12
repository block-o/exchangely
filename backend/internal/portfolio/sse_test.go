package portfolio

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// Feature: portfolio-tracker, Property 17: SSE suppression for non-held assets
//
// Validates: Requirements 6.3
//
// For any ticker price update for an asset NOT present in the connected user's
// portfolio, the filtering logic shall not flag the update as relevant.
func TestPropertySSESuppressionForNonHeldAssets(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		quoteCurrency := "USD"

		// Generate a set of held assets (1-5 assets from a known pool).
		allAssets := []string{"BTC", "ETH", "SOL", "ADA", "DOT", "AVAX", "LINK", "MATIC", "UNI", "AAVE"}
		numHeld := rapid.IntRange(1, 5).Draw(t, "numHeld")

		// Pick held assets using distinct indices.
		heldIndices := make(map[int]bool)
		for len(heldIndices) < numHeld {
			idx := rapid.IntRange(0, len(allAssets)-1).Draw(t, fmt.Sprintf("heldIdx_%d", len(heldIndices)))
			heldIndices[idx] = true
		}

		var holdings []domain.Holding
		heldSymbols := make(map[string]bool)
		for idx := range heldIndices {
			sym := allAssets[idx]
			heldSymbols[sym] = true
			holdings = append(holdings, domain.Holding{
				ID:            uuid.New(),
				UserID:        uuid.New(),
				AssetSymbol:   sym,
				Quantity:      1.0,
				QuoteCurrency: quoteCurrency,
				Source:        "manual",
			})
		}

		heldPairs := HeldAssetPairs(holdings, quoteCurrency)

		// Generate ticker updates only for assets NOT in the held set.
		var nonHeldAssets []string
		for _, sym := range allAssets {
			if !heldSymbols[sym] {
				nonHeldAssets = append(nonHeldAssets, sym)
			}
		}

		// If all assets are held, there are no non-held assets to test.
		if len(nonHeldAssets) == 0 {
			return
		}

		numUpdates := rapid.IntRange(1, len(nonHeldAssets)).Draw(t, "numUpdates")
		var updatedPairs []string
		for i := 0; i < numUpdates; i++ {
			sym := rapid.SampledFrom(nonHeldAssets).Draw(t, fmt.Sprintf("updateSym_%d", i))
			updatedPairs = append(updatedPairs, sym+quoteCurrency)
		}

		// The filtering logic should NOT flag any of these as relevant.
		relevant := HasRelevantUpdate(heldPairs, updatedPairs)
		if relevant {
			t.Fatalf("expected no relevant updates for non-held assets, but got relevant=true\nheld: %v\nupdated: %v",
				heldSymbols, updatedPairs)
		}
	})
}
