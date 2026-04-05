package consolidate

import (
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

func TestFromRawAggregatesMultipleSourcesPerBucket(t *testing.T) {
	items, err := FromRaw("1h", []candle.Candle{
		{Pair: "BTCUSD", Timestamp: 10, Open: 100, High: 110, Low: 95, Close: 108, Volume: 10},
		{Pair: "BTCUSD", Timestamp: 10, Open: 102, High: 111, Low: 94, Close: 107, Volume: 12},
	})
	if err != nil {
		t.Fatalf("consolidate failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(items))
	}
	if items[0].Open != 101 || items[0].Close != 107.5 || items[0].High != 111 || items[0].Low != 94 || items[0].Volume != 22 {
		t.Fatalf("unexpected consolidated candle: %+v", items[0])
	}
}

func TestDailyFromHourlyBuildsDayBucket(t *testing.T) {
	items, err := DailyFromHourly([]candle.Candle{
		{Pair: "BTCEUR", Timestamp: 1711929600, Open: 10, High: 12, Low: 9, Close: 11, Volume: 1},
		{Pair: "BTCEUR", Timestamp: 1711933200, Open: 11, High: 14, Low: 10, Close: 13, Volume: 2},
	})
	if err != nil {
		t.Fatalf("daily consolidate failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 daily candle, got %d", len(items))
	}
	if items[0].Open != 10 || items[0].Close != 13 || items[0].High != 14 || items[0].Low != 9 || items[0].Volume != 3 {
		t.Fatalf("unexpected daily candle: %+v", items[0])
	}
}
