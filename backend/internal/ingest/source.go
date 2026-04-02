package ingest

import (
	"context"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

type Request struct {
	Pair      string
	Base      string
	Quote     string
	Interval  string
	StartTime time.Time
	EndTime   time.Time
}

type Source interface {
	Name() string
	Supports(request Request) bool
	FetchCandles(ctx context.Context, request Request) ([]candle.Candle, error)
}
