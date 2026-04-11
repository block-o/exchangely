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
func BuildDescription(t Task) string {
	fmtTime := func(ts time.Time) string { return ts.UTC().Format("Jan 2 2006 15:04") }

	switch t.Type {
	case TypeBackfill:
		return fmt.Sprintf("%s candles %s → %s", t.Interval, fmtTime(t.WindowStart), fmtTime(t.WindowEnd))
	case TypeConsolidate:
		return fmt.Sprintf("Rebuild daily candle for %s", fmtTime(t.WindowStart))
	case TypeDataSanity:
		return fmt.Sprintf("Integrity sweep %s %s → %s", t.Pair, fmtTime(t.WindowStart), fmtTime(t.WindowEnd))
	case TypeGapValidation:
		return fmt.Sprintf("Gap sweep %s %s → %s", t.Pair, fmtTime(t.WindowStart), fmtTime(t.WindowEnd))
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
