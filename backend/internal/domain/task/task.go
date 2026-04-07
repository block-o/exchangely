package task

import "time"

const (
	TypeBackfill      = "historical_sweep"
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
	LastError   string     `json:"last_error,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	RetryCount  int        `json:"retry_count"`
	RetryAt     *time.Time `json:"retry_at,omitempty"`
}
