package consolidate

import (
	"testing"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

func TestFromRawAggregatesMultipleSourcesPerBucket(t *testing.T) {
	items, err := FromRaw("1h", []candle.Candle{
		{Pair: "BTCUSD", Timestamp: 10, Open: 100, High: 110, Low: 95, Close: 108, Volume: 10, Source: "s1"},
		{Pair: "BTCUSD", Timestamp: 10, Open: 102, High: 111, Low: 94, Close: 107, Volume: 12, Source: "s2"},
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

func TestFromRawSnapshotsDeduplicate(t *testing.T) {
	// 3 snapshots from coingecko for the same timestamp.
	// Last one has different values.
	items, err := FromRaw("1h", []candle.Candle{
		{Pair: "BTCUSD", Timestamp: 1000, Open: 100, High: 101, Low: 99, Close: 100, Volume: 1, Source: "coingecko", Finalized: false},
		{Pair: "BTCUSD", Timestamp: 1000, Open: 100, High: 103, Low: 98, Close: 102, Volume: 2, Source: "coingecko", Finalized: false},
		{Pair: "BTCUSD", Timestamp: 1000, Open: 100, High: 104, Low: 97, Close: 103, Volume: 3, Source: "coingecko", Finalized: true},
	})
	if err != nil {
		t.Fatalf("FromRaw failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(items))
	}
	got := items[0]
	// Volume should be 3 (latest wins), not 6 (summed snapshots).
	if got.Volume != 3 || got.High != 104 || got.Low != 97 || got.Close != 103 || !got.Finalized {
		t.Fatalf("unexpected consolidated snapshot: %+v", got)
	}
}

func TestFromRawMultipleSourcesAggregates(t *testing.T) {
	// Same timestamp, different sources.
	items, err := FromRaw("1h", []candle.Candle{
		{Pair: "BTCUSD", Timestamp: 1000, Open: 100, High: 105, Low: 95, Close: 100, Volume: 10, Source: "binance"},
		{Pair: "BTCUSD", Timestamp: 1000, Open: 102, High: 106, Low: 96, Close: 104, Volume: 15, Source: "kraken"},
	})
	if err != nil {
		t.Fatalf("FromRaw failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(items))
	}
	got := items[0]
	// Price should be average: (100+102)/2 = 101; (100+104)/2 = 102.
	// Volume should be summed: 10+15 = 25.
	// High should be max(105, 106) = 106. Low should be min(95, 96) = 95.
	if got.Open != 101 || got.Close != 102 || got.High != 106 || got.Low != 95 || got.Volume != 25 {
		t.Fatalf("unexpected consolidated sources: %+v", got)
	}
}

func TestFromRawMultipleBuckets(t *testing.T) {
	// Mix of sub-intervals across two hours.
	items, err := FromRaw("1h", []candle.Candle{
		// Hour 1
		{Pair: "BTCUSD", Timestamp: 3600, Open: 10, High: 12, Low: 9, Close: 11, Volume: 1, Source: "s1"},
		{Pair: "BTCUSD", Timestamp: 3600 + 300, Open: 11, High: 13, Low: 10, Close: 12, Volume: 1, Source: "s1"},
		// Hour 2
		{Pair: "BTCUSD", Timestamp: 7200, Open: 12, High: 14, Low: 11, Close: 13, Volume: 1, Source: "s1"},
	})
	if err != nil {
		t.Fatalf("FromRaw failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(items))
	}
	if items[0].Timestamp != 3600 || items[0].Volume != 2 || items[0].Close != 12 {
		t.Fatalf("unexpected first hour: %+v", items[0])
	}
	if items[1].Timestamp != 7200 || items[1].Volume != 1 || items[1].Open != 12 {
		t.Fatalf("unexpected second hour: %+v", items[1])
	}
}

func TestDailyFromHourlyWithGaps(t *testing.T) {
	// Only 2 hours provided for a day.
	items, err := DailyFromHourly([]candle.Candle{
		{Pair: "BTCEUR", Timestamp: 1711929600, Open: 10, High: 12, Low: 9, Close: 11, Volume: 1},
		{Pair: "BTCEUR", Timestamp: 1711983600, Open: 15, High: 16, Low: 14, Close: 15.5, Volume: 3},
	})
	if err != nil {
		t.Fatalf("DailyFromHourly failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 daily candle, got %d", len(items))
	}
	got := items[0]
	// Open should be from the first hour (10). Close from the last hour (15.5).
	// Volume sum: 1+3=4. High max: 16. Low min: 9.
	if got.Open != 10 || got.Close != 15.5 || got.High != 16 || got.Low != 9 || got.Volume != 4 {
		t.Fatalf("unexpected daily candle with gaps: %+v", got)
	}
}

func TestConsolidateEdgeCases(t *testing.T) {
	// Empty slice
	res, err := FromRaw("1h", nil)
	if err != nil || res != nil {
		t.Fatalf("expected nil for empty slice, got %v, %v", res, err)
	}

	// Unsupported interval
	_, err = FromRaw("1m", []candle.Candle{{Pair: "BTCUSD"}})
	if err == nil {
		t.Fatal("expected error for 1m interval")
	}

	// Single candle
	items, _ := FromRaw("1h", []candle.Candle{
		{Pair: "BTCUSD", Timestamp: 3600, Open: 100, High: 105, Low: 95, Close: 102, Volume: 5, Source: "s1", Finalized: true},
	})
	if len(items) != 1 || items[0].Volume != 5 || !items[0].Finalized {
		t.Fatalf("unexpected single candle result: %+v", items[0])
	}
}
