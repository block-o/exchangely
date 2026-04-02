package service

import (
	"strings"

	"github.com/block-o/exchangely/backend/internal/domain/asset"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

type CatalogService struct {
	assets []asset.Asset
	pairs  []pair.Pair
}

func NewCatalogService(quotes []string) *CatalogService {
	baseAssets := []asset.Asset{
		{Symbol: "BTC", Name: "Bitcoin", Type: "crypto"},
		{Symbol: "ETH", Name: "Ethereum", Type: "crypto"},
		{Symbol: "SOL", Name: "Solana", Type: "crypto"},
		{Symbol: "XRP", Name: "XRP", Type: "crypto"},
		{Symbol: "ADA", Name: "Cardano", Type: "crypto"},
		{Symbol: "LINK", Name: "Chainlink", Type: "crypto"},
		{Symbol: "AVAX", Name: "Avalanche", Type: "crypto"},
		{Symbol: "DOGE", Name: "Dogecoin", Type: "crypto"},
	}

	quoteAssets := make([]asset.Asset, 0, len(quotes))
	for _, quote := range quotes {
		quoteAssets = append(quoteAssets, asset.Asset{
			Symbol: strings.ToUpper(quote),
			Name:   strings.ToUpper(quote),
			Type:   "quote",
		})
	}

	allAssets := append(append([]asset.Asset{}, baseAssets...), quoteAssets...)
	pairs := make([]pair.Pair, 0, len(baseAssets)*len(quoteAssets))
	for _, base := range baseAssets {
		for _, quote := range quoteAssets {
			pairs = append(pairs, pair.Pair{
				Base:   base.Symbol,
				Quote:  quote.Symbol,
				Symbol: base.Symbol + quote.Symbol,
			})
		}
	}

	return &CatalogService{
		assets: allAssets,
		pairs:  pairs,
	}
}

func (s *CatalogService) Assets() []asset.Asset {
	return append([]asset.Asset{}, s.assets...)
}

func (s *CatalogService) Pairs() []pair.Pair {
	return append([]pair.Pair{}, s.pairs...)
}
