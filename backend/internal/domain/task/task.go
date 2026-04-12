package task

import (
	"fmt"
	"time"
)

const (
	TypeBackfill      = "historical_backfill"
	TypeConsolidate   = "consolidation"
	TypeRealtime      = "live_ticker"
	TypeDataSanity    = "integrity_check"
	TypeCleanup       = "task_cleanup"
	TypeNewsFetch     = "news_fetch"
	TypeGapValidation = "gap_validation"
)

type Task struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Pair        string     `json:"pair"`
	Interval    string     `json:"interval"`
	Cadence     string     `json:"cadence"`
	WindowStart time.Time  `json:"window_start"`
	WindowEnd   time.Time  `json:"window_end"`
	Status      string     `json:"status,omitempty"`
	Description string     `json:"description,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	RetryCount  int        `json:"retry_count"`
	RetryAt     *time.Time `json:"retry_at,omitempty"`
}

// BuildDescription returns a human-readable summary for a task based on its type
// and fields. Task types where the description would be redundant (e.g. live_ticker)
// return an empty string.
// BuildDescription sets the human-readable description and scheduling cadence
// on a task based on its type and fields.
func BuildDescription(t Task) string {
	fmtTime := func(ts time.Time) string { return ts.UTC().Format("Jan 2 2006 15:04") }

	switch t.Type {
	case TypeBackfill:
		return fmt.Sprintf("%s candles %s → %s", t.Interval, fmtTime(t.WindowStart), fmtTime(t.WindowEnd))
	case TypeConsolidate:
		return fmt.Sprintf("Rebuild daily candle for %s", fmtTime(t.WindowStart))
	case TypeDataSanity:
		return fmt.Sprintf("Verify %s candle integrity from %s to %s", t.Interval, fmtTime(t.WindowStart), fmtTime(t.WindowEnd))
	case TypeGapValidation:
		return fmt.Sprintf("Verify %s candle completeness from %s to %s", t.Interval, fmtTime(t.WindowStart), fmtTime(t.WindowEnd))
	case TypeCleanup:
		return "Prune completed/failed task log"
	case TypeNewsFetch:
		if t.Pair != "" && t.Pair != "*" {
			return fmt.Sprintf("Fetch news from %s", t.Pair)
		}
		return "Fetch latest crypto news"
	default:
		return ""
	}
}

// Enrich populates computed fields (Description and Cadence) on a task.
func Enrich(t *Task) {
	t.Description = BuildDescription(*t)
	t.Cadence = SchedulingCadence(*t)
}

// SchedulingCadence returns a human-readable string describing how often
// this task type is scheduled. This is derived from the task type, not the
// data resolution interval.
func SchedulingCadence(t Task) string {
	switch t.Type {
	case TypeRealtime:
		return t.Interval
	case TypeBackfill:
		return "once"
	case TypeConsolidate:
		return "once"
	case TypeDataSanity:
		return "24h"
	case TypeGapValidation:
		return "24h"
	case TypeCleanup:
		return "1d"
	case TypeNewsFetch:
		return t.Interval
	default:
		return t.Interval
	}
}
