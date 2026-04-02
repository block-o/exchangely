package service

import (
	"context"
	"strings"

	"github.com/block-o/exchangely/backend/internal/domain/asset"
	"github.com/block-o/exchangely/backend/internal/domain/pair"
)

type CatalogRepository interface {
	UpsertAssets(ctx context.Context, assets []asset.Asset) error
	UpsertPairs(ctx context.Context, pairs []pair.Pair) error
	ListAssets(ctx context.Context) ([]asset.Asset, error)
	ListPairs(ctx context.Context) ([]pair.Pair, error)
}

type CatalogService struct {
	repo   CatalogRepository
	quotes []string
}

func NewCatalogService(repo CatalogRepository, quotes []string) *CatalogService {
	return &CatalogService{
		repo:   repo,
		quotes: quotes,
	}
}

func (s *CatalogService) Seed(ctx context.Context) error {
	assets := bootstrapAssets(s.quotes)
	if err := s.repo.UpsertAssets(ctx, assets); err != nil {
		return err
	}
	return s.repo.UpsertPairs(ctx, bootstrapPairs(assets))
}

func (s *CatalogService) Assets(ctx context.Context) ([]asset.Asset, error) {
	return s.repo.ListAssets(ctx)
}

func (s *CatalogService) Pairs(ctx context.Context) ([]pair.Pair, error) {
	return s.repo.ListPairs(ctx)
}

func bootstrapAssets(quotes []string) []asset.Asset {
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

	items := append([]asset.Asset{}, baseAssets...)
	for _, quote := range quotes {
		symbol := strings.ToUpper(strings.TrimSpace(quote))
		if symbol == "" {
			continue
		}
		name := symbol
		assetType := "quote"
		if symbol == "EUR" {
			name = "Euro"
			assetType = "fiat"
		}
		if symbol == "USDT" {
			name = "Tether USDt"
			assetType = "stablecoin"
		}
		items = append(items, asset.Asset{
			Symbol: symbol,
			Name:   name,
			Type:   assetType,
		})
	}

	return items
}

func bootstrapPairs(assets []asset.Asset) []pair.Pair {
	baseSymbols := make([]string, 0)
	quotes := make([]string, 0)

	for _, item := range assets {
		switch item.Type {
		case "crypto":
			baseSymbols = append(baseSymbols, item.Symbol)
		case "fiat", "stablecoin", "quote":
			quotes = append(quotes, item.Symbol)
		}
	}

	pairs := make([]pair.Pair, 0, len(baseSymbols)*len(quotes))
	for _, base := range baseSymbols {
		for _, quote := range quotes {
			pairs = append(pairs, pair.Pair{
				Base:   base,
				Quote:  quote,
				Symbol: base + quote,
			})
		}
	}

	return pairs
}
