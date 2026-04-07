package pair

import "time"

type Pair struct {
	Base          string    `json:"base"`
	Quote         string    `json:"quote"`
	Symbol        string    `json:"symbol"`
	BackfillStart time.Time `json:"backfill_start_at"`
}
