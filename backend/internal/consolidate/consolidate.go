package consolidate

import (
	"fmt"
	"sort"
	"time"

	"github.com/block-o/exchangely/backend/internal/domain/candle"
)

func FromRaw(interval string, items []candle.Candle) ([]candle.Candle, error) {
	if interval != "1h" && interval != "1d" {
		return nil, fmt.Errorf("unsupported raw consolidation interval %q", interval)
	}
	if len(items) == 0 {
		return nil, nil
	}

	grouped := map[int64][]candle.Candle{}
	for _, item := range items {
		grouped[item.Timestamp] = append(grouped[item.Timestamp], item)
	}

	keys := make([]int64, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]candle.Candle, 0, len(keys))
	for _, key := range keys {
		group := grouped[key]
		openSum := 0.0
		closeSum := 0.0
		high := group[0].High
		low := group[0].Low
		volume := 0.0
		for _, item := range group {
			openSum += item.Open
			closeSum += item.Close
			if item.High > high {
				high = item.High
			}
			if item.Low < low {
				low = item.Low
			}
			volume += item.Volume
		}
		result = append(result, candle.Candle{
			Pair:      group[0].Pair,
			Interval:  interval,
			Timestamp: key,
			Open:      openSum / float64(len(group)),
			High:      high,
			Low:       low,
			Close:     closeSum / float64(len(group)),
			Volume:    volume,
			Source:    "consolidated",
			Finalized: true,
		})
	}

	return result, nil
}

func DailyFromHourly(items []candle.Candle) ([]candle.Candle, error) {
	if len(items) == 0 {
		return nil, nil
	}

	sorted := append([]candle.Candle{}, items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })

	grouped := map[int64][]candle.Candle{}
	for _, item := range sorted {
		dayStart := time.Unix(item.Timestamp, 0).UTC().Truncate(24 * time.Hour).Unix()
		grouped[dayStart] = append(grouped[dayStart], item)
	}

	keys := make([]int64, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]candle.Candle, 0, len(keys))
	for _, key := range keys {
		group := grouped[key]
		open := group[0].Open
		closePrice := group[len(group)-1].Close
		high := group[0].High
		low := group[0].Low
		volume := 0.0
		for _, item := range group {
			if item.High > high {
				high = item.High
			}
			if item.Low < low {
				low = item.Low
			}
			volume += item.Volume
		}
		result = append(result, candle.Candle{
			Pair:      group[0].Pair,
			Interval:  "1d",
			Timestamp: key,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			Source:    "consolidated",
			Finalized: true,
		})
	}

	return result, nil
}
