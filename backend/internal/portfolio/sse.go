package portfolio

import (
	domain "github.com/block-o/exchangely/backend/internal/domain/portfolio"
)

// HeldAssetPairs builds a set of ticker pair symbols (e.g. "BTCUSD") from a
// user's holdings for the given quote currency. This is used by the portfolio
// SSE stream to filter ticker updates to only those affecting held assets.
func HeldAssetPairs(holdings []domain.Holding, quoteCurrency string) map[string]bool {
	set := make(map[string]bool, len(holdings))
	for _, h := range holdings {
		pairSymbol := h.AssetSymbol + quoteCurrency
		set[pairSymbol] = true
	}
	return set
}

// HasRelevantUpdate checks whether any of the updated ticker pair symbols
// match an asset in the user's held set. Returns true if at least one updated
// pair is held, meaning a portfolio revaluation should be pushed.
func HasRelevantUpdate(heldPairs map[string]bool, updatedPairs []string) bool {
	for _, pair := range updatedPairs {
		if heldPairs[pair] {
			return true
		}
	}
	return false
}
