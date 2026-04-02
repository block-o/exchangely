package service

import (
	"strings"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
	"github.com/block-o/exchangely/backend/internal/domain/ticker"
)

type MarketService struct{}

func NewMarketService() *MarketService {
	return &MarketService{}
}

func (s *MarketService) Historical(pairSymbol, interval string, start, end time.Time) []candle.Candle {
	if end.IsZero() {
		end = time.Now().UTC()
	}
	if start.IsZero() {
		if interval == "1d" {
			start = end.AddDate(0, 0, -14)
		} else {
			start = end.Add(-24 * time.Hour)
		}
	}

	step := time.Hour
	if interval == "1d" {
		step = 24 * time.Hour
	}

	result := make([]candle.Candle, 0)
	basePrice := float64(len(pairSymbol) * 100)
	index := 0.0

	for ts := start.UTC(); !ts.After(end.UTC()); ts = ts.Add(step) {
		open := basePrice + index
		close := open + 2.5
		result = append(result, candle.Candle{
			Pair:      strings.ToUpper(pairSymbol),
			Interval:  interval,
			Timestamp: ts.Unix(),
			Open:      open,
			High:      close + 1.25,
			Low:       open - 1.10,
			Close:     close,
			Volume:    1000 + (index * 10),
			Source:    "scaffold",
			Finalized: true,
		})
		index += 1
	}

	return result
}

func (s *MarketService) Ticker(pairSymbol string) ticker.Ticker {
	pairSymbol = strings.ToUpper(pairSymbol)
	price := float64(len(pairSymbol) * 100)
	return ticker.Ticker{
		Pair:           pairSymbol,
		Price:          price + 12.75,
		Variation24H:   3.42,
		LastUpdateUnix: time.Now().UTC().Unix(),
		Source:         "scaffold",
	}
}
