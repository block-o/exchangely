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
			want: "Verify 1h candle integrity from Apr 1 2026 10:00 to Apr 1 2026 11:00",
		},
		{
			name: "gap validation",
			task: Task{Type: TypeGapValidation, Pair: "BTCEUR", Interval: "1d", WindowStart: base, WindowEnd: base.Add(24 * time.Hour)},
			want: "Verify 1d candle completeness from Apr 1 2026 10:00 to Apr 2 2026 10:00",
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

func TestSchedulingCadence(t *testing.T) {
	tests := []struct {
		name string
		task Task
		want string
	}{
		{"live_ticker uses interval", Task{Type: TypeRealtime, Interval: "5s"}, "5s"},
		{"backfill is one-shot", Task{Type: TypeBackfill, Interval: "1h"}, "once"},
		{"consolidation is one-shot", Task{Type: TypeConsolidate, Interval: "1d"}, "once"},
		{"integrity check is 24h", Task{Type: TypeDataSanity, Interval: "1h"}, "24h"},
		{"gap validation is 24h", Task{Type: TypeGapValidation, Interval: "1d"}, "24h"},
		{"cleanup is 1d", Task{Type: TypeCleanup, Interval: "1d"}, "1d"},
		{"news fetch uses interval", Task{Type: TypeNewsFetch, Interval: "15m"}, "15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SchedulingCadence(tt.task)
			if got != tt.want {
				t.Errorf("SchedulingCadence() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnrichPopulatesCadenceAndDescription(t *testing.T) {
	task := Task{
		Type:     TypeRealtime,
		Pair:     "BTCEUR",
		Interval: "5s",
	}
	Enrich(&task)

	if task.Cadence != "5s" {
		t.Errorf("Cadence = %q, want %q", task.Cadence, "5s")
	}

	backfill := Task{
		Type:        TypeBackfill,
		Interval:    "1h",
		WindowStart: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	Enrich(&backfill)

	if backfill.Cadence != "once" {
		t.Errorf("Cadence = %q, want %q", backfill.Cadence, "once")
	}
	if backfill.Description == "" {
		t.Error("expected non-empty description after Enrich")
	}
}
