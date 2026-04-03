package task

import "time"

const (
	TypeBackfill    = "backfill"
	TypeConsolidate = "consolidate"
	TypeRealtime    = "realtime_poll"
)

type Task struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Pair        string    `json:"pair"`
	Interval    string    `json:"interval"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Status      string    `json:"status,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}
