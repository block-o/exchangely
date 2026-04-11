package task

import (
	"testing"
	"time"
)

func TestBuildDescriptionPerTaskType(t *testing.T) {
	base := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		task Task
		want string
	}{
		{
			name: "hourly backfill",
			task: Task{Type: TypeBackfill, Interval: "1h", WindowStart: base, WindowEnd: base.Add(time.Hour)},
			want: "1h candles Apr 1 2026 10:00 → Apr 1 2026 11:00",
		},
		{
			name: "daily backfill",
			task: Task{Type: TypeBackfill, Interval: "1d", WindowStart: base, WindowEnd: base.Add(24 * time.Hour)},
			want: "1d candles Apr 1 2026 10:00 → Apr 2 2026 10:00",
		},
		{
			name: "consolidation",
			task: Task{Type: TypeConsolidate, WindowStart: base},
			want: "Rebuild daily candle for Apr 1 2026 10:00",
		},
		{
			name: "integrity check",
			task: Task{Type: TypeDataSanity, Pair: "BTCEUR", Interval: "1h", WindowStart: base, WindowEnd: base.Add(time.Hour)},
			want: "Integrity sweep BTCEUR Apr 1 2026 10:00 → Apr 1 2026 11:00",
		},
		{
			name: "gap validation",
			task: Task{Type: TypeGapValidation, Pair: "BTCEUR", WindowStart: base, WindowEnd: base.Add(24 * time.Hour)},
			want: "Gap sweep BTCEUR Apr 1 2026 10:00 → Apr 2 2026 10:00",
		},
		{
			name: "cleanup",
			task: Task{Type: TypeCleanup},
			want: "Prune completed/failed task log",
		},
		{
			name: "news fetch with source",
			task: Task{Type: TypeNewsFetch, Pair: "coindesk"},
			want: "Fetch news from coindesk",
		},
		{
			name: "news fetch global",
			task: Task{Type: TypeNewsFetch, Pair: "*"},
			want: "Fetch latest crypto news",
		},
		{
			name: "live ticker returns empty",
			task: Task{Type: TypeRealtime},
			want: "",
		},
		{
			name: "unknown type returns empty",
			task: Task{Type: "something_new"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildDescription(tt.task)
			if got != tt.want {
				t.Errorf("BuildDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
