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
		groupKey := item.Timestamp
		if interval == "1h" {
			groupKey = time.Unix(item.Timestamp, 0).UTC().Truncate(time.Hour).Unix()
		}
		grouped[groupKey] = append(grouped[groupKey], item)
	}

	keys := make([]int64, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]candle.Candle, 0, len(keys))
	for _, key := range keys {
		group := grouped[key]
		sort.Slice(group, func(i, j int) bool { return group[i].Timestamp < group[j].Timestamp })

		earliestTs := group[0].Timestamp
		latestTs := group[len(group)-1].Timestamp

		openSum, openCount := 0.0, 0
		closeSum, closeCount := 0.0, 0
		high := group[0].High
		low := group[0].Low
		volume := 0.0

		for _, item := range group {
			if item.Timestamp == earliestTs {
				openSum += item.Open
				openCount++
			}
			if item.Timestamp == latestTs {
				closeSum += item.Close
				closeCount++
			}
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
			Open:      openSum / float64(openCount),
			High:      high,
			Low:       low,
			Close:     closeSum / float64(closeCount),
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
