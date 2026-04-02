package service

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/consolidate"
	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

type RealtimeMarketStore interface {
	UpsertRawCandles(ctx context.Context, interval string, candles []candle.Candle) error
	RawCandles(ctx context.Context, pairSymbol, interval string, start, end time.Time) ([]candle.Candle, error)
	UpsertCandles(ctx context.Context, interval string, candles []candle.Candle) error
}

type RealtimeIngestService struct {
	store RealtimeMarketStore
}

func NewRealtimeIngestService(store RealtimeMarketStore) *RealtimeIngestService {
	return &RealtimeIngestService{store: store}
}

func (s *RealtimeIngestService) IngestRealtimeCandles(ctx context.Context, candles []candle.Candle) error {
	if len(candles) == 0 {
		return nil
	}

	if err := s.store.UpsertRawCandles(ctx, "1h", candles); err != nil {
		return err
	}

	first := candles[0]
	windowStart := time.Unix(first.Timestamp, 0).UTC().Truncate(time.Hour)
	windowEnd := windowStart.Add(time.Hour)

	rawCandles, err := s.store.RawCandles(ctx, first.Pair, "1h", windowStart, windowEnd)
	if err != nil {
		return err
	}

	consolidated, err := consolidate.FromRaw("1h", rawCandles)
	if err != nil {
		return err
	}
	if len(consolidated) == 0 {
		return nil
	}

	return s.store.UpsertCandles(ctx, "1h", consolidated)
}
